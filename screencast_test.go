package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestScreenStream 端到端验证被控端链路：采集 → H.264 编码 → 访问单元切分 → WebSocket 发送。
func TestScreenStream(t *testing.T) {
	if os.Getenv("OPENREMOTE_STREAM_TEST") == "" {
		t.Skip("set OPENREMOTE_STREAM_TEST=1 to run the screen capture test")
	}

	abs, _ := filepath.Abs("bin")
	os.Setenv("PATH", os.Getenv("PATH")+string(os.PathListSeparator)+abs)

	type result struct {
		config map[string]any
		frames int
		keyAU  bool
		err    error
	}
	done := make(chan result, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			done <- result{err: err}
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		conn.SetReadLimit(16 << 20)

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		out := result{}
		for out.frames < 12 {
			msgType, data, err := conn.Read(ctx)
			if err != nil {
				out.err = err
				done <- out
				return
			}
			if msgType == websocket.MessageText {
				_ = json.Unmarshal(data, &out.config)
				continue
			}
			if out.frames == 0 {
				out.keyAU = strings.Contains(string(data), string([]byte{0, 0, 0, 1, 0x67}))
			}
			out.frames++
		}
		done <- out
		<-ctx.Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	go func() {
		_ = runScreenSession(ctx, "ws"+strings.TrimPrefix(server.URL, "http"))
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("stream error: %v", got.err)
		}
		if got.config["type"] != "video" {
			t.Fatalf("missing video config: %v", got.config)
		}
		if got.frames < 12 {
			t.Fatalf("too few frames: %d", got.frames)
		}
		if !got.keyAU {
			t.Fatalf("first access unit is not a keyframe with SPS")
		}
		t.Logf("config=%v frames=%d", got.config, got.frames)
	case <-time.After(18 * time.Second):
		t.Fatal("timeout waiting for frames")
	}
}

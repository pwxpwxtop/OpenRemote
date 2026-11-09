package node

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func dialRegister(t *testing.T, serverURL, id string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	writeMessage(conn, Message{Type: TypeRegister, DeviceId: id, Name: id, OS: "linux"})
	return conn
}

func readType(t *testing.T, conn *websocket.Conn, want string) Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read %s: %v", want, err)
		}
		var msg Message
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		if msg.Type == want {
			return msg
		}
	}
}

func TestRelay(t *testing.T) {
	hub := NewHub("")
	server := httptest.NewServer(hub.Handler())
	defer server.Close()

	agent := dialRegister(t, server.URL, "agent")
	defer agent.Close(websocket.StatusNormalClosure, "")
	control := dialRegister(t, server.URL, "control")
	defer control.Close(websocket.StatusNormalClosure, "")

	time.Sleep(200 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, _ := json.Marshal(Message{Type: TypeConnectRequest, RequestId: "r1", TargetId: "agent"})
	if err := control.Write(ctx, websocket.MessageText, request); err != nil {
		t.Fatal(err)
	}

	incoming := readType(t, agent, TypeIncomingRequest)
	response, _ := json.Marshal(Message{Type: TypeConnectResponse, RequestId: incoming.RequestId, Accepted: true})
	if err := agent.Write(ctx, websocket.MessageText, response); err != nil {
		t.Fatal(err)
	}

	result := readType(t, control, TypeConnectResult)
	if !result.Accepted || result.WsUrl == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	tunnel := readType(t, agent, TypeTunnelRequest)
	if tunnel.WsUrl == "" {
		t.Fatalf("unexpected tunnel: %+v", tunnel)
	}

	proxyConn, _, err := websocket.Dial(ctx, result.WsUrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer proxyConn.Close(websocket.StatusNormalClosure, "")

	agentConn, _, err := websocket.Dial(ctx, tunnel.WsUrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer agentConn.Close(websocket.StatusNormalClosure, "")

	if err := agentConn.Write(ctx, websocket.MessageBinary, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	msgType, data, err := proxyConn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if msgType != websocket.MessageBinary || len(data) != 3 || data[1] != 2 {
		t.Fatalf("unexpected frame: type=%v data=%v", msgType, data)
	}

	if err := proxyConn.Write(ctx, websocket.MessageText, []byte(`{"event":"keyframe"}`)); err != nil {
		t.Fatal(err)
	}
	msgType, data, err = agentConn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if msgType != websocket.MessageText || string(data) != `{"event":"keyframe"}` {
		t.Fatalf("unexpected input: type=%v data=%s", msgType, data)
	}
}

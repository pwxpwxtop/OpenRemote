package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

const (
	defaultFPS     = 30
	defaultBitrate = 2500 // kbps
)

// errRestartEncoder 表示需要重启编码器：分辨率变化或控制端请求了关键帧。
var errRestartEncoder = errors.New("restart encoder")

// signalRestart 触发一次编码器重启，重复请求会被合并。
func signalRestart(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// scaledSize 按最大宽度等比缩放，宽度为 0 或无需缩放时返回原始尺寸。
// 输出宽高都取偶数，保证 H.264 4:2:0 编码可用。
func scaledSize(w, h, maxWidth int) (int, int) {
	if maxWidth <= 0 || w <= maxWidth || w <= 0 || h <= 0 {
		return w, h
	}
	outW := maxWidth &^ 1
	outH := (h*outW/w + 1) &^ 1
	if outH < 2 {
		outH = 2
	}
	return outW, outH
}

// runScreenSession 是被控端的共享会话：
// 用 ffmpeg 采集并编码 H.264 Annex-B，把每个访问单元作为一条二进制消息发给节点，
// 同时从同一条通道接收键鼠事件并注入系统。
func runScreenSession(ctx context.Context, wsURL string) error {
	dialCtx, dialCancel := context.WithTimeout(ctx, 15*time.Second)
	conn, _, err := websocket.Dial(dialCtx, wsURL, nil)
	dialCancel()
	if err != nil {
		return err
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(1 << 20)

	sessionCtx, stop := context.WithCancel(ctx)
	defer stop()

	_, _, vw, vh := virtualScreen()
	if vw <= 0 || vh <= 0 {
		return fmt.Errorf("无法获取屏幕尺寸")
	}

	var screenW, screenH atomic.Int64
	screenW.Store(int64(vw))
	screenH.Store(int64(vh))

	// 控制端可调整画质：maxWidth=0 表示原始分辨率。
	var streamMaxWidth atomic.Int64
	streamBitrate := atomic.Int64{}
	streamBitrate.Store(defaultBitrate)
	streamFps := atomic.Int64{}
	streamFps.Store(defaultFPS)

	restart := make(chan struct{}, 1)

	// 文件传输与视频共用一个通道，按二进制首字节区分消息类型。
	files := newFileBroker(conn)

	// 控制端输入：键鼠事件注入系统，keyframe / 画质调整会重启编码器。
	go func() {
		for {
			msgType, data, err := conn.Read(sessionCtx)
			if err != nil {
				stop()
				return
			}
			if msgType == websocket.MessageBinary {
				files.handleChunk(sessionCtx, data)
				continue
			}

			var ev inputEvent
			if json.Unmarshal(data, &ev) != nil {
				continue
			}
			switch ev.Event {
			case "keyframe":
				signalRestart(restart)
			case "stream":
				width := ev.MaxWidth
				if width < 0 {
					width = 0
				}
				bitrate := ev.Bitrate
				if bitrate <= 0 {
					bitrate = defaultBitrate
				}
				if bitrate < 200 {
					bitrate = 200
				}
				if bitrate > 20000 {
					bitrate = 20000
				}
				fps := ev.Fps
				if fps <= 0 {
					fps = defaultFPS
				}
				if fps < 1 {
					fps = 1
				}
				if fps > 60 {
					fps = 60
				}
				if int(streamMaxWidth.Load()) != width || int(streamBitrate.Load()) != bitrate || int(streamFps.Load()) != fps {
					streamMaxWidth.Store(int64(width))
					streamBitrate.Store(int64(bitrate))
					streamFps.Store(int64(fps))
					signalRestart(restart)
				}
			case "file":
				files.handleEvent(sessionCtx, ev)
			case "clipboard":
				go pasteClipboard(ev.Text)
			case "copy":
				// 控制端按下 Ctrl+C 后，把远端剪切板内容回传过去。
				go sendClipboardToController(conn)
			default:
				handleInput(ev, int(screenW.Load()), int(screenH.Load()))
			}
		}
	}()

	encoders, err := availableEncoders("h264")
	if err != nil {
		sendAgentError(conn, err.Error())
		return err
	}
	encoderIndex := 0

	for {
		if encoderIndex >= len(encoders) {
			sendAgentError(conn, "远端没有可用的 H.264 编码器")
			return fmt.Errorf("所有 H.264 编码器都无法工作")
		}

		vx, vy, width, height := virtualScreen()
		if width <= 0 || height <= 0 {
			return fmt.Errorf("无法获取屏幕尺寸")
		}
		screenW.Store(int64(width))
		screenH.Store(int64(height))

		encoder := encoders[encoderIndex]
		maxWidth := int(streamMaxWidth.Load())
		bitrate := int(streamBitrate.Load())
		fps := int(streamFps.Load())
		outW, outH := scaledSize(width, height, maxWidth)
		enc, err := startFFmpeg(encoder, vx, vy, width, height, fps, bitrate, outW, outH)
		if err != nil {
			log.Printf("[screen] start %s: %v", encoder, err)
			encoderIndex++
			continue
		}

		if err := sendVideoConfig(conn, outW, outH); err != nil {
			enc.stop()
			return err
		}

		err = pumpFrames(sessionCtx, conn, enc, restart, width, height)
		enc.stop()

		if errors.Is(err, errRestartEncoder) {
			log.Printf("[screen] restarting encoder")
			continue
		}
		if sessionCtx.Err() != nil {
			return sessionCtx.Err()
		}

		// 硬件编码器可能能通过探测却在真实采集时失败，回退到下一个候选。
		log.Printf("[screen] encoder %s failed: %v", encoder, err)
		encoderIndex++
	}
}

func sendVideoConfig(conn *websocket.Conn, width, height int) error {
	config, err := json.Marshal(map[string]any{
		"type":   "video",
		"codec":  "h264",
		"width":  width,
		"height": height,
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, config)
}

// sendAgentError 把被控端的失败原因回传给控制端，便于界面直接展示。
func sendAgentError(conn *websocket.Conn, message string) {
	data, err := json.Marshal(map[string]string{"type": "error", "message": message})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = conn.Write(ctx, websocket.MessageText, data)
}

// sendClipboardToController 等远端应用完成复制后，把剪切板文本回传给控制端。
func sendClipboardToController(conn *websocket.Conn) {
	time.Sleep(250 * time.Millisecond)
	text := readClipboardText()
	if text == "" {
		return
	}
	data, err := json.Marshal(map[string]string{"type": "clipboard", "text": text})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = conn.Write(ctx, websocket.MessageText, data)
}

// pumpFrames 把 ffmpeg 输出的 Annex-B 码流切成访问单元逐个发送。
func pumpFrames(ctx context.Context, conn *websocket.Conn, enc *ffmpegEncoder, restart <-chan struct{}, width, height int) error {
	reader := newAnnexBReader(enc.stdout)
	var accessUnit []byte
	hasVCL := false
	sent := 0
	dropped := 0
	var bytesSent int
	lastSizeCheck := time.Now()

	// 独立的发送协程 + 单帧缓冲：网络跟不上时丢弃旧画面而不是排队，
	// 这样发出去的始终是最新帧，鼠标拖影和延迟不会越积越大。
	frames := make(chan []byte, 1)
	sendErr := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			select {
			case <-done:
				return
			case frame := <-frames:
				if err := writeAccessUnit(ctx, conn, frame); err != nil {
					sendErr <- err
					return
				}
			}
		}
	}()

	enqueue := func(au []byte, key bool) error {
		frame := make([]byte, len(au))
		copy(frame, au)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-sendErr:
			return err
		case frames <- frame:
			return nil
		default:
		}

		if !key {
			dropped++
			return nil
		}
		// 关键帧必须送达：挤掉积压的旧帧后再入队。
		select {
		case <-frames:
			dropped++
		default:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-sendErr:
			return err
		case frames <- frame:
			return nil
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-restart:
			return errRestartEncoder
		default:
		}

		raw, err := reader.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Printf("[screen] encoder output closed after %d frames", sent)
			} else {
				log.Printf("[screen] read encoder output: %v", err)
			}
			if stderr := enc.stderr.String(); stderr != "" {
				return fmt.Errorf("编码器中断: %s", firstLine(stderr))
			}
			return err
		}

		vcl, firstSlice := nalInfo(raw, false)
		if vcl {
			if hasVCL && firstSlice {
				if err := enqueue(accessUnit, hasKeyNal(accessUnit)); err != nil {
					return err
				}
				accessUnit = accessUnit[:0]
			}
			hasVCL = true
		} else if hasVCL {
			// 参数集/SEI 出现在上一个画面之后，说明下一个访问单元已经开始。
			if err := enqueue(accessUnit, hasKeyNal(accessUnit)); err != nil {
				return err
			}
			accessUnit = accessUnit[:0]
			hasVCL = false
		}
		accessUnit = append(accessUnit, raw...)

		sent++
		bytesSent += len(raw)
		if sent == 1 || sent%300 == 0 {
			log.Printf("[screen] sent frames=%d dropped=%d bytes=%d", sent, dropped, bytesSent)
		}

		// 分辨率变化时重启编码器，避免画面被拉伸或错位。
		if time.Since(lastSizeCheck) > time.Second {
			lastSizeCheck = time.Now()
			_, _, w, h := virtualScreen()
			if w != width || h != height {
				return errRestartEncoder
			}
		}
	}
}

// hasKeyNal 判断访问单元里是否含 IDR 或 SPS，这类帧不能被丢弃。
func hasKeyNal(au []byte) bool {
	offset := 0
	for {
		start, length, ok := indexStartCode(au, offset)
		if !ok {
			return false
		}
		head := start + length
		if head >= len(au) {
			return false
		}
		nalType := au[head] & 0x1F
		if nalType == 5 || nalType == 7 {
			return true
		}
		offset = head + 1
	}
}

func writeAccessUnit(ctx context.Context, conn *websocket.Conn, accessUnit []byte) error {
	if len(accessUnit) == 0 {
		return nil
	}
	// 二进制消息统一带一字节类型前缀，便于和文件分片共用一条通道。
	buf := make([]byte, 1+len(accessUnit))
	buf[0] = msgTypeVideo
	copy(buf[1:], accessUnit)

	writeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageBinary, buf)
}

// ffmpegEncoder 管理一个 ffmpeg 子进程，输出裸 H.264 码流。
type ffmpegEncoder struct {
	cmd      *exec.Cmd
	stdout   io.ReadCloser
	stderr   *limitedBuffer
	stopOnce sync.Once
}

func (e *ffmpegEncoder) stop() {
	e.stopOnce.Do(func() {
		if e.cmd != nil && e.cmd.Process != nil {
			_ = e.cmd.Process.Kill()
		}
		if e.stdout != nil {
			_ = e.stdout.Close()
		}
		if e.cmd != nil {
			_ = e.cmd.Wait()
		}
	})
}

// nalInfo 判断一个 Annex-B 片段是否是 VCL 片、以及是否是画面首片。
func nalInfo(raw []byte, isH265 bool) (vcl bool, firstSlice bool) {
	offset, length, ok := indexStartCode(raw, 0)
	if !ok {
		return false, false
	}
	nal := raw[offset+length:]
	if len(nal) < 2 {
		return false, false
	}

	if isH265 {
		nalType := (nal[0] >> 1) & 0x3F
		if nalType > 31 {
			return false, false
		}
		if len(nal) < 3 {
			return true, false
		}
		return true, nal[2]&0x80 != 0
	}

	nalType := nal[0] & 0x1F
	if nalType != 1 && nalType != 5 {
		return false, false
	}
	// H.264 的 slice 头以 ue(v) 编码的 first_mb_in_slice 开始，首位为 1 表示值为 0（画面首片）。
	return true, nal[1]&0x80 != 0
}

// indexStartCode 在 buf 中从 from 开始查找 Annex-B 起始码，返回起始位置与长度（3 或 4）。
func indexStartCode(buf []byte, from int) (int, int, bool) {
	for i := from; i+3 <= len(buf); i++ {
		if buf[i] != 0 || buf[i+1] != 0 {
			continue
		}
		if buf[i+2] == 1 {
			return i, 3, true
		}
		if buf[i+2] == 0 && i+3 < len(buf) && buf[i+3] == 1 {
			return i, 4, true
		}
	}
	return 0, 0, false
}

// annexBReader 从 ffmpeg 的标准输出中逐个读出带起始码的 NAL 片段。
type annexBReader struct {
	reader *bufio.Reader
	buf    []byte
	eof    bool
}

func newAnnexBReader(r io.Reader) *annexBReader {
	return &annexBReader{reader: bufio.NewReaderSize(r, 64*1024)}
}

func (a *annexBReader) next() ([]byte, error) {
	for {
		first, firstLen, ok := indexStartCode(a.buf, 0)
		if ok {
			next, _, okNext := indexStartCode(a.buf, first+firstLen)
			if okNext {
				raw := a.buf[first:next]
				a.buf = a.buf[next:]
				return raw, nil
			}
		}

		if a.eof {
			if ok && len(a.buf) > first {
				raw := a.buf[first:]
				a.buf = nil
				return raw, nil
			}
			return nil, io.EOF
		}

		chunk := make([]byte, 64*1024)
		n, err := a.reader.Read(chunk)
		if n > 0 {
			a.buf = append(a.buf, chunk[:n]...)
			continue
		}
		if err != nil {
			a.eof = true
		}
	}
}

// firstLine 取多行文本的第一行，用于把 ffmpeg 报错压成一条提示。
func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return text[:index]
	}
	return text
}

// limitedBuffer 只保留前若干字节的 stderr，用于失败时定位问题。
type limitedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.buf.Len() < 8*1024 {
		l.buf.Write(p)
	}
	return len(p), nil
}

func (l *limitedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

var (
	ffmpegPathOnce sync.Once
	ffmpegPath     string
	ffmpegPathErr  error

	encoderMu    sync.Mutex
	encoderCache = map[string][]string{}
)

// locateFFmpeg 优先使用程序目录下的 ffmpeg，其次回退到 PATH。
func locateFFmpeg() (string, error) {
	ffmpegPathOnce.Do(func() {
		if exe, err := os.Executable(); err == nil {
			dir := filepath.Dir(exe)
			candidates := []string{
				filepath.Join(dir, ffmpegName),
				filepath.Join(dir, "assets", ffmpegName),
				filepath.Join(dir, "bin", ffmpegName),
			}
			for _, candidate := range candidates {
				if _, statErr := os.Stat(candidate); statErr == nil {
					ffmpegPath = candidate
					return
				}
			}
		}
		if path, err := exec.LookPath("ffmpeg"); err == nil {
			ffmpegPath = path
			return
		}
		ffmpegPathErr = fmt.Errorf("未找到 ffmpeg，请安装 ffmpeg 或把它放在程序目录下")
	})
	log.Printf("[screen] ffmpeg=%q err=%v", ffmpegPath, ffmpegPathErr)
	return ffmpegPath, ffmpegPathErr
}

// availableEncoders 探测当前机器可用的编码器，按性能从高到低返回。
func availableEncoders(codec string) ([]string, error) {
	ffmpeg, err := locateFFmpeg()
	if err != nil {
		return nil, err
	}

	encoderMu.Lock()
	if cached, ok := encoderCache[codec]; ok {
		encoderMu.Unlock()
		return cached, nil
	}
	encoderMu.Unlock()

	available := make([]string, 0, 4)
	for _, name := range encoderCandidates(codec) {
		if encoderWorks(ffmpeg, name) {
			available = append(available, name)
		}
	}
	if len(available) == 0 {
		return nil, fmt.Errorf("没有可用的 %s 编码器，请确认 ffmpeg 支持硬件编码或软件编码", codec)
	}

	encoderMu.Lock()
	encoderCache[codec] = available
	encoderMu.Unlock()
	return available, nil
}

func encoderWorks(ffmpeg string, name string) bool {
	cmd := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=128x128:r=1:d=1",
		"-frames:v", "1", "-c:v", name, "-f", "null", "-",
	)
	hideWindow(cmd)
	return cmd.Run() == nil
}

// startFFmpeg 启动采集与编码进程，输出 H.264 Annex-B。
// outW/outH 是编码输出尺寸，小于采集尺寸时用缩放滤镜降分辨率。
func startFFmpeg(encoder string, vx, vy, vw, vh, fps, bitrate, outW, outH int) (*ffmpegEncoder, error) {
	ffmpeg, err := locateFFmpeg()
	if err != nil {
		return nil, err
	}
	if fps <= 0 {
		fps = defaultFPS
	}
	if bitrate <= 0 {
		bitrate = defaultBitrate
	}
	if outW <= 0 || outH <= 0 {
		outW, outH = vw, vh
	}

	rate := strconv.Itoa(bitrate) + "k"
	bufsize := strconv.Itoa(bitrate/2+200) + "k"
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-fflags", "nobuffer", "-flags", "low_delay",
	}
	args = append(args, captureArgs(fps, vx, vy, vw, vh)...)
	if outW != vw || outH != vh {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d:flags=fast_bilinear", outW, outH))
	}

	switch encoder {
	case "h264_nvenc":
		args = append(args, "-c:v", encoder, "-preset", "p1", "-tune", "ull", "-rc", "cbr",
			"-b:v", rate, "-maxrate", rate, "-bufsize", bufsize, "-g", strconv.Itoa(fps), "-bf", "0",
			"-profile:v", "baseline", "-pix_fmt", "yuv420p")
	case "h264_qsv":
		args = append(args, "-c:v", encoder, "-preset", "veryfast", "-low_delay_brc", "1",
			"-async_depth", "1", "-bf", "0", "-g", strconv.Itoa(fps), "-b:v", rate,
			"-profile:v", "baseline", "-pix_fmt", "nv12")
	case "h264_amf":
		args = append(args, "-c:v", encoder, "-usage", "lowlatency", "-quality", "speed",
			"-rc", "cbr", "-b:v", rate, "-g", strconv.Itoa(fps), "-bf", "0",
			"-profile:v", "baseline", "-pix_fmt", "yuv420p")
	case "h264_videotoolbox":
		args = append(args, "-c:v", encoder, "-realtime", "1", "-prio_speed", "1",
			"-b:v", rate, "-maxrate", rate, "-bufsize", bufsize,
			"-g", strconv.Itoa(fps), "-bf", "0", "-profile:v", "baseline", "-pix_fmt", "yuv420p")
	default:
		args = append(args, "-c:v", encoder, "-preset", "ultrafast", "-tune", "zerolatency",
			"-b:v", rate, "-maxrate", rate, "-bufsize", bufsize,
			"-g", strconv.Itoa(fps), "-bf", "0", "-profile:v", "baseline", "-pix_fmt", "yuv420p",
			"-x264-params", "repeat-headers=1:bframes=0:scenecut=0")
	}
	args = append(args, "-an", "-f", "h264", "pipe:1")

	cmd := exec.Command(ffmpeg, args...)
	hideWindow(cmd)

	stderr := &limitedBuffer{}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("创建 ffmpeg 管道失败: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 ffmpeg 失败: %w", err)
	}

	enc := &ffmpegEncoder{cmd: cmd, stdout: stdout, stderr: stderr}
	bindToJob(cmd)
	log.Printf("[screen] ffmpeg started with %s at %dx%d", encoder, vw, vh)
	return enc, nil
}

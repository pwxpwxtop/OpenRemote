package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/wailsapp/wails/v3/pkg/application"

	"changeme/internal/node"
)

// nodeVersion 是嵌入客户端的转发节点版本，用于判断服务器上的节点是否需要升级。
const nodeVersion = "1.5.0"

// HubService 是客户端与中转节点之间的桥：注册本机、维护在线设备、转发授权请求，
// 并在被控时把本机 H.264 画面流送进节点中转通道。
type HubService struct {
	app *application.App

	mu      sync.Mutex
	writeMu sync.Mutex
	conn    *websocket.Conn
	cancel  context.CancelFunc
	devices []node.DeviceInfo
	screens map[string]context.CancelFunc
}

func NewHubService() *HubService {
	return &HubService{screens: make(map[string]context.CancelFunc)}
}

// DeployNode 通过 SSH 在服务器上安装并启动转发节点，返回节点的 WebSocket 地址。
func (h *HubService) DeployNode(req DeployRequest) (string, error) {
	return deployNodeViaSSH(req)
}

// EnsureNode 确保服务器上已有可用的转发节点：先探测，已安装就直接返回节点地址；
// 没有安装则通过 SSH 自动上传并启动。
func (h *HubService) EnsureNode(host string, sshPort int, user string, password string, nodePort int) (string, error) {
	if host == "" {
		return "", fmt.Errorf("缺少主机地址")
	}
	if nodePort <= 0 {
		nodePort = 9000
	}
	if url, version := h.probeNode(host, nodePort); url != "" {
		// 节点已运行：没有密码无法升级时直接使用；版本一致也直接使用。
		if password == "" || version == nodeVersion {
			return url, nil
		}
	}
	if password == "" {
		return "", fmt.Errorf("服务器尚未安装转发节点，请填写 SSH 密码以便自动安装")
	}
	if sshPort <= 0 {
		sshPort = 22
	}
	return deployNodeViaSSH(DeployRequest{
		Host:     host,
		SSHPort:  sshPort,
		User:     user,
		Password: password,
		NodePort: nodePort,
	})
}

// TestConnection 测试服务器的 SSH 连接是否可用（不安装任何东西）。
func (h *HubService) TestConnection(host string, port int, user string, password string) error {
	return testSSHConnection(host, port, user, password)
}

// ProbeNode 探测服务器上是否已经装好并运行着转发节点。
func (h *HubService) ProbeNode(host string, port int) string {
	url, _ := h.probeNode(host, port)
	return url
}

// probeNode 同时返回节点地址和服务器上节点的版本号（旧版节点可能没有版本信息）。
func (h *HubService) probeNode(host string, port int) (string, string) {
	if host == "" {
		return "", ""
	}
	if port <= 0 {
		port = 9000
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+"/health", nil)
	if err != nil {
		return "", ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ""
	}

	var health struct {
		Version string `json:"version"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&health)
	return "ws://" + address + "/ws", health.Version
}

// Connect 连接节点并注册本机设备。
func (h *HubService) Connect(deviceId string, name string, nodeURL string) error {
	if deviceId == "" {
		return fmt.Errorf("缺少设备 ID")
	}
	if nodeURL == "" {
		return fmt.Errorf("缺少节点地址")
	}

	h.Disconnect()

	ctx, cancel := context.WithCancel(context.Background())
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 10*time.Second)
	conn, _, err := websocket.Dial(dialCtx, nodeURL, nil)
	dialCancel()
	if err != nil {
		cancel()
		return fmt.Errorf("连接节点失败: %w", err)
	}

	h.mu.Lock()
	h.conn = conn
	h.cancel = cancel
	h.mu.Unlock()

	if name == "" {
		name = deviceId
	}

	register := node.Message{
		Type:     node.TypeRegister,
		DeviceId: deviceId,
		Name:     name,
		OS:       runtime.GOOS,
	}
	if err := h.write(register); err != nil {
		h.Disconnect()
		return err
	}

	go h.readLoop(ctx, conn)
	return nil
}

// Disconnect 断开与节点的连接，并结束本机正在共享的画面会话。
func (h *HubService) Disconnect() {
	h.mu.Lock()
	conn := h.conn
	cancel := h.cancel
	h.conn = nil
	h.cancel = nil
	screens := h.screens
	h.screens = make(map[string]context.CancelFunc)
	h.mu.Unlock()

	for _, stop := range screens {
		stop()
	}

	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "client disconnect")
	}
}

// ListDevices 返回当前在线设备（含本机）。
func (h *HubService) ListDevices() []node.DeviceInfo {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]node.DeviceInfo(nil), h.devices...)
}

// RequestConnect 向目标设备发起远程控制请求，返回请求 ID。
// 结果通过 hub:connect-result 事件返回，成功时带上控制端直连节点的 wsUrl。
func (h *HubService) RequestConnect(targetId string) (string, error) {
	if targetId == "" {
		return "", fmt.Errorf("缺少目标设备")
	}
	requestId, err := randomRequestId()
	if err != nil {
		return "", err
	}
	if err := h.write(node.Message{
		Type:      node.TypeConnectRequest,
		RequestId: requestId,
		TargetId:  targetId,
	}); err != nil {
		return "", err
	}
	return requestId, nil
}

// Respond 是被控端的应答：接受或拒绝远程控制请求。
func (h *HubService) Respond(requestId string, accepted bool) error {
	if requestId == "" {
		return fmt.Errorf("缺少请求 ID")
	}
	return h.write(node.Message{
		Type:      node.TypeConnectResponse,
		RequestId: requestId,
		Accepted:  accepted,
	})
}

// ReadClipboard 读取本机剪切板中的文本，供远程控制页「发送剪切板」使用。
func (h *HubService) ReadClipboard() string {
	return readClipboardText()
}

// WriteClipboard 把远端剪切板文本写入本机剪切板。
func (h *HubService) WriteClipboard(text string) error {
	return writeClipboardText(text)
}

func (h *HubService) readLoop(ctx context.Context, conn *websocket.Conn) {
	defer conn.Close(websocket.StatusNormalClosure, "")

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			h.mu.Lock()
			current := h.conn == conn
			if current {
				h.conn = nil
				h.cancel = nil
			}
			h.mu.Unlock()
			if current {
				h.emit("hub:disconnected", map[string]string{"reason": err.Error()})
			}
			return
		}

		var msg node.Message
		if json.Unmarshal(data, &msg) != nil {
			continue
		}

		switch msg.Type {
		case node.TypeDevices:
			h.mu.Lock()
			h.devices = msg.Devices
			h.mu.Unlock()
			h.emit("hub:devices", msg.Devices)
		case node.TypeIncomingRequest:
			// 被控端不再弹窗确认，收到请求直接同意。
			_ = h.Respond(msg.RequestId, true)
		case node.TypeConnectResult:
			h.emit("hub:connect-result", map[string]any{
				"requestId": msg.RequestId,
				"accepted":  msg.Accepted,
				"wsUrl":     msg.WsUrl,
				"reason":    msg.Reason,
			})
		case node.TypeTunnelRequest:
			// 被控端收到后连回节点，把本机画面送进中转通道。
			h.startScreenSession(msg.WsUrl)
		case node.TypeError:
			h.emit("hub:error", map[string]string{"message": msg.Message})
		}
	}
}

// startScreenSession 在独立协程里共享本机画面，直到连接断开或节点断开。
func (h *HubService) startScreenSession(wsURL string) {
	if wsURL == "" {
		return
	}

	id, err := randomRequestId()
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())

	h.mu.Lock()
	if h.conn == nil {
		h.mu.Unlock()
		cancel()
		return
	}
	h.screens[id] = cancel
	h.mu.Unlock()

	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.screens, id)
			h.mu.Unlock()
			cancel()
		}()

		log.Printf("[screen] session opened: %s", wsURL)
		if err := runScreenSession(ctx, wsURL); err != nil {
			log.Printf("[screen] session ended: %v", err)
		} else {
			log.Printf("[screen] session closed")
		}
	}()
}

func (h *HubService) write(msg node.Message) error {
	h.mu.Lock()
	conn := h.conn
	h.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("尚未连接节点")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	return conn.Write(ctx, websocket.MessageText, data)
}

func (h *HubService) emit(name string, payload any) {
	if h.app != nil {
		h.app.Event.Emit(name, payload)
	}
}

func randomRequestId() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

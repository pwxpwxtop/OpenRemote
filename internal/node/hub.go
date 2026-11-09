package node

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Version 是节点二进制的版本号，由程序入口设置；客户端据此判断是否需要升级节点。
var Version = "dev"

// Hub 是运行在服务器上的中转节点：维护在线设备、转发授权请求，
// 并在控制端与被控端之间双向搬运 H.264 码流与键鼠事件。
type Hub struct {
	publicURL string

	mu       sync.Mutex
	devices  map[string]*client
	requests map[string]*pendingRequest
	tunnels  map[string]*tunnelEntry
}

type client struct {
	id   string
	name string
	os   string
	host string
	conn *websocket.Conn
	// writeMu 保证同一个 WebSocket 上的并发写不会交错。
	writeMu sync.Mutex
}

type pendingRequest struct {
	fromId   string
	targetId string
	host     string
}

// tunnelEntry 表示一次已批准的会话：控制端连 /vnc/{deviceId}?token=，
// 被控端连 /tunnel/{token}，两端就绪后节点在它们之间双向搬运数据。
type tunnelEntry struct {
	targetId string
	expires  time.Time
	control  *websocket.Conn
	agent    *websocket.Conn
	paired   bool
	ready    chan struct{}
}

func NewHub(publicURL string) *Hub {
	return &Hub{
		publicURL: strings.TrimRight(publicURL, "/"),
		devices:   make(map[string]*client),
		requests:  make(map[string]*pendingRequest),
		tunnels:   make(map[string]*tunnelEntry),
	}
}

// Handler 返回节点的 HTTP 路由：/ws（信令）、/vnc/{deviceId}（控制端接入）、/tunnel/{token}（被控端回连）、/health。
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"version": Version,
		})
	})
	mux.HandleFunc("/ws", h.handleWS)
	mux.HandleFunc("/vnc/", h.handleProxy)
	mux.HandleFunc("/tunnel/", h.handleTunnel)
	return mux
}

func (h *Hub) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Wails WebView 的 origin 不是普通 http 页面，跳过 origin 校验。
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()

	_, data, err := conn.Read(ctx)
	if err != nil {
		return
	}
	var first Message
	if json.Unmarshal(data, &first) != nil || first.Type != TypeRegister || first.DeviceId == "" {
		writeMessage(conn, Message{Type: TypeError, Message: "第一条消息必须是 register"})
		return
	}

	cl := &client{
		id:   first.DeviceId,
		name: first.Name,
		os:   first.OS,
		host: r.Host,
		conn: conn,
	}
	if cl.name == "" {
		cl.name = cl.id
	}

	h.register(cl)
	defer h.unregister(cl)
	h.broadcastDevices()

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var msg Message
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		switch msg.Type {
		case TypeConnectRequest:
			h.handleConnectRequest(cl, msg)
		case TypeConnectResponse:
			h.handleConnectResponse(cl, msg)
		}
	}
}

func (h *Hub) register(cl *client) {
	h.mu.Lock()
	if old, ok := h.devices[cl.id]; ok && old != cl {
		_ = old.conn.Close(websocket.StatusNormalClosure, "replaced by a new session")
	}
	h.devices[cl.id] = cl
	h.mu.Unlock()
	log.Printf("[hub] device online: %s (%s)", cl.id, cl.name)
}

func (h *Hub) unregister(cl *client) {
	h.mu.Lock()
	if current, ok := h.devices[cl.id]; ok && current == cl {
		delete(h.devices, cl.id)
	}
	for id, req := range h.requests {
		if req.fromId == cl.id || req.targetId == cl.id {
			delete(h.requests, id)
		}
	}
	h.mu.Unlock()
	log.Printf("[hub] device offline: %s", cl.id)
	h.broadcastDevices()
}

func (h *Hub) broadcastDevices() {
	h.mu.Lock()
	list := make([]DeviceInfo, 0, len(h.devices))
	targets := make([]*client, 0, len(h.devices))
	for _, c := range h.devices {
		list = append(list, DeviceInfo{DeviceId: c.id, Name: c.name, OS: c.os})
		targets = append(targets, c)
	}
	h.mu.Unlock()

	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	for _, c := range targets {
		c.send(Message{Type: TypeDevices, Devices: list})
	}
}

func (h *Hub) handleConnectRequest(from *client, msg Message) {
	requestId := msg.RequestId
	if requestId == "" {
		requestId = randomHex(16)
	}

	h.mu.Lock()
	target, ok := h.devices[msg.TargetId]
	if !ok {
		h.mu.Unlock()
		from.send(Message{Type: TypeConnectResult, RequestId: requestId, Accepted: false, Reason: "offline"})
		return
	}
	h.requests[requestId] = &pendingRequest{
		fromId:   from.id,
		targetId: target.id,
		host:     from.host,
	}
	h.mu.Unlock()

	log.Printf("[hub] connect request %s: %s -> %s", requestId, from.name, target.name)
	target.send(Message{
		Type:      TypeIncomingRequest,
		RequestId: requestId,
		FromId:    from.id,
		FromName:  from.name,
	})
}

func (h *Hub) handleConnectResponse(from *client, msg Message) {
	h.mu.Lock()
	req, ok := h.requests[msg.RequestId]
	if !ok || req.targetId != from.id {
		h.mu.Unlock()
		return
	}
	delete(h.requests, msg.RequestId)
	requester := h.devices[req.fromId]
	h.mu.Unlock()

	if requester == nil {
		return
	}
	if !msg.Accepted {
		requester.send(Message{Type: TypeConnectResult, RequestId: msg.RequestId, Accepted: false, Reason: "rejected"})
		return
	}

	// 控制端得到接入地址后直接连节点；被控端拿到回连地址后开始推送画面。
	token := randomHex(24)
	h.mu.Lock()
	h.expireTunnelsLocked()
	h.tunnels[token] = &tunnelEntry{
		targetId: req.targetId,
		expires:  time.Now().Add(2 * time.Minute),
		ready:    make(chan struct{}),
	}
	h.mu.Unlock()

	requester.send(Message{
		Type:      TypeConnectResult,
		RequestId: msg.RequestId,
		Accepted:  true,
		WsUrl:     h.buildProxyURL(req.host, req.targetId, token),
	})
	from.send(Message{
		Type:      TypeTunnelRequest,
		RequestId: msg.RequestId,
		Token:     token,
		WsUrl:     h.buildTunnelURL(req.host, token),
	})
}

func (h *Hub) wsBase(host string) string {
	base := h.publicURL
	if base == "" {
		base = "http://" + host
	}
	switch {
	case strings.HasPrefix(base, "https://"):
		return "wss://" + strings.TrimPrefix(base, "https://")
	case strings.HasPrefix(base, "http://"):
		return "ws://" + strings.TrimPrefix(base, "http://")
	case strings.HasPrefix(base, "wss://"), strings.HasPrefix(base, "ws://"):
		return base
	default:
		return "ws://" + base
	}
}

func (h *Hub) buildProxyURL(host, deviceId, token string) string {
	return strings.TrimRight(h.wsBase(host), "/") + "/vnc/" + deviceId + "?token=" + token
}

func (h *Hub) buildTunnelURL(host, token string) string {
	return strings.TrimRight(h.wsBase(host), "/") + "/tunnel/" + token
}

func (h *Hub) expireTunnelsLocked() {
	now := time.Now()
	for token, entry := range h.tunnels {
		if now.After(entry.expires) {
			if entry.control != nil {
				_ = entry.control.Close(websocket.StatusNormalClosure, "")
			}
			if entry.agent != nil {
				_ = entry.agent.Close(websocket.StatusNormalClosure, "")
			}
			delete(h.tunnels, token)
		}
	}
}

// handleProxy 是控制端的接入端：校验 token 后等待被控端回连，再开始搬运数据。
func (h *Hub) handleProxy(w http.ResponseWriter, r *http.Request) {
	deviceId := strings.TrimPrefix(r.URL.Path, "/vnc/")
	token := r.URL.Query().Get("token")

	entry := h.takeTunnel(token, deviceId)
	if entry == nil {
		http.Error(w, "invalid or expired token", http.StatusForbidden)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}

	if !h.attach(entry, conn, nil) {
		_ = conn.Close(websocket.StatusInternalError, "tunnel unavailable")
	}
}

// handleTunnel 是被控端回连的入口，成功配对后节点开始双向搬运数据。
func (h *Hub) handleTunnel(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/tunnel/")
	entry := h.takeTunnel(token, "")
	if entry == nil {
		http.Error(w, "invalid or expired token", http.StatusForbidden)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}

	if !h.attach(entry, nil, conn) {
		_ = conn.Close(websocket.StatusInternalError, "session unavailable")
	}
}

func (h *Hub) takeTunnel(token, deviceId string) *tunnelEntry {
	if token == "" {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	entry, ok := h.tunnels[token]
	if !ok {
		return nil
	}
	if (deviceId != "" && entry.targetId != deviceId) || time.Now().After(entry.expires) {
		delete(h.tunnels, token)
		return nil
	}
	return entry
}

// attach 挂上一端的连接；两端齐备后关闭 ready 并启动搬运协程。
// 返回 false 表示等待 30 秒后仍未配对，调用方应关闭自己的连接。
func (h *Hub) attach(entry *tunnelEntry, control, agent *websocket.Conn) bool {
	h.mu.Lock()
	if control != nil {
		entry.control = control
	}
	if agent != nil {
		entry.agent = agent
	}
	paired := false
	if !entry.paired && entry.control != nil && entry.agent != nil {
		entry.paired = true
		paired = true
		for token, candidate := range h.tunnels {
			if candidate == entry {
				delete(h.tunnels, token)
				break
			}
		}
	}
	h.mu.Unlock()

	if paired {
		close(entry.ready)
		go h.bridge(entry.control, entry.agent)
		return true
	}

	select {
	case <-entry.ready:
		return true
	case <-time.After(30 * time.Second):
		h.mu.Lock()
		entry.paired = true
		for token, candidate := range h.tunnels {
			if candidate == entry {
				delete(h.tunnels, token)
				break
			}
		}
		h.mu.Unlock()
		return false
	}
}

// bridge 在控制端与被控端之间双向搬运数据。
// 保留消息类型（文本/二进制），这样键鼠事件与 H.264 帧可以走同一个通道。
func (h *Hub) bridge(control, agent *websocket.Conn) {
	const maxMessage = 16 << 20
	control.SetReadLimit(maxMessage)
	agent.SetReadLimit(maxMessage)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pipe := func(src, dst *websocket.Conn, done chan<- struct{}) {
		for {
			msgType, data, err := src.Read(ctx)
			if err != nil {
				done <- struct{}{}
				return
			}
			writeCtx, stop := context.WithTimeout(ctx, 15*time.Second)
			err = dst.Write(writeCtx, msgType, data)
			stop()
			if err != nil {
				done <- struct{}{}
				return
			}
		}
	}

	done := make(chan struct{}, 2)
	go pipe(control, agent, done)
	go pipe(agent, control, done)
	<-done

	cancel()
	_ = control.Close(websocket.StatusNormalClosure, "")
	_ = agent.Close(websocket.StatusNormalClosure, "")
	log.Printf("[hub] relay session closed")
}

func (c *client) send(msg Message) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	writeMessage(c.conn, msg)
}

func writeMessage(conn *websocket.Conn, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = conn.Write(ctx, websocket.MessageText, data)
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

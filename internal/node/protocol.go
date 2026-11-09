package node

// DeviceInfo 是节点广播给所有客户端的一台在线设备。
type DeviceInfo struct {
	DeviceId string `json:"deviceId"`
	Name     string `json:"name"`
	OS       string `json:"os"`
}

// Message 是 WebSocket 上客户端与节点之间传递的统一消息体。
// 通过 Type 字段区分用途，未使用的字段会被 omitempty 省略。
type Message struct {
	Type string `json:"type"`

	// register / devices
	DeviceId string       `json:"deviceId,omitempty"`
	Name     string       `json:"name,omitempty"`
	OS       string       `json:"os,omitempty"`
	Devices  []DeviceInfo `json:"devices,omitempty"`

	// connectRequest / incomingRequest
	RequestId string `json:"requestId,omitempty"`
	TargetId  string `json:"targetId,omitempty"`
	FromId    string `json:"fromId,omitempty"`
	FromName  string `json:"fromName,omitempty"`

	// connectResponse / connectResult
	Accepted bool   `json:"accepted,omitempty"`
	WsUrl    string `json:"wsUrl,omitempty"`
	Reason   string `json:"reason,omitempty"`

	// tunnelRequest：被控端收到后连回节点，把 H.264 画面流送进中转通道
	Token string `json:"token,omitempty"`

	// error
	Message string `json:"message,omitempty"`
}

const (
	TypeRegister        = "register"
	TypeDevices         = "devices"
	TypeConnectRequest  = "connectRequest"
	TypeIncomingRequest = "incomingRequest"
	TypeConnectResponse = "connectResponse"
	TypeConnectResult   = "connectResult"
	TypeTunnelRequest   = "tunnelRequest"
	TypeError           = "error"
)

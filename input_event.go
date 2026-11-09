package main

// inputEvent 是控制端通过数据通道发来的键鼠事件。
// X/Y 是相对画面左上角的比例（0~1），由各平台换算成绝对坐标。
// stream 事件用于控制端调整编码分辨率、帧率与码率；
// file 事件用于文件传输；clipboard 事件用于把控制端剪切板内容写入被控端。
type inputEvent struct {
	Event  string  `json:"event"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Button int     `json:"button"`
	Delta  int     `json:"delta"`
	Vk     int     `json:"vk"`
	// Code 是浏览器物理键位（如 KeyA、ArrowUp），用于映射扫描码，
	// 让被控端按自己的键盘布局接收按键。
	Code     string `json:"code"`
	MaxWidth int    `json:"maxWidth"`
	Bitrate  int    `json:"bitrate"`
	Fps      int    `json:"fps"`
	Action   string `json:"action"`
	ID       uint32 `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Text     string `json:"text"`
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

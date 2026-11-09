//go:build linux

package main

import "changeme/internal/x11"

// handleInput 通过 XTEST 扩展把键鼠事件注入到当前 X11 会话。
func handleInput(ev inputEvent, vw, vh int) {
	_ = x11.Handle(x11.Event{
		Event:  ev.Event,
		X:      ev.X,
		Y:      ev.Y,
		Button: ev.Button,
		Delta:  ev.Delta,
		Vk:     ev.Vk,
	}, vw, vh)
}

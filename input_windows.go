//go:build windows

package main

import (
	"syscall"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")

	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procSetCursorPos     = user32.NewProc("SetCursorPos")
	procMouseEvent       = user32.NewProc("mouse_event")
)

const (
	smXVirtualScreen  = 76
	smYVirtualScreen  = 77
	smCXVirtualScreen = 78
	smCYVirtualScreen = 79

	mouseLeftDown   = 0x0002
	mouseLeftUp     = 0x0004
	mouseRightDown  = 0x0008
	mouseRightUp    = 0x0010
	mouseMiddleDown = 0x0020
	mouseMiddleUp   = 0x0040
	mouseWheel      = 0x0800

	keyEventKeyUp = 0x0002
)

// virtualScreen 返回 Windows 虚拟桌面（多显示器合并）的原点与尺寸。
func virtualScreen() (int, int, int, int) {
	x, _, _ := procGetSystemMetrics.Call(smXVirtualScreen)
	y, _, _ := procGetSystemMetrics.Call(smYVirtualScreen)
	w, _, _ := procGetSystemMetrics.Call(smCXVirtualScreen)
	h, _, _ := procGetSystemMetrics.Call(smCYVirtualScreen)
	return int(int32(x)), int(int32(y)), int(int32(w)), int(int32(h))
}

func handleInput(ev inputEvent, vw, vh int) {
	vx, vy, _, _ := virtualScreen()

	switch ev.Event {
	case "move":
		procSetCursorPos.Call(
			uintptr(vx+int(clamp01(ev.X)*float64(vw))),
			uintptr(vy+int(clamp01(ev.Y)*float64(vh))),
		)
	case "down":
		procMouseEvent.Call(uintptr(buttonFlag(ev.Button, true)), 0, 0, 0, 0)
	case "up":
		procMouseEvent.Call(uintptr(buttonFlag(ev.Button, false)), 0, 0, 0, 0)
	case "wheel":
		procMouseEvent.Call(uintptr(mouseWheel), 0, 0, uintptr(ev.Delta), 0)
	case "keydown":
		injectKey(ev.Code, ev.Vk, true)
	case "keyup":
		injectKey(ev.Code, ev.Vk, false)
	}
}

func buttonFlag(button int, down bool) int {
	switch button {
	case 1:
		if down {
			return mouseMiddleDown
		}
		return mouseMiddleUp
	case 2:
		if down {
			return mouseRightDown
		}
		return mouseRightUp
	default:
		if down {
			return mouseLeftDown
		}
		return mouseLeftUp
	}
}

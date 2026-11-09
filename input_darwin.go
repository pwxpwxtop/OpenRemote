//go:build darwin

package main

/*
#cgo LDFLAGS: -framework CoreGraphics -framework ApplicationServices
#include <ApplicationServices/ApplicationServices.h>
*/
import "C"

import "unsafe"

// handleInput 通过 CoreGraphics 事件注入键鼠操作。
// 首次使用时 macOS 会要求为 OpenRemote 开启「辅助功能」权限，否则事件会被丢弃。
func handleInput(ev inputEvent, vw, vh int) {
	switch ev.Event {
	case "move":
		postMouse(C.kCGEventMouseMoved, C.kCGMouseButtonLeft, ev.X, ev.Y, vw, vh)
	case "down", "up":
		down := ev.Event == "down"
		switch ev.Button {
		case 1:
			eventType := C.CGEventType(C.kCGEventLeftMouseUp)
			if down {
				eventType = C.kCGEventLeftMouseDown
			}
			postMouse(eventType, C.kCGMouseButtonLeft, ev.X, ev.Y, vw, vh)
		case 2:
			eventType := C.CGEventType(C.kCGEventRightMouseUp)
			if down {
				eventType = C.kCGEventRightMouseDown
			}
			postMouse(eventType, C.kCGMouseButtonRight, ev.X, ev.Y, vw, vh)
		default:
			eventType := C.CGEventType(C.kCGEventOtherMouseUp)
			if down {
				eventType = C.kCGEventOtherMouseDown
			}
			postMouse(eventType, C.kCGMouseButtonCenter, ev.X, ev.Y, vw, vh)
		}
	case "wheel":
		lines := ev.Delta / 120
		if lines == 0 {
			switch {
			case ev.Delta > 0:
				lines = 1
			case ev.Delta < 0:
				lines = -1
			default:
				return
			}
		}
		if lines > 10 {
			lines = 10
		} else if lines < -10 {
			lines = -10
		}
		event := C.CGEventCreateScrollWheelEvent(nil, C.kCGScrollEventUnitLine, 1, C.int32_t(lines))
		post(event)
	case "keydown", "keyup":
		code, ok := vkToMacKeycode(ev.Vk)
		if !ok {
			return
		}
		event := C.CGEventCreateKeyboardEvent(nil, C.CGKeyCode(code), C.bool(ev.Event == "keydown"))
		post(event)
	}
}

func postMouse(eventType C.CGEventType, button C.CGMouseButton, x, y float64, vw, vh int) {
	ox, oy, _, _ := virtualScreen()
	px := C.CGFloat(ox) + C.CGFloat(clamp01(x)*float64(vw))
	py := C.CGFloat(oy) + C.CGFloat(clamp01(y)*float64(vh))
	event := C.CGEventCreateMouseEvent(nil, eventType, C.CGPointMake(px, py), button)
	post(event)
}

// post 发送事件并释放，避免每次键鼠操作都泄漏一个 CGEvent。
func post(event C.CGEventRef) {
	if event == nil {
		return
	}
	C.CGEventPost(C.kCGHIDEventTap, event)
	C.CFRelease(C.CFTypeRef(unsafe.Pointer(event)))
}

// vkToMacKeycode 把 Windows 虚拟键码映射成 macOS 的虚拟键码。
func vkToMacKeycode(vk int) (int, bool) {
	keycodes := map[int]int{
		0x41: 0x00, // A
		0x53: 0x01, // S
		0x44: 0x02, // D
		0x46: 0x03, // F
		0x48: 0x04, // H
		0x47: 0x05, // G
		0x5A: 0x06, // Z
		0x58: 0x07, // X
		0x43: 0x08, // C
		0x56: 0x09, // V
		0x42: 0x0B, // B
		0x51: 0x0C, // Q
		0x57: 0x0D, // W
		0x45: 0x0E, // E
		0x52: 0x0F, // R
		0x59: 0x10, // Y
		0x54: 0x11, // T
		0x31: 0x12, // 1
		0x32: 0x13, // 2
		0x33: 0x14, // 3
		0x34: 0x15, // 4
		0x36: 0x16, // 6
		0x35: 0x17, // 5
		0xBB: 0x18, // =
		0x39: 0x19, // 9
		0x37: 0x1A, // 7
		0xBD: 0x1B, // -
		0x38: 0x1C, // 8
		0x30: 0x1D, // 0
		0xDD: 0x1E, // ]
		0x4F: 0x1F, // O
		0x55: 0x20, // U
		0xDB: 0x21, // [
		0x49: 0x22, // I
		0x50: 0x23, // P
		0x0D: 0x24, // Return
		0x4C: 0x25, // L
		0x4A: 0x26, // J
		0xDE: 0x27, // '
		0x4B: 0x28, // K
		0xBA: 0x29, // ;
		0xDC: 0x2A, // \
		0xBC: 0x2B, // ,
		0xBF: 0x2C, // /
		0x4E: 0x2D, // N
		0x4D: 0x2E, // M
		0xBE: 0x2F, // .
		0x09: 0x30, // Tab
		0x20: 0x31, // Space
		0xC0: 0x32, // `
		0x08: 0x33, // Backspace
		0x1B: 0x35, // Esc
		0x5B: 0x37, // Command
		0xA0: 0x38, // LShift
		0x10: 0x38, // Shift
		0x14: 0x39, // CapsLock
		0xA4: 0x3A, // LAlt
		0x12: 0x3A, // Alt
		0xA2: 0x3B, // LCtrl
		0x11: 0x3B, // Ctrl
		0xA1: 0x3C, // RShift
		0xA5: 0x3D, // RAlt
		0xA3: 0x3E, // RCtrl
		0x60: 0x52, // 小键盘 0
		0x61: 0x53,
		0x62: 0x54,
		0x63: 0x55,
		0x64: 0x56,
		0x65: 0x57,
		0x66: 0x58,
		0x67: 0x59,
		0x68: 0x5B,
		0x69: 0x5C,
		0x6A: 0x43, // 小键盘 *
		0x6B: 0x45, // 小键盘 +
		0x6D: 0x4E, // 小键盘 -
		0x6E: 0x41, // 小键盘 .
		0x6F: 0x4B, // 小键盘 /
		0x90: 0x47, // NumLock
		0x70: 0x7A, // F1
		0x71: 0x78, // F2
		0x72: 0x63, // F3
		0x73: 0x76, // F4
		0x74: 0x60, // F5
		0x75: 0x61, // F6
		0x76: 0x62, // F7
		0x77: 0x64, // F8
		0x78: 0x65, // F9
		0x79: 0x6D, // F10
		0x7A: 0x67, // F11
		0x7B: 0x6F, // F12
		0x25: 0x7B, // Left
		0x27: 0x7C, // Right
		0x28: 0x7D, // Down
		0x26: 0x7E, // Up
		0x24: 0x73, // Home
		0x23: 0x77, // End
		0x21: 0x74, // PageUp
		0x22: 0x79, // PageDown
		0x2E: 0x75, // Delete
		0x2D: 0x72, // Insert
	}
	code, ok := keycodes[vk]
	return code, ok
}

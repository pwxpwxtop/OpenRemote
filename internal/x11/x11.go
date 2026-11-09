// Package x11 用纯 Go 实现 Linux/X11 下的屏幕尺寸查询与键鼠注入（XTEST）。
package x11

import (
	"fmt"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/jezek/xgb/xtest"
)

const (
	keyPress      = 2
	keyRelease    = 3
	buttonPress   = 4
	buttonRelease = 5
	motionNotify  = 6

	// 一次滚轮事件在 Windows 下是 120，X11 下每次按键滚一格。
	wheelStep = 120
)

// Event 与客户端下发的键鼠事件保持一致（X/Y 为画面内的比例）。
type Event struct {
	Event  string
	X      float64
	Y      float64
	Button int
	Delta  int
	Vk     int
}

var (
	connOnce sync.Once
	conn     *xgb.Conn
	setup    *xproto.SetupInfo
	rootWin  xproto.Window
	connErr  error

	keymapOnce  sync.Mutex
	keymapReady bool
	keysymToKey = map[uint32]xproto.Keycode{}
)

func connection() (*xgb.Conn, xproto.Window, error) {
	connOnce.Do(func() {
		c, err := xgb.NewConn()
		if err != nil {
			connErr = fmt.Errorf("连接 X11 失败: %w", err)
			return
		}
		if err := xtest.Init(c); err != nil {
			c.Close()
			connErr = fmt.Errorf("X11 缺少 XTEST 扩展: %w", err)
			return
		}
		s := xproto.Setup(c)
		conn, setup, rootWin = c, s, s.DefaultScreen(c).Root
	})
	return conn, rootWin, connErr
}

// Bounds 返回根窗口的尺寸，即整个 X11 虚拟桌面（多显示器合并）。
func Bounds() (int, int, int, int, error) {
	c, root, err := connection()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	geo, err := xproto.GetGeometry(c, xproto.Drawable(root)).Reply()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return 0, 0, int(geo.Width), int(geo.Height), nil
}

// Handle 把一条键鼠事件注入到当前 X11 会话。
func Handle(ev Event, screenW, screenH int) error {
	c, root, err := connection()
	if err != nil {
		return err
	}

	switch ev.Event {
	case "move":
		x := int16(clamp01(ev.X) * float64(screenW))
		y := int16(clamp01(ev.Y) * float64(screenH))
		xtest.FakeInput(c, motionNotify, 0, 0, root, x, y, 0)
		return nil
	case "down", "up":
		button := x11Button(ev.Button)
		if ev.Event == "down" {
			xtest.FakeInput(c, buttonPress, button, 0, root, 0, 0, 0)
		} else {
			xtest.FakeInput(c, buttonRelease, button, 0, root, 0, 0, 0)
		}
		return nil
	case "wheel":
		button := byte(5) // 向下滚
		if ev.Delta > 0 {
			button = 4 // 向上滚
		}
		steps := ev.Delta / wheelStep
		if steps < 0 {
			steps = -steps
		}
		if steps == 0 {
			steps = 1
		}
		if steps > 3 {
			steps = 3
		}
		for i := 0; i < steps; i++ {
			xtest.FakeInput(c, buttonPress, button, 0, root, 0, 0, 0)
			xtest.FakeInput(c, buttonRelease, button, 0, root, 0, 0, 0)
		}
		return nil
	case "keydown", "keyup":
		if ev.Vk <= 0 {
			return nil
		}
		keysym, ok := vkToKeysym(ev.Vk)
		if !ok {
			return nil
		}
		keycode, ok := keycodeForKeysym(c, keysym)
		if !ok {
			return nil
		}
		typ := byte(keyPress)
		if ev.Event == "keyup" {
			typ = keyRelease
		}
		xtest.FakeInput(c, typ, byte(keycode), 0, root, 0, 0, 0)
		return nil
	}
	return nil
}

// x11Button 把 Windows 按键（0 左、1 中、2 右）换成 X11 按键号。
func x11Button(button int) byte {
	switch button {
	case 1:
		return 2
	case 2:
		return 3
	default:
		return 1
	}
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

// keycodeForKeysym 查询键盘映射，把 keysym 转成当前布局下的 keycode。
func keycodeForKeysym(c *xgb.Conn, keysym uint32) (xproto.Keycode, bool) {
	keymapOnce.Lock()
	defer keymapOnce.Unlock()

	if !keymapReady {
		min, max := setup.MinKeycode, setup.MaxKeycode
		count := byte(max - min + 1)
		reply, err := xproto.GetKeyboardMapping(c, min, count).Reply()
		if err != nil {
			return 0, false
		}
		per := int(reply.KeysymsPerKeycode)
		for i := 0; i < int(count); i++ {
			index := i * per
			if index >= len(reply.Keysyms) {
				break
			}
			ks := uint32(reply.Keysyms[index])
			if ks == 0 {
				continue
			}
			if _, exists := keysymToKey[ks]; !exists {
				keysymToKey[ks] = min + xproto.Keycode(i)
			}
		}
		keymapReady = true
	}

	keycode, ok := keysymToKey[keysym]
	return keycode, ok
}

// vkToKeysym 把 Windows 虚拟键码映射成 X11 keysym（未按 Shift 的那一列）。
func vkToKeysym(vk int) (uint32, bool) {
	switch {
	case vk >= 0x30 && vk <= 0x39: // 0-9
		return uint32('0' + (vk - 0x30)), true
	case vk >= 0x41 && vk <= 0x5A: // A-Z
		return uint32('a' + (vk - 0x41)), true
	case vk >= 0x60 && vk <= 0x69: // 小键盘 0-9
		return 0xffb0 + uint32(vk-0x60), true
	case vk >= 0x70 && vk <= 0x7B: // F1-F12
		return 0xffbe + uint32(vk-0x70), true
	}

	special := map[int]uint32{
		0x08: 0xff08, // Backspace
		0x09: 0xff09, // Tab
		0x0D: 0xff0d, // Enter
		0x13: 0xff13, // Pause
		0x14: 0xffe5, // CapsLock
		0x1B: 0xff1b, // Esc
		0x20: 0x0020, // Space
		0x21: 0xff55, // PageUp
		0x22: 0xff56, // PageDown
		0x23: 0xff57, // End
		0x24: 0xff50, // Home
		0x25: 0xff51, // Left
		0x26: 0xff52, // Up
		0x27: 0xff53, // Right
		0x28: 0xff54, // Down
		0x2C: 0xff61, // PrintScreen
		0x2D: 0xff63, // Insert
		0x2E: 0xffff, // Delete
		0x5B: 0xffeb, // Win
		0x5C: 0xffec, // Win
		0x5D: 0xff67, // Menu
		0x6A: 0xffaa, // 小键盘 *
		0x6B: 0xffab, // 小键盘 +
		0x6D: 0xffad, // 小键盘 -
		0x6E: 0xffae, // 小键盘 .
		0x6F: 0xffaf, // 小键盘 /
		0x90: 0xff7f, // NumLock
		0x91: 0xff14, // ScrollLock
		0xA0: 0xffe1, // LShift
		0xA1: 0xffe2, // RShift
		0xA2: 0xffe3, // LCtrl
		0xA3: 0xffe4, // RCtrl
		0xA4: 0xffe9, // LAlt
		0xA5: 0xffea, // RAlt
		0x10: 0xffe1,
		0x11: 0xffe3,
		0x12: 0xffe9,
		// OEM 按键取未按 Shift 时的符号。
		0xBA: 0x003b, // ;
		0xBB: 0x003d, // =
		0xBC: 0x002c, // ,
		0xBD: 0x002d, // -
		0xBE: 0x002e, // .
		0xBF: 0x002f, // /
		0xC0: 0x0060, // `
		0xDB: 0x005b, // [
		0xDC: 0x005c, // \
		0xDD: 0x005d, // ]
		0xDE: 0x0027, // '
	}
	keysym, ok := special[vk]
	return keysym, ok
}

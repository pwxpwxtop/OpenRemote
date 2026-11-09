//go:build linux

package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

var clipboardWriteCommands = [][]string{
	{"wl-copy"},
	{"xclip", "-selection", "clipboard"},
	{"xsel", "--clipboard", "--input"},
}

var clipboardReadCommands = [][]string{
	{"wl-paste", "--no-newline"},
	{"xclip", "-selection", "clipboard", "-o"},
	{"xsel", "--clipboard", "--output"},
}

// readClipboardText 依次尝试 Wayland 与 X11 的剪切板工具。
func readClipboardText() string {
	for _, args := range clipboardReadCommands {
		out, err := exec.Command(args[0], args[1:]...).Output()
		if err == nil {
			return string(out)
		}
	}
	return ""
}

// writeClipboardText 把文本写入当前会话的剪切板。
func writeClipboardText(text string) error {
	for _, args := range clipboardWriteCommands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("没有可用的剪切板工具（需要 wl-copy / xclip / xsel）")
}

// pasteClipboard 写入剪切板后尝试用 xdotool 触发 Ctrl+V。
func pasteClipboard(text string) {
	if err := writeClipboardText(text); err != nil {
		log.Printf("[clipboard] write: %v", err)
		return
	}
	time.Sleep(120 * time.Millisecond)
	if err := exec.Command("xdotool", "key", "--clearmodifiers", "ctrl+v").Run(); err != nil {
		log.Printf("[clipboard] paste: %v", err)
	}
}

//go:build darwin

package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// readClipboardText 读取 macOS 剪切板文本。
func readClipboardText() string {
	out, err := exec.Command("pbpaste").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// writeClipboardText 把文本写入 macOS 剪切板。
func writeClipboardText(text string) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("写入剪切板失败: %w", err)
	}
	return nil
}

// pasteClipboard 写入剪切板后，通过 System Events 触发粘贴。
func pasteClipboard(text string) {
	if err := writeClipboardText(text); err != nil {
		log.Printf("[clipboard] write: %v", err)
		return
	}
	time.Sleep(120 * time.Millisecond)
	cmd := exec.Command("osascript", "-e", `tell application "System Events" to keystroke "v" using command down`)
	if err := cmd.Run(); err != nil {
		log.Printf("[clipboard] paste: %v", err)
	}
}

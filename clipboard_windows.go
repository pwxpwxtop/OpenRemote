//go:build windows

package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// readClipboardText 读取 Windows 剪切板中的文本。
// 通过 PowerShell 读取，避免直接操作 Win32 内存句柄。
func readClipboardText() string {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		"[Console]::OutputEncoding=[Text.Encoding]::UTF8; Get-Clipboard -Raw")
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\r\n")
}

// writeClipboardText 把文本写入 Windows 剪切板。
func writeClipboardText(text string) error {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		"[Console]::InputEncoding=[Text.Encoding]::UTF8; Set-Clipboard -Value ([Console]::In.ReadToEnd())")
	cmd.Stdin = strings.NewReader(text)
	hideWindow(cmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("写入剪切板失败: %w", err)
	}
	return nil
}

// pasteClipboard 把文本写入被控端剪切板，并模拟 Ctrl+V 粘贴到当前焦点窗口。
func pasteClipboard(text string) {
	if err := writeClipboardText(text); err != nil {
		log.Printf("[clipboard] write: %v", err)
		return
	}
	time.Sleep(150 * time.Millisecond)

	injectKey("ControlLeft", 17, true)
	injectKey("KeyV", 86, true)
	time.Sleep(30 * time.Millisecond)
	injectKey("KeyV", 86, false)
	injectKey("ControlLeft", 17, false)
}

//go:build linux

package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"changeme/internal/x11"
)

// virtualScreen 返回 X11 根窗口（整个虚拟桌面）的原点与尺寸。
func virtualScreen() (int, int, int, int) {
	x, y, w, h, err := x11.Bounds()
	if err != nil {
		log.Printf("[x11] %v", err)
		return 0, 0, 0, 0
	}
	return x, y, w, h
}

// captureArgs 返回 Linux 上 x11grab 采集桌面所需的输入参数。
// 仅支持 X11 会话；Wayland 下需要桌面门户，暂不处理。
func captureArgs(fps, vx, vy, vw, vh int) []string {
	args := []string{"-f", "x11grab", "-framerate", strconv.Itoa(fps), "-draw_mouse", "1"}
	if vw > 0 && vh > 0 {
		args = append(args, "-video_size", fmt.Sprintf("%dx%d", vw, vh))
	}
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0.0"
	}
	return append(args, "-i", fmt.Sprintf("%s+%d,%d", display, vx, vy))
}

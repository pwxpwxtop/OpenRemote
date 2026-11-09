//go:build windows

package main

import (
	"fmt"
	"strconv"
)

// captureArgs 返回 Windows 上 gdigrab 采集桌面所需的输入参数。
func captureArgs(fps, vx, vy, vw, vh int) []string {
	args := []string{"-f", "gdigrab", "-framerate", strconv.Itoa(fps)}
	if vw > 0 && vh > 0 {
		args = append(args,
			"-offset_x", strconv.Itoa(vx),
			"-offset_y", strconv.Itoa(vy),
			"-video_size", fmt.Sprintf("%dx%d", vw, vh),
		)
	}
	return append(args, "-i", "desktop")
}

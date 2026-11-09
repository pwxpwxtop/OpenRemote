//go:build linux || darwin

package main

import (
	"os/exec"
	"runtime"
)

const ffmpegName = "ffmpeg"

// hideWindow 在 Linux/macOS 上没有对应的窗口概念，保持空实现。
func hideWindow(cmd *exec.Cmd) {}

// bindToJob 只在 Windows 上用 Job Object 管理子进程，这里无需处理。
func bindToJob(cmd *exec.Cmd) {}

// encoderCandidates 返回当前平台可尝试的编码器，按性能从高到低排列。
func encoderCandidates(codec string) []string {
	if runtime.GOOS == "darwin" {
		if codec == "h264" {
			return []string{"h264_videotoolbox", "libx264"}
		}
		return []string{"hevc_videotoolbox", "libx265"}
	}
	if codec == "h264" {
		return []string{"h264_nvenc", "h264_qsv", "libx264"}
	}
	return []string{"hevc_nvenc", "hevc_qsv", "libx265"}
}

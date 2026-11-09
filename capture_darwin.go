//go:build darwin

package main

/*
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>
*/
import "C"

import (
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
)

// virtualScreen 返回主显示器的原点与尺寸（单位是逻辑点，CGEvent 也使用同一坐标系）。
func virtualScreen() (int, int, int, int) {
	bounds := C.CGDisplayBounds(C.CGMainDisplayID())
	return int(bounds.origin.x), int(bounds.origin.y), int(bounds.size.width), int(bounds.size.height)
}

var (
	screenDeviceOnce sync.Once
	screenDeviceArg  = "1:none"

	screenZeroPattern = regexp.MustCompile(`\[(\d+)\]\s+Capture screen 0\b`)
	screenAnyPattern  = regexp.MustCompile(`\[(\d+)\]\s+Capture screen`)
)

// captureArgs 返回 macOS 上 avfoundation 采集主屏幕所需的输入参数。
func captureArgs(fps, vx, vy, vw, vh int) []string {
	return []string{
		"-f", "avfoundation",
		"-framerate", strconv.Itoa(fps),
		"-capture_cursor", "1",
		"-i", screenDevice(),
	}
}

// screenDevice 探测 avfoundation 里屏幕采集设备的序号，默认回退到 1:none。
func screenDevice() string {
	screenDeviceOnce.Do(func() {
		ffmpeg, err := locateFFmpeg()
		if err != nil {
			return
		}
		out, _ := exec.Command(ffmpeg,
			"-hide_banner", "-f", "avfoundation", "-list_devices", "true", "-i", "",
		).CombinedOutput()
		if index := parseScreenIndex(string(out)); index >= 0 {
			screenDeviceArg = strconv.Itoa(index) + ":none"
		}
		log.Printf("[screen] avfoundation screen device: %s", screenDeviceArg)
	})
	return screenDeviceArg
}

func parseScreenIndex(text string) int {
	for _, pattern := range []*regexp.Regexp{screenZeroPattern, screenAnyPattern} {
		match := pattern.FindStringSubmatch(text)
		if len(match) != 2 {
			continue
		}
		if index, err := strconv.Atoi(match[1]); err == nil {
			return index
		}
	}
	return -1
}

//go:build windows

package main

import (
	"log"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"
)

const (
	ffmpegName     = "ffmpeg.exe"
	createNoWindow = 0x08000000
)

// hideWindow 让 ffmpeg 子进程不弹出控制台窗口。
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
}

// encoderCandidates 返回当前平台可尝试的编码器，按性能从高到低排列。
func encoderCandidates(codec string) []string {
	if codec == "h264" {
		return []string{"h264_nvenc", "h264_qsv", "h264_amf", "libx264"}
	}
	return []string{"hevc_nvenc", "hevc_qsv", "hevc_amf", "libx265"}
}

var (
	jobHandle     syscall.Handle
	jobHandleOnce sync.Once
)

// bindToJob 把子进程加入 Job Object，客户端异常退出时系统会一并结束 ffmpeg，
// 避免残留进程持续采集屏幕。
func bindToJob(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	jobHandleOnce.Do(func() {
		handle, err := createKillOnCloseJob()
		if err != nil {
			log.Printf("[screen] create job object: %v", err)
			return
		}
		jobHandle = handle
	})
	if jobHandle == 0 {
		return
	}
	_ = assignProcessToJob(jobHandle, cmd.Process.Pid)
}

var (
	modkernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW         = modkernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = modkernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = modkernel32.NewProc("AssignProcessToJobObject")
	procOpenProcess              = modkernel32.NewProc("OpenProcess")
)

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                [6]uint64
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

const (
	jobObjectExtendedLimitInformationClass = 9
	jobObjectLimitKillOnJobClose           = 0x2000
	processSetQuota                        = 0x0100
	processTerminate                       = 0x0001
)

func createKillOnCloseJob() (syscall.Handle, error) {
	handle, _, err := procCreateJobObjectW.Call(0, 0)
	if handle == 0 {
		return 0, err
	}

	info := jobObjectExtendedLimitInformation{}
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	if _, _, callErr := procSetInformationJobObject.Call(
		handle,
		uintptr(jobObjectExtendedLimitInformationClass),
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
	); callErr != nil && callErr.Error() != "The operation completed successfully." {
		return 0, callErr
	}
	return syscall.Handle(handle), nil
}

// assignProcessToJob 用进程句柄把子进程加入 Job（PID 不能直接当句柄用）。
func assignProcessToJob(job syscall.Handle, pid int) error {
	processHandle, _, openErr := procOpenProcess.Call(processSetQuota|processTerminate, 0, uintptr(uint32(pid)))
	if processHandle == 0 {
		return openErr
	}
	defer syscall.CloseHandle(syscall.Handle(processHandle))

	result, _, assignErr := procAssignProcessToJobObject.Call(uintptr(job), processHandle)
	if result == 0 {
		return assignErr
	}
	return nil
}

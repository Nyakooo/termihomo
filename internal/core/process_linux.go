package core

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"golang.org/x/sys/unix"
)

func processIdentity(pid int) (string, error) {
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", err
	}
	end := strings.LastIndexByte(string(stat), ')')
	if end < 0 {
		return "", i18n.Message("invalid process record")
	}
	fields := strings.Fields(string(stat[end+1:]))
	if len(fields) < 20 || fields[0] == "Z" || fields[0] == "X" {
		return "", os.ErrProcessDone
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		if runtime.GOOS == "android" && (errors.Is(err, os.ErrPermission) || errors.Is(err, os.ErrNotExist)) {
			return "start-ticks:" + fields[19], nil
		}
		return "", err
	}
	return strings.TrimSpace(string(boot)) + ":" + fields[19], nil
}

// Open the PID descriptor before checking identity. It binds subsequent signals
// to that process even if the numeric PID is later reused.
type managedProcess int

func (state instance) openProcess() (managedProcess, error) {
	// Older Android app sandboxes can send SIGSYS for pidfd syscalls.
	if runtime.GOOS == "android" {
		return -1, state.checkProcessIdentity()
	}
	fd, err := unix.PidfdOpen(state.PID, 0)
	if errors.Is(err, unix.ESRCH) {
		return -1, os.ErrProcessDone
	}
	if err != nil {
		return -1, err
	}
	if err := state.checkProcessIdentity(); err != nil {
		unix.Close(fd)
		return -1, err
	}
	return managedProcess(fd), nil
}

func (state instance) checkProcessIdentity() error {
	identity, err := processIdentity(state.PID)
	if errors.Is(err, os.ErrNotExist) || err == nil && identity != state.Identity {
		return os.ErrProcessDone
	}
	if err == nil && runtime.GOOS == "android" && strings.HasPrefix(identity, "start-ticks:") {
		var uptime unix.Timespec
		if err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &uptime); err != nil {
			return err
		}
		// Sandbox policy can hide boot_id. Reject records from a previous boot
		// before using the process start ticks to identify the managed child.
		if state.Started.IsZero() || time.Since(state.Started) > time.Duration(uptime.Nano())+5*time.Second {
			return os.ErrProcessDone
		}
	}
	return err
}

func (p managedProcess) close() {
	if p >= 0 {
		_ = unix.Close(int(p))
	}
}

func (p managedProcess) stop(state instance) error {
	if runtime.GOOS == "android" && p < 0 {
		return state.stopWithoutPIDFD()
	}
	fd := int(p)
	if err := unix.PidfdSendSignal(fd, unix.SIGTERM, nil, 0); err != nil && !errors.Is(err, unix.ESRCH) {
		return err
	}
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	if count, err := unix.Poll(fds, 4000); err != nil {
		return err
	} else if count == 0 {
		if err := unix.PidfdSendSignal(fd, unix.SIGKILL, nil, 0); err != nil && !errors.Is(err, unix.ESRCH) {
			return err
		}
		if count, err := unix.Poll(fds, 2000); err != nil || count == 0 {
			return i18n.Message("Mihomo did not exit after stop")
		}
	}
	return nil
}

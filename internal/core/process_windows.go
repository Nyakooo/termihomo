package core

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"golang.org/x/sys/windows"
)

type managedProcess windows.Handle

func processIdentity(pid int) (string, error) {
	p, err := openWindowsProcess(pid)
	if err != nil {
		return "", err
	}
	defer p.close()
	return p.identity()
}

func openWindowsProcess(pid int) (managedProcess, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		err = os.ErrProcessDone
	}
	return managedProcess(h), err
}

func (p managedProcess) identity() (string, error) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.Handle(p), &created, &exited, &kernel, &user); err != nil {
		return "", err
	}
	if code, err := windows.WaitForSingleObject(windows.Handle(p), 0); err != nil {
		return "", err
	} else if code == windows.WAIT_OBJECT_0 {
		return "", os.ErrProcessDone
	}
	return strconv.FormatInt(created.Nanoseconds(), 10), nil
}

func (state instance) openProcess() (managedProcess, error) {
	p, err := openWindowsProcess(state.PID)
	if err != nil {
		return 0, err
	}
	identity, err := p.identity()
	if err == nil && identity != state.Identity {
		err = os.ErrProcessDone
	}
	if err != nil {
		p.close()
		return 0, err
	}
	return p, nil
}

func (p managedProcess) close() { _ = windows.CloseHandle(windows.Handle(p)) }

func (p managedProcess) stop(state instance) error {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(state.PID))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil
	}
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	identity, err := managedProcess(handle).identity()
	if errors.Is(err, os.ErrProcessDone) || err == nil && identity != state.Identity {
		return nil
	}
	if err != nil {
		return err
	}
	// Detached Windows processes do not receive console signals. Give Mihomo
	// a chance to remove its TUN routes before terminating the child.
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	_ = state.request(ctx, http.MethodPatch, "/configs", strings.NewReader(`{"tun":{"enable":false}}`), nil)
	cancel()
	if err := windows.TerminateProcess(handle, 0); err != nil {
		if code, _ := windows.WaitForSingleObject(handle, 0); code == windows.WAIT_OBJECT_0 {
			return nil
		}
		return err
	}
	if code, err := windows.WaitForSingleObject(handle, 6000); err != nil {
		return err
	} else if code != windows.WAIT_OBJECT_0 {
		return i18n.Message("Mihomo did not exit after stop")
	}
	return nil
}

func (m Manager) startKernel() (*exec.Cmd, func(bool) error, error) {
	cmd := exec.Command(m.binary(), "-d", m.runDir(), "-f", m.configPath())
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
	cmd.Env = mihomoEnv()
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	return cmd, func(bool) error { return nil }, nil
}

func openProfile(path string) (*os.File, error) {
	return os.Open(path)
}

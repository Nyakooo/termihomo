package core

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"golang.org/x/sys/unix"
)

func processIdentity(pid int) (string, error) {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if errors.Is(err, unix.ESRCH) || errors.Is(err, unix.EIO) || (err == nil && (info.Proc.P_pid != int32(pid) || info.Proc.P_stat == 5)) {
		return "", os.ErrProcessDone
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d", info.Proc.P_starttime.Sec, info.Proc.P_starttime.Usec), nil
}

type managedProcess struct {
	state instance
	queue int
}

func (state instance) openProcess() (managedProcess, error) {
	fd, err := unix.Kqueue()
	if err != nil {
		return managedProcess{}, err
	}
	unix.CloseOnExec(fd)
	p := managedProcess{state: state, queue: fd}
	event := unix.Kevent_t{Ident: uint64(state.PID), Filter: unix.EVFILT_PROC, Flags: unix.EV_ADD | unix.EV_ONESHOT, Fflags: unix.NOTE_EXIT}
	if _, err := unix.Kevent(fd, []unix.Kevent_t{event}, nil, nil); err != nil {
		p.close()
		if errors.Is(err, unix.ESRCH) {
			err = os.ErrProcessDone
		}
		return managedProcess{}, err
	}
	if err := p.check(); err != nil {
		p.close()
		return managedProcess{}, err
	}
	return p, nil
}

func (p managedProcess) check() error {
	identity, err := processIdentity(p.state.PID)
	if err == nil && identity != p.state.Identity {
		return os.ErrProcessDone
	}
	return err
}

func (p managedProcess) close() { _ = unix.Close(p.queue) }

func (p managedProcess) stop(_ instance) error {
	for _, step := range []struct {
		signal unix.Signal
		wait   time.Duration
	}{{unix.SIGTERM, 4 * time.Second}, {unix.SIGKILL, 2 * time.Second}} {
		if err := p.check(); errors.Is(err, os.ErrProcessDone) {
			return nil
		} else if err != nil {
			return err
		}
		if err := unix.Kill(p.state.PID, step.signal); errors.Is(err, unix.ESRCH) {
			return nil
		} else if err != nil {
			return err
		}
		timeout := unix.NsecToTimespec(step.wait.Nanoseconds())
		var events [1]unix.Kevent_t
		if n, err := unix.Kevent(p.queue, nil, events[:], &timeout); err != nil {
			return err
		} else if n > 0 {
			return nil
		}
	}
	return i18n.Message("Mihomo did not exit after stop")
}

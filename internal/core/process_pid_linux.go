package core

import (
	"errors"
	"os"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"golang.org/x/sys/unix"
)

func (state instance) stopWithoutPIDFD() error {
	for _, step := range []struct {
		signal unix.Signal
		wait   time.Duration
	}{{unix.SIGTERM, 4 * time.Second}, {unix.SIGKILL, 2 * time.Second}} {
		if err := state.checkProcessIdentity(); errors.Is(err, os.ErrProcessDone) {
			return nil
		} else if err != nil {
			return err
		}
		if err := unix.Kill(state.PID, step.signal); errors.Is(err, unix.ESRCH) {
			return nil
		} else if err != nil {
			return err
		}
		for deadline := time.Now().Add(step.wait); time.Now().Before(deadline); {
			if err := state.checkProcessIdentity(); errors.Is(err, os.ErrProcessDone) {
				return nil
			} else if err != nil {
				return err
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	return i18n.Message("Mihomo did not exit after stop")
}

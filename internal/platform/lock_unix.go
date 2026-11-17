//go:build linux || darwin

package platform

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

func (p Paths) lockMode(name string, exclusive bool) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(p.Data, name), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0) {
		err = i18n.Message("instance lock must be a private regular file")
	}
	if err == nil {
		flags := syscall.LOCK_SH
		if exclusive {
			flags = syscall.LOCK_EX | syscall.LOCK_NB
		}
		err = syscall.Flock(int(f.Fd()), flags)
		if errors.Is(err, syscall.EWOULDBLOCK) {
			err = ErrInstanceBusy
		}
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

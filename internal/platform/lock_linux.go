package platform

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

// ErrInstanceBusy means another command is modifying the managed instance.
var ErrInstanceBusy = i18n.Message("managed instance is busy")

// LockInstance takes an exclusive lock in the private data directory. Closing
// the returned file releases the lock; the lock file must remain in place.
func (p Paths) LockInstance() (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(p.Data, "instance.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0) {
		err = i18n.Message("instance lock must be a private regular file")
	}
	if err == nil {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
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

package platform

import (
	"os"
	"path/filepath"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

// ErrInstanceBusy means another command is modifying the managed instance.
var ErrInstanceBusy = i18n.Message("managed instance is busy")

// LockInstance takes an exclusive lock in the private data directory. Closing
// the returned file releases the lock; the lock file must remain in place.
func (p Paths) LockInstance() (*os.File, error) {
	return p.lock("instance.lock")
}

func (p Paths) LockServiceProbe() (*os.File, error) {
	return p.lock("service-probe.lock")
}

func (p Paths) LockGeodata() (*os.File, error) {
	return p.lock("geodata.lock")
}

// LockExecutableUpdate serializes replacements of the executable at path.
func LockExecutableUpdate(path string) (*os.File, error) {
	return (Paths{Data: filepath.Dir(path)}).lock(filepath.Base(path) + ".update.lock")
}

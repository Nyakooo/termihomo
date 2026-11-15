//go:build linux || darwin

package platform

import (
	"github.com/Nyakooo/termihomo/internal/i18n"
	"os"
)

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return i18n.Message("application directories must be private directories")
	}
	return nil
}

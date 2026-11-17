//go:build !windows

package update

import (
	"errors"
	"os"
)

func RemoveExecutable(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		return false, err
	}
	if err := os.Remove(path + ".update.lock"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return false, nil
}

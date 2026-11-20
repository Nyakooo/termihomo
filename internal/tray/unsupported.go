//go:build android || (!linux && !windows && (!darwin || !cgo))

package tray

import (
	"os/exec"

	"github.com/Nyakooo/termihomo/internal/platform"
)

func supported() bool                                               { return false }
func HandleWorker([]string, func(platform.Paths) DesktopState) bool { return false }

func workerCommand(executable string, paths platform.Paths) (*exec.Cmd, func(), error) {
	return directWorker(executable, executable, paths), func() {}, nil
}

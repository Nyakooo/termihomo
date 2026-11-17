//go:build android || (!linux && !windows && (!darwin || !cgo))

package tray

import (
	"os/exec"

	"github.com/Nyakooo/termihomo/internal/platform"
)

func supported() bool                                               { return false }
func HandleWorker([]string, func(platform.Paths) DesktopState) bool { return false }

func workerCommand(executable, _ string) (*exec.Cmd, func(), error) {
	return directWorker(executable, executable), func() {}, nil
}

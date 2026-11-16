//go:build android || (!linux && !windows && (!darwin || !cgo))

package tray

import "os/exec"

func supported() bool            { return false }
func HandleWorker([]string) bool { return false }

func workerCommand(executable string) (*exec.Cmd, func(), error) {
	return directWorker(executable), func() {}, nil
}

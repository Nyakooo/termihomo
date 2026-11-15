//go:build linux || darwin

package core

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

func (m Manager) startKernel() (*exec.Cmd, func(bool) error, error) {
	// The child waits for an explicit byte before exec. If this command
	// exits before recording the process identity, EOF makes the child exit.
	readGate, writeGate, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	shell := "/bin/sh"
	if runtime.GOOS == "android" {
		shell = "/system/bin/sh"
	}
	cmd := exec.Command(shell, "-c", `IFS= read -r gate <&3 && [ "$gate" = start ] && exec 3<&- && exec "$@"`,
		"termihomo-start", m.binary(), "-d", m.runDir(), "-f", m.configPath())
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.ExtraFiles = []*os.File{readGate}
	cmd.Env = mihomoEnv()
	if err := cmd.Start(); err != nil {
		readGate.Close()
		writeGate.Close()
		return nil, nil, err
	}
	readGate.Close()
	release := func(start bool) error {
		defer writeGate.Close()
		if start {
			_, err := writeGate.Write([]byte("start\n"))
			return err
		}
		return nil
	}
	return cmd, release, nil
}

func openProfile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
}

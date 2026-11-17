// Package tray runs the desktop status menu independently of the terminal UI.
package tray

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Nyakooo/termihomo/internal/platform"
)

const workerArg = "--tray-worker"
const NodesArg = "--tray-nodes"
const ConsoleArg = "--tray-console"

type InterfaceAction string

const (
	ExitInterface InterfaceAction = "tray-exit"
	OpenNodes     InterfaceAction = "tray-nodes"
)

type Summary struct {
	Active  bool
	Running bool
	CanTest bool
	Lines   [5]string
}

type ReadSummary func(ctx context.Context, test bool, refresh func()) Summary
type DesktopState struct {
	Enabled   func() bool
	Read      ReadSummary
	Pause     func(context.Context) error
	CycleMode func(context.Context) error
	ToggleTUN func(context.Context) error
	ToggleLAN func(context.Context) error
}

func Available() bool { return supported() }

func InterfaceToken(paths platform.Paths, action InterfaceAction) string {
	token, _ := os.ReadFile(filepath.Join(paths.Data, string(action)))
	return string(token)
}

func requestInterfaceAction(paths platform.Paths, action InterfaceAction) error {
	file, err := os.CreateTemp(paths.Data, "."+string(action)+"-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.WriteString(filepath.Base(file.Name()))
	if err := errors.Join(err, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(paths.Data, string(action)))
}

// Shutdown closes the tray and registered interfaces without changing preferences.
func Shutdown(ctx context.Context, paths platform.Paths) error {
	stopped := func() (bool, error) {
		lock, err := paths.LockTray()
		if errors.Is(err, platform.ErrInstanceBusy) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		defer lock.Close()
		open, err := paths.TrayInterfaceOpen()
		return !open, err
	}
	if done, err := stopped(); done || err != nil {
		return err
	}
	if err := requestInterfaceAction(paths, ExitInterface); err != nil {
		return err
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if done, err := stopped(); done || err != nil {
				return err
			}
		}
	}
}

// Start launches a detached worker. The worker takes the tray lock for its lifetime.
func Start(paths platform.Paths) error {
	lock, err := paths.LockTray()
	if errors.Is(err, platform.ErrInstanceBusy) {
		return nil
	}
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		lock.Close()
		return err
	}
	command, cleanup, err := workerCommand(executable, paths.Data)
	lock.Close()
	if err != nil {
		return err
	}
	command.Env = append(command.Environ(), "GH_TOKEN=", "GITHUB_TOKEN=")
	if err := command.Start(); err != nil {
		cleanup()
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}

func directWorker(executable, launcher string) *exec.Cmd {
	return exec.Command(executable, workerArg, launcher)
}

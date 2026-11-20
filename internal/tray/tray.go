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
	Active               bool
	Running              bool
	CanTest              bool
	CanToggleSystemProxy bool
	Lines                [6]string
}

type ReadSummary func(ctx context.Context, test bool, refresh func()) Summary
type DesktopState struct {
	Enabled           func() bool
	Read              ReadSummary
	Pause             func(context.Context) error
	CycleMode         func(context.Context) error
	ToggleTUN         func(context.Context) error
	ToggleLAN         func(context.Context) error
	ToggleSystemProxy func(context.Context) error
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
	command, cleanup, err := workerCommand(executable, paths)
	lock.Close()
	if err != nil {
		return err
	}
	command.Env = workerEnvironment(paths)
	if err := command.Start(); err != nil {
		cleanup()
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}

func directWorker(executable, launcher string, paths platform.Paths) *exec.Cmd {
	return exec.Command(executable, workerArg, launcher, paths.Config, paths.Data, paths.Logs)
}

// workerPaths preserves native and custom directory layouts across processes.
func workerPaths(args []string) (platform.Paths, error) {
	if len(args) != 3 {
		return platform.Paths{}, errors.New("invalid tray worker directories")
	}
	for _, path := range args {
		if !filepath.IsAbs(path) {
			return platform.Paths{}, errors.New("tray worker directories must be absolute")
		}
	}
	return platform.Paths{Config: args[0], Data: args[1], Logs: args[2]}, nil
}

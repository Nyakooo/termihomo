package tray

import (
	"context"
	_ "embed"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"fyne.io/systray"
	"github.com/Nyakooo/termihomo/internal/platform"
	"golang.org/x/sys/windows"
)

//go:embed icon.ico
var icon []byte

//go:embed icon-gray.ico
var grayIcon []byte

var loadedIconPaths = make(map[bool]string, 2)

func supported() bool { return true }

func setIcon(active bool) {
	// Systray caches the Windows icon handle by path after the first load.
	if path, ok := loadedIconPaths[active]; ok {
		_ = systray.SetIconFromFilePath(path)
		return
	}
	data := grayIcon
	if active {
		data = icon
	}
	file, err := os.CreateTemp("", "termihomo-tray-*.ico")
	if err != nil {
		return
	}
	defer os.Remove(file.Name())
	_, err = file.Write(data)
	closeErr := file.Close()
	if err == nil && closeErr == nil && systray.SetIconFromFilePath(file.Name()) == nil {
		loadedIconPaths[active] = file.Name()
	}
}

func workerCommand(executable string, paths platform.Paths) (*exec.Cmd, func(), error) {
	// Use a separate image so a resident tray cannot lock termihomo.exe during updates.
	target := filepath.Join(paths.Data, "tray-worker.exe")
	input, err := os.Open(executable)
	if err != nil {
		return nil, nil, err
	}
	defer input.Close()
	file, err := os.CreateTemp(paths.Data, ".tray-worker-*.exe")
	if err != nil {
		return nil, nil, err
	}
	defer os.Remove(file.Name())
	_, err = io.Copy(file, input)
	if err := errors.Join(err, file.Close()); err != nil {
		return nil, nil, err
	}
	if err := os.Rename(file.Name(), target); err != nil {
		return nil, nil, err
	}
	cmd := directWorker(target, executable, paths)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd, func() {}, nil
}

func cleanupWorker() {}

func openTerminal(executable string, args ...string) error {
	cmd := exec.Command(executable, append([]string{ConsoleArg}, args...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func watchDesktopSession(context.Context) {}

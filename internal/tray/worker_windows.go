package tray

import (
	_ "embed"
	"os"
	"os/exec"
	"syscall"

	"fyne.io/systray"
	"golang.org/x/sys/windows"
)

//go:embed icon.ico
var icon []byte

func supported() bool { return true }

func setIcon() {
	// Give the icon a worker-owned path, then remove it after Windows loads it.
	file, err := os.CreateTemp("", "termihomo-tray-*.ico")
	if err != nil {
		return
	}
	defer os.Remove(file.Name())
	_, err = file.Write(icon)
	closeErr := file.Close()
	if err == nil && closeErr == nil {
		_ = systray.SetIconFromFilePath(file.Name())
	}
}

func workerCommand(executable string) (*exec.Cmd, func(), error) {
	cmd := directWorker(executable)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd, func() {}, nil
}

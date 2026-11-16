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

func workerCommand(executable string) (*exec.Cmd, func(), error) {
	cmd := directWorker(executable)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd, func() {}, nil
}

//go:build !android

package tray

import (
	"context"
	_ "embed"
	"os"
	"os/exec"
	"syscall"
	"time"

	"fyne.io/systray"
	"github.com/Nyakooo/termihomo/internal/i18n"
	"github.com/godbus/dbus/v5"
)

//go:embed icon.png
var icon []byte

//go:embed icon-gray.png
var grayIcon []byte

func supported() bool {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" ||
		(os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("DISPLAY") == "") {
		return false
	}
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer bus.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	const watcher = "org.kde.StatusNotifierWatcher"
	var property dbus.Variant
	err = bus.Object(watcher, "/StatusNotifierWatcher").CallWithContext(ctx,
		"org.freedesktop.DBus.Properties.Get", 0, watcher, "IsStatusNotifierHostRegistered").Store(&property)
	return err == nil && property.Value() == true
}

func setIcon(active bool) {
	if active {
		systray.SetIcon(icon)
	} else {
		systray.SetIcon(grayIcon)
	}
}

func workerCommand(executable, _ string) (*exec.Cmd, func(), error) {
	command := directWorker(executable, executable)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return command, func() {}, nil
}

func cleanupWorker() {}

func openTerminal(executable string, args ...string) error {
	for _, terminal := range []struct {
		name string
		flag string
	}{
		{"xdg-terminal-exec", "--"},
		{"x-terminal-emulator", "-e"},
		{"gnome-terminal", "--"},
		{"konsole", "-e"},
		{"xterm", "-e"},
	} {
		path, err := exec.LookPath(terminal.name)
		if err != nil {
			continue
		}
		cmd := exec.Command(path, append([]string{terminal.flag, executable}, args...)...)
		if err := cmd.Start(); err != nil {
			continue
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case err := <-done:
			timer.Stop()
			if err == nil {
				return nil
			}
		case <-timer.C:
			return nil
		}
	}
	return i18n.Message("Could not open a terminal")
}

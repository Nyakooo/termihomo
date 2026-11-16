//go:build !android

package tray

import (
	"context"
	_ "embed"
	"os"
	"os/exec"
	"time"

	"fyne.io/systray"
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

func workerCommand(executable string) (*exec.Cmd, func(), error) {
	return directWorker(executable), func() {}, nil
}

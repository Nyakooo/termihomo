//go:build cgo

package tray

import (
	_ "embed"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"fyne.io/systray"
)

//go:embed icon.png
var icon []byte

//go:embed icon-gray.png
var grayIcon []byte

const bundleInfo = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>io.github.nyakooo.termihomo.tray</string>
<key>CFBundleName</key><string>Termihomo</string>
<key>CFBundleExecutable</key><string>termihomo</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>LSUIElement</key><true/>
</dict></plist>`

func supported() bool { return true }

func setIcon(active bool) {
	if active {
		systray.SetIcon(icon)
	} else {
		systray.SetIcon(grayIcon)
	}
}

func workerCommand(executable string) (*exec.Cmd, func(), error) {
	dir, err := os.MkdirTemp("", "termihomo-tray-")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	contents := filepath.Join(dir, "Termihomo.app", "Contents")
	target := filepath.Join(contents, "MacOS", "termihomo")
	err = os.MkdirAll(filepath.Dir(target), 0o700)
	if err == nil {
		err = os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(bundleInfo), 0o600)
	}
	if err == nil {
		err = os.Link(executable, target)
		if err != nil {
			err = copyExecutable(executable, target)
		}
	}
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return directWorker(target), cleanup, nil
}

func copyExecutable(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	if closeErr := output.Close(); err == nil {
		err = closeErr
	}
	return err
}

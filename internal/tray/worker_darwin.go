//go:build cgo

package tray

import (
	"context"
	_ "embed"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

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

func workerCommand(executable, _ string) (*exec.Cmd, func(), error) {
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
	command := directWorker(target, executable)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return command, cleanup, nil
}

func cleanupWorker() {
	executable, err := os.Executable()
	if err != nil {
		return
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
	dir := filepath.Dir(bundle)
	parent, parentErr := os.Stat(filepath.Dir(dir))
	temp, tempErr := os.Stat(os.TempDir())
	if parentErr == nil && tempErr == nil && os.SameFile(parent, temp) &&
		filepath.Base(bundle) == "Termihomo.app" && strings.HasPrefix(filepath.Base(dir), "termihomo-tray-") {
		_ = os.RemoveAll(bundle)
		_ = os.Remove(dir)
	}
}

func openTerminal(executable string, args ...string) error {
	file, err := os.CreateTemp("", "termihomo-*.command")
	if err != nil {
		return err
	}
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	}
	command := quote(executable)
	for _, arg := range args {
		command += " " + quote(arg)
	}
	_, err = io.WriteString(file, "#!/bin/sh\n/bin/rm -f -- "+quote(file.Name())+"\nexec "+command+"\n")
	err = errors.Join(err, file.Chmod(0o700), file.Close())
	if err == nil {
		err = exec.Command("/usr/bin/open", "-a", "Terminal", file.Name()).Run()
	}
	if err != nil {
		_ = os.Remove(file.Name())
	}
	return err
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

func watchDesktopSession(context.Context) {}

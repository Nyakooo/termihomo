package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

const bootUnitName = "termihomo.service"

func (m Manager) bootUnitPaths() (unit, link string) {
	dir := filepath.Join(filepath.Dir(m.Paths.Config), "systemd", "user")
	return filepath.Join(dir, bootUnitName), filepath.Join(dir, "default.target.wants", bootUnitName)
}

func (m Manager) startOnBoot() (bool, error) {
	_, link := m.bootUnitPaths()
	_, err := os.Stat(link)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Repair older units that called plain `start`, so boot-only error handling
	// also applies after an upgrade without toggling the startup setting.
	return true, m.writeBootUnit()
}

func (m Manager) setStartOnBoot(ctx context.Context, enabled bool) error {
	if runtime.GOOS == "android" {
		return i18n.Message("Start on boot is unavailable in Termux")
	}
	unit, link := m.bootUnitPaths()
	if !enabled {
		err := os.Remove(link)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		return err
	}
	// Lingering starts the user manager at boot even without a login session.
	output, err := exec.CommandContext(ctx, "loginctl", "--no-ask-password", "enable-linger", strconv.Itoa(os.Getuid())).CombinedOutput()
	if err != nil {
		return i18n.Errorf("Could not enable startup before login: %s", strings.TrimSpace(string(output)+" "+err.Error()))
	}
	if err := m.writeBootUnit(); err != nil {
		return err
	}
	if err := os.Symlink(filepath.Join("..", bootUnitName), link); errors.Is(err, os.ErrExist) {
		target, readErr := os.Readlink(link)
		if readErr == nil && (target == filepath.Join("..", bootUnitName) || target == unit) {
			return nil
		}
		return err
	} else {
		return err
	}
}

func unitArgument(value string) string {
	return strconv.Quote(strings.ReplaceAll(value, "%", "%%"))
}

func (m Manager) removeBootRegistration(ctx context.Context) error {
	if runtime.GOOS == "android" {
		return nil
	}
	if err := m.setStartOnBoot(ctx, false); err != nil {
		return err
	}
	unit, _ := m.bootUnitPaths()
	if err := os.Remove(unit); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (m Manager) writeBootUnit() error {
	unit, _ := m.bootUnitPaths()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	// A oneshot remains active after the CLI exits so systemd keeps its Mihomo child.
	service := fmt.Sprintf(`[Unit]
Description=Termihomo proxy

[Service]
Type=oneshot
RemainAfterExit=yes
Environment=%s %s %s
ExecStart=%s --boot-start %s %s %s

[Install]
WantedBy=default.target
`, unitArgument("XDG_CONFIG_HOME="+filepath.Dir(m.Paths.Config)),
		unitArgument("XDG_DATA_HOME="+filepath.Dir(m.Paths.Data)),
		unitArgument("XDG_STATE_HOME="+filepath.Dir(filepath.Dir(m.Paths.Logs))),
		unitArgument(":"+executable), unitArgument(m.Paths.Config), unitArgument(m.Paths.Data), unitArgument(m.Paths.Logs))
	if old, err := os.ReadFile(unit); err == nil && bytes.Equal(old, []byte(service)) {
		return nil
	}
	return atomicWrite(unit, []byte(service))
}

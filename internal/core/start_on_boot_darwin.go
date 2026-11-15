package core

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

func bootPlistPath() string {
	return fmt.Sprintf("/Library/LaunchDaemons/com.termihomo.user-%d.plist", os.Getuid())
}

func (m Manager) startOnBoot() (bool, error) {
	_, err := os.Stat(bootPlistPath())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (m Manager) setStartOnBoot(ctx context.Context, enabled bool) error {
	target := bootPlistPath()
	if !enabled {
		exists, err := m.startOnBoot()
		if err != nil || !exists {
			return err
		}
		return runMacAdmin(ctx, "/bin/rm -f "+shellQuote(target))
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	current, err := user.Current()
	if err != nil {
		return err
	}
	path := func(s string) string {
		var out bytes.Buffer
		_ = xml.EscapeText(&out, []byte(s))
		return out.String()
	}
	label := strings.TrimSuffix(filepath.Base(target), ".plist")
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>UserName</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>--boot-start</string><string>%s</string><string>%s</string><string>%s</string></array>
<key>RunAtLoad</key><true/>
<key>AbandonProcessGroup</key><true/>
</dict></plist>
`, path(label), path(current.Username), path(exe), path(m.Paths.Config), path(m.Paths.Data), path(m.Paths.Logs))
	command := `set -e
temporary=$(/usr/bin/mktemp /Library/LaunchDaemons/.termihomo.XXXXXXXX)
trap '/bin/rm -f "$temporary"' EXIT
/usr/bin/printf %s ` + shellQuote(body) + ` > "$temporary"
/usr/sbin/chown root:wheel "$temporary"
/bin/chmod 0644 "$temporary"
/bin/mv -f "$temporary" ` + shellQuote(target)
	return runMacAdmin(ctx, command)
}

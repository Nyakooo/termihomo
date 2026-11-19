package core

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"
)

// Desktop login is separate from the pre-login proxy service.
func (m Manager) syncDesktopStartup(enabled bool) error {
	if runtime.GOOS == "android" {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{exe, "--tray-autostart", m.Paths.Config, m.Paths.Data, m.Paths.Logs}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path, body := desktopStartupEntry(runtime.GOOS, args, home, os.Getenv("APPDATA"))
	if path == "" {
		return nil
	}
	if !enabled {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	data := []byte(body)
	if runtime.GOOS == "windows" {
		data = []byte{0xff, 0xfe}
		for _, unit := range utf16.Encode([]rune(body)) {
			data = binary.LittleEndian.AppendUint16(data, unit)
		}
	}
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return atomicWrite(path, data)
}

func desktopStartupEntry(targetOS string, args []string, home, appData string) (path, body string) {
	switch targetOS {
	case "linux":
		path = filepath.Join(filepath.Dir(args[2]), "autostart", "termihomo-tray.desktop")
		quoted := make([]string, len(args))
		for i, a := range args {
			quoted[i] = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", "\\$", "%", "%%").Replace(a) + `"`
		}
		// Desktop entry string escaping is applied after Exec argument escaping.
		command := strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`, "\t", `\t`).Replace(strings.Join(quoted, " "))
		body = "[Desktop Entry]\nType=Application\nName=Termihomo tray\nExec=" + command + "\nTerminal=false\n"
	case "darwin":
		path = filepath.Join(home, "Library", "LaunchAgents", "com.termihomo.tray.plist")
		var arguments strings.Builder
		for _, a := range args {
			var b bytes.Buffer
			_ = xml.EscapeText(&b, []byte(a))
			arguments.WriteString("<string>" + b.String() + "</string>")
		}
		body = `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>Label</key><string>com.termihomo.tray</string><key>ProgramArguments</key><array>` + arguments.String() + `</array><key>RunAtLoad</key><true/><key>AbandonProcessGroup</key><true/><key>LimitLoadToSessionType</key><string>Aqua</string></dict></plist>`
	case "windows":
		base := appData
		if !filepath.IsAbs(base) {
			base = filepath.Join(home, "AppData", "Roaming")
		}
		path = filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "termihomo-tray.vbs")
		quoted := make([]string, len(args))
		for i, a := range args {
			quoted[i] = windowsStartupArgument(a)
		}
		body = fmt.Sprintf("CreateObject(\"WScript.Shell\").Run \"%s\", 0, False\r\n", strings.ReplaceAll(strings.Join(quoted, " "), `"`, `""`))
	default:
		return "", ""
	}
	return path, body
}

func windowsStartupArgument(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, r := range value {
		if r == '\\' {
			slashes++
			continue
		}
		if r == '"' {
			b.WriteString(strings.Repeat(`\`, slashes*2+1))
		} else {
			b.WriteString(strings.Repeat(`\`, slashes))
		}
		slashes = 0
		b.WriteRune(r)
	}
	b.WriteString(strings.Repeat(`\`, slashes*2))
	b.WriteByte('"')
	return b.String()
}

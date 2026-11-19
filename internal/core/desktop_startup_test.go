package core

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Nyakooo/termihomo/internal/platform"
)

func TestDesktopTrayStartupRegistration(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux desktop autostart")
	}
	root := t.TempDir()
	m := Manager{Paths: platform.Paths{Config: filepath.Join(root, "config", "termihomo"), Data: filepath.Join(root, "data", "termihomo"), Logs: filepath.Join(root, "state", "termihomo", "logs")}}
	stub := filepath.Join(root, "loginctl")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+":"+os.Getenv("PATH"))
	if err := m.SetToolSetting(t.Context(), startOnBootKey, "true"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config", "autostart", "termihomo-tray.desktop")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("boot enabled but desktop tray startup missing: %v", err)
	}
	for _, want := range []string{"Type=Application\n", "--tray-autostart", m.Paths.Config, m.Paths.Data, m.Paths.Logs} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("startup entry missing %q: %s", want, body)
		}
	}
	// Upgrades must repair a pre-existing boot service without toggling it.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ToolSettings(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("existing boot registration not migrated: %v", err)
	}
	if err := m.SetToolSetting(t.Context(), startOnBootKey, "false"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("desktop startup not removed: %v", err)
	}
}

func TestDesktopStartupEntries(t *testing.T) {
	args := []string{`/Applications/代理 App/termihomo`, "--tray-autostart", `/home/test/a & b/config/termihomo`, `/home/test/data/termihomo`, `/home/test/state/termihomo/logs`}
	for _, platform := range []string{"linux", "darwin", "windows"} {
		t.Run(platform, func(t *testing.T) {
			path, body := desktopStartupEntry(platform, args, "/home/test", "/home/test/roaming")
			if path == "" || !strings.Contains(body, "--tray-autostart") {
				t.Fatalf("entry: %q, %q", path, body)
			}
			switch platform {
			case "linux":
				if !strings.Contains(body, `Exec="/Applications/代理 App/termihomo" "--tray-autostart"`) || !strings.Contains(path, "autostart") {
					t.Fatal(body)
				}
			case "darwin":
				var plist struct {
					Dict struct {
						Array struct {
							Strings []string `xml:"string"`
						} `xml:"array"`
					} `xml:"dict"`
				}
				if err := xml.Unmarshal([]byte(body), &plist); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(plist.Dict.Array.Strings, args) {
					t.Fatalf("lost launch arguments: %#v", plist)
				}
			case "windows":
				if !strings.Contains(body, `""/Applications/代理 App/termihomo""`) || !strings.HasSuffix(body, ", 0, False\r\n") {
					t.Fatal(body)
				}
			}
		})
	}
}

func TestWindowsStartupArgument(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{`C:\Program Files\代理\`, `"C:\Program Files\代理\\"`},
		{`a"b`, `"a\"b"`},
		{``, `""`},
	} {
		if got := windowsStartupArgument(tc.input); got != tc.want {
			t.Fatalf("quote %q: got %q, want %q", tc.input, got, tc.want)
		}
	}
}

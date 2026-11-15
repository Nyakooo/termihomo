//go:build !android

package core

import (
	"github.com/Nyakooo/termihomo/internal/platform"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartOnBootIsIndependentOfAutoRun(t *testing.T) {
	root := t.TempDir()
	m := Manager{Paths: platform.Paths{
		Config: filepath.Join(root, "config", "termihomo"),
		Data:   filepath.Join(root, "data", "termihomo"),
		Logs:   filepath.Join(root, "state", "termihomo", "logs"),
	}}
	loginctl := filepath.Join(root, "loginctl")
	stub := "#!/bin/sh\n[ \"$1\" = --no-ask-password ] && [ \"$2\" = enable-linger ] && [ \"$3\" = \"$(id -u)\" ] && [ \"$TERMIHOMO_TEST_LINGER\" = yes ]\n"
	if err := os.WriteFile(loginctl, []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+":"+os.Getenv("PATH"))
	t.Setenv("TERMIHOMO_TEST_LINGER", "no")
	set := func(key, value string) {
		t.Helper()
		if err := m.SetToolSetting(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	check := func(boot, auto string) {
		t.Helper()
		values, err := m.ToolSettings()
		if err != nil || values[startOnBootKey] != boot || values["app.auto-run"] != auto {
			t.Fatalf("boot = %q, auto-run = %q, error = %v", values[startOnBootKey], values["app.auto-run"], err)
		}
	}
	set("app.auto-run", "true")
	if err := m.SetToolSetting(t.Context(), startOnBootKey, "true"); err == nil {
		t.Fatal("denied lingering should fail to enable startup")
	}
	check("false", "true")
	t.Setenv("TERMIHOMO_TEST_LINGER", "yes")
	set(startOnBootKey, "true")
	set(startOnBootKey, "true")
	check("true", "true")
	unit, link := m.bootUnitPaths()
	body, err := os.ReadFile(link)
	if err != nil || !strings.Contains(string(body), "RemainAfterExit=yes\n") ||
		!strings.Contains(string(body), " start\n\n[Install]\nWantedBy=default.target\n") {
		t.Fatalf("invalid boot service %s: %q, %v", unit, body, err)
	}
	set("app.auto-run", "false")
	check("true", "false")
	set(startOnBootKey, "")
	check("false", "false")
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("boot dependency should be removed: %v", err)
	}
}

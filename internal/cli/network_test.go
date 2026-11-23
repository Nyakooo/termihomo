package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nyakooo/termihomo/internal/platform"
)

func isolateCLI(t *testing.T) {
	t.Helper()
	t.Setenv("TERMIHOMO_LANG", "en")
	for _, key := range []string{"HOME", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		t.Setenv(key, t.TempDir())
	}
}

func TestNetworkCommandArguments(t *testing.T) {
	for _, args := range [][]string{
		{"down"}, {"tun", "start"}, {"tun", "stop"}, {"tun", "status"}, {"tun", "status", "--json"},
		{"sysproxy", "start"}, {"sysproxy", "stop"}, {"sysproxy", "status", "--json"},
		{"mode", "rule"}, {"mode", "global"}, {"mode", "direct"}, {"mode", "status", "--json"},
	} {
		if !validManagedCommand(args) {
			t.Errorf("valid command rejected: %v", args)
		}
	}
	for _, args := range [][]string{
		nil, {"down", "--json"}, {"tun"}, {"tun", "start", "--json"}, {"sysproxy", "stop", "--json"},
		{"sysproxy", "status", "extra"}, {"mode"}, {"mode", "invalid"}, {"mode", "rule", "--json"},
	} {
		if validManagedCommand(args) {
			t.Errorf("invalid command accepted: %v", args)
		}
	}
}

func TestTUNStatusWithoutInstalledCore(t *testing.T) {
	isolateCLI(t)
	var english string
	for _, lang := range []string{"en", "zh-CN"} {
		var out, diagnostics bytes.Buffer
		if code := runCLI([]string{"--lang", lang, "tun", "status", "--json"}, &out, &diagnostics); code != 0 {
			t.Fatalf("status failed: %s", &diagnostics)
		}
		var status struct {
			State                                  string `json:"state"`
			Available, Enabled, Configured, Paused bool
		}
		if err := json.Unmarshal(out.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		if status.State != "NotInstalled" || status.Available || status.Enabled || status.Configured || status.Paused {
			t.Fatalf("unexpected status: %s", &out)
		}
		if english != "" && english != out.String() {
			t.Fatal("JSON depends on selected language")
		}
		english = out.String()
	}
}

func TestDownIsIdempotentAndPreservesPreferences(t *testing.T) {
	isolateCLI(t)
	var out, diagnostics bytes.Buffer
	if code := runCLI([]string{"init"}, &out, &diagnostics); code != 0 {
		t.Fatal(diagnostics.String())
	}
	// Resolve the same platform-specific directories as the actual command.
	paths, err := platform.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(paths.Config, "overrides.yaml")
	settings := []byte("app.system-tray: true\napp.start-on-boot: true\n")
	if err := os.WriteFile(file, settings, 0600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		diagnostics.Reset()
		if code := runCLI([]string{"down"}, &out, &diagnostics); code != 0 {
			t.Fatalf("down failed: %s", &diagnostics)
		}
	}
	got, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(got, settings) {
		t.Fatalf("preferences changed: %q, %v", got, err)
	}
}

func TestInvalidNetworkFlagsAreRejectedBeforeExecution(t *testing.T) {
	isolateCLI(t)
	for _, args := range [][]string{{"sysproxy", "start", "--yes"}, {"mode", "rule", "--yes"}, {"tun", "stop", "--yes"}} {
		var out, diagnostics bytes.Buffer
		if code := runCLI(args, &out, &diagnostics); code != 2 || !strings.Contains(diagnostics.String(), "unknown command") {
			t.Fatalf("%v: code=%d diagnostics=%s", args, code, &diagnostics)
		}
	}
}

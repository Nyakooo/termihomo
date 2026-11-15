package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runCLI(args []string, stdout, stderr *bytes.Buffer) int {
	return RunWithInput(args, strings.NewReader(""), stdout, stderr)
}

func TestInitIsolatesUserDirectories(t *testing.T) {
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		t.Setenv(key, "relative-path-ignored")
	}
	for range 2 {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("LOCALAPPDATA", home)
		var logs bytes.Buffer
		if code := runCLI([]string{"init"}, &bytes.Buffer{}, &logs); code != 0 {
			t.Fatalf("init exit %d: %s", code, &logs)
		}
		dirs := []string{".config/termihomo", ".local/share/termihomo", ".local/state/termihomo/logs"}
		switch runtime.GOOS {
		case "darwin":
			dirs = []string{"Library/Application Support/termihomo/config", "Library/Application Support/termihomo/data", "Library/Application Support/termihomo/state/logs"}
		case "windows":
			dirs = []string{"termihomo/config", "termihomo/data", "termihomo/state/logs"}
		}
		for _, dir := range dirs {
			info, err := os.Stat(filepath.Join(home, dir))
			if err != nil {
				t.Fatal(err)
			}
			if !info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o700) {
				t.Errorf("%s: expected private directory, got %v", dir, info.Mode())
			}
		}
	}
}

func TestInitFailureDoesNotLogSecrets(t *testing.T) {
	const secret = "test-private-subscription-token"
	t.Setenv("HOME", t.TempDir())
	blocker := filepath.Join(t.TempDir(), secret)
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocker)
	var logs bytes.Buffer
	if code := runCLI([]string{"init"}, &bytes.Buffer{}, &logs); code != 1 {
		t.Fatalf("init exit = %d; want 1", code)
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatal("log leaked the secret")
	}
	var event struct{ Level string }
	if err := json.Unmarshal(logs.Bytes(), &event); err != nil || event.Level != "ERROR" {
		t.Fatalf("expected a structured error event, got %s", &logs)
	}
}

func TestLanguageSelectionAndStatusJSON(t *testing.T) {
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		t.Setenv(key, t.TempDir())
	}
	t.Setenv("TERMIHOMO_LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "zh_CN.UTF-8")

	run := func(wantCode int, args ...string) string {
		t.Helper()
		var out, diagnostics bytes.Buffer
		if code := runCLI(args, &out, &diagnostics); code != wantCode {
			t.Fatalf("%v exited %d: %s", args, code, &diagnostics)
		}
		if wantCode != 0 {
			return diagnostics.String()
		}
		return out.String()
	}
	if got := run(0, "--help"); !strings.Contains(got, "Usage:") {
		t.Fatalf("default should be English regardless of locale: %q", got)
	}
	t.Setenv("TERMIHOMO_LANG", "en")
	if got := run(0, "--help"); !strings.Contains(got, "Usage:") {
		t.Fatalf("environment did not override locale: %q", got)
	}
	if got := run(0, "--lang", "zh-CN", "status"); strings.TrimSpace(got) != "未安装" {
		t.Fatalf("flag did not override environment: %q", got)
	}
	if got := run(1, "--lang", "zh-CN", "profile", "update", "--yes"); !strings.Contains(got, "尚未添加订阅") {
		t.Fatalf("core error was not translated: %q", got)
	}
	if got := run(2, "--lang", "unsupported"); !strings.Contains(got, "--lang en") {
		t.Fatalf("missing language guidance: %q", got)
	}
	english := run(0, "--lang", "en", "status", "--json")
	chinese := run(0, "--lang", "zh-CN", "status", "--json")
	if english != chinese || !strings.Contains(chinese, `"state":"NotInstalled"`) {
		t.Fatalf("localized status JSON changed its contract: en=%q zh=%q", english, chinese)
	}
}

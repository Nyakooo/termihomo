//go:build linux && (amd64 || arm64)

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitIsolatesUserDirectories(t *testing.T) {
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		t.Setenv(key, "relative-path-ignored")
	}
	for range 2 {
		home := t.TempDir()
		t.Setenv("HOME", home)
		var logs bytes.Buffer
		if code := Run([]string{"init"}, &bytes.Buffer{}, &logs); code != 0 {
			t.Fatalf("init exit %d: %s", code, &logs)
		}
		for _, dir := range []string{".config/termihomo", ".local/share/termihomo", ".local/state/termihomo/logs"} {
			info, err := os.Stat(filepath.Join(home, dir))
			if err != nil {
				t.Fatal(err)
			}
			if !info.IsDir() || info.Mode().Perm() != 0o700 {
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
	if code := Run([]string{"init"}, &bytes.Buffer{}, &logs); code != 1 {
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

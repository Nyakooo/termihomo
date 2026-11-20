package tray

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Nyakooo/termihomo/internal/platform"
)

func TestWorkerReceivesExactDirectories(t *testing.T) {
	base := filepath.Join(t.TempDir(), "用户 data", "termihomo")
	paths := platform.Paths{Config: filepath.Join(base, "config"), Data: filepath.Join(base, "data"), Logs: filepath.Join(base, "state", "logs")}
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command, cleanup, err := workerCommand(executable, paths)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	want := []string{workerArg, executable, paths.Config, paths.Data, paths.Logs}
	if !slices.Equal(command.Args[1:], want) {
		t.Fatalf("tray worker lost parent directories: got %q, want %q", command.Args[1:], want)
	}
	restored, err := workerPaths(command.Args[3:])
	if err != nil || restored != paths {
		t.Fatalf("tray worker resolved wrong directories: %+v, %v", restored, err)
	}
}

func TestWorkerEnvironmentPreservesNativeDesktopBases(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Linux intentionally forwards XDG bases")
	}
	original := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", original)
	t.Setenv("XDG_DATA_HOME", original)
	t.Setenv("XDG_STATE_HOME", original)
	paths := platform.Paths{Config: filepath.Join(original, "config"), Data: filepath.Join(original, "data"), Logs: filepath.Join(original, "state", "logs")}
	values := make(map[string]string)
	for _, entry := range workerEnvironment(paths) {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME"} {
		if values[key] != original {
			t.Fatalf("worker changed %s from %q to %q", key, original, values[key])
		}
	}
}

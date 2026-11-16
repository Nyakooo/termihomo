package update

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReplaceRunningWindowsExecutable(t *testing.T) {
	// Run a copy of this test image so the target really is a mapped executable.
	const stagedEnv = "TERMIHOMO_TEST_REPLACE_SOURCE"
	if source := os.Getenv(stagedEnv); source != "" {
		target, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		lock, err := os.Create(target + ".update.lock")
		if err != nil {
			t.Fatal(err)
		}
		lock.Close()
		if err := replaceExecutable(source, target); err != nil && !errors.Is(err, ErrUpdateScheduled) {
			t.Fatal(err)
		}
		return
	}

	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	image, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "running.exe")
	source := filepath.Join(dir, "replacement.exe")
	if err := os.WriteFile(target, image, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("replacement"), 0o755); err != nil {
		t.Fatal(err)
	}

	// The PowerShell worker inherits stderr. CombinedOutput waits for it to
	// close the pipe after the child test process exits and replacement finishes.
	cmd := exec.CommandContext(t.Context(), target, "-test.run=^TestReplaceRunningWindowsExecutable$")
	cmd.Env = append(os.Environ(), stagedEnv+"="+source)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("update child failed: %v\n%s", err, output)
	} else {
		got, err := os.ReadFile(target)
		if err != nil || string(got) != "replacement" {
			t.Fatalf("replacement was not installed: %v\n%s", err, output)
		}
	}
}

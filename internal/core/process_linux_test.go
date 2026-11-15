package core

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestStopWithoutPIDFDChecksProcessIdentity(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	pid := cmd.Process.Pid
	identity, err := processIdentity(pid)
	if err != nil {
		t.Fatal(err)
	}
	state := instance{PID: pid, Identity: identity, Started: time.Now()}
	stale := state
	stale.Identity = "stale"
	if err := stale.stopWithoutPIDFD(); err != nil {
		t.Fatal(err)
	}
	if err := state.checkProcessIdentity(); err != nil {
		t.Fatalf("a mismatched state stopped the child: %v", err)
	}
	if err := state.stopWithoutPIDFD(); err != nil {
		t.Fatal(err)
	}
	if err := state.checkProcessIdentity(); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("verified process is still running: %v", err)
	}
}

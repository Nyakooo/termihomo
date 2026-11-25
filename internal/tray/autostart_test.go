package tray

import (
	"context"
	"github.com/Nyakooo/termihomo/internal/platform"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitForDesktop(t *testing.T) {
	calls := 0
	ready := func() bool { calls++; return calls == 3 }
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if !waitForDesktop(ctx, func() bool { return true }, ready, time.Millisecond) || calls != 3 {
		t.Fatal("did not retry late desktop host")
	}
	calls = 0
	if waitForDesktop(ctx, func() bool { return false }, ready, time.Millisecond) || calls != 0 {
		t.Fatal("disabled tray must not start")
	}
	canceled, stop := context.WithCancel(t.Context())
	stop()
	if waitForDesktop(canceled, func() bool { return true }, func() bool { return false }, time.Millisecond) {
		t.Fatal("started without host")
	}
}

func TestAutostartRestoresNetworkWithTrayDisabled(t *testing.T) {
	root := t.TempDir()
	restored := false
	handled := HandleAutostart([]string{"--tray-autostart", filepath.Join(root, "config"), filepath.Join(root, "data"), filepath.Join(root, "logs")}, func(platform.Paths) DesktopState {
		return DesktopState{
			Enabled:        func() bool { return false },
			RestoreNetwork: func(context.Context) error { restored = true; return nil },
		}
	})
	if !handled || !restored {
		t.Fatal("desktop network restoration depends on tray visibility")
	}
}

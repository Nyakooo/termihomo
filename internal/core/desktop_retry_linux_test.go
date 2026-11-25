//go:build !android

package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDesktopRestoreWaitsForBootLock(t *testing.T) {
	fakeDesktopProxy(t)
	m := networkTestManager(t)
	networkTestController(t, m)
	if err := m.saveNetworkPreference(systemProxyPreference, true); err != nil {
		t.Fatal(err)
	}
	lock, err := m.Paths.LockInstance()
	if err != nil {
		t.Fatal(err)
	}
	timer := time.AfterFunc(20*time.Millisecond, func() { lock.Close() })
	defer func() {
		if timer.Stop() {
			lock.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := m.restoreDesktopNetwork(ctx, 10*time.Millisecond); err != nil {
		t.Fatalf("boot lock was not retried: %v", err)
	}
	if !m.SystemProxy(t.Context()).Owned {
		t.Fatal("desktop proxy was not restored")
	}
}

func TestDesktopRestoreWaitsForSession(t *testing.T) {
	fakeDesktopProxy(t)
	m := networkTestManager(t)
	networkTestController(t, m)
	if err := m.saveNetworkPreference(systemProxyPreference, true); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(os.Getenv("TEST_PROXY_DIR"), "unavailable")
	if err := os.WriteFile(marker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	timer := time.AfterFunc(20*time.Millisecond, func() { os.Remove(marker) })
	defer timer.Stop()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := m.restoreDesktopNetwork(ctx, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if !m.SystemProxy(t.Context()).Owned {
		t.Fatal("desktop session availability was not retried")
	}
}

func TestDesktopRestoreHonorsTimeout(t *testing.T) {
	m := networkTestManager(t)
	if err := m.saveNetworkPreference(systemProxyPreference, true); err != nil {
		t.Fatal(err)
	}
	lock, err := m.Paths.LockInstance()
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := m.restoreDesktopNetwork(ctx, time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("retry did not stop at timeout: %v", err)
	}
}

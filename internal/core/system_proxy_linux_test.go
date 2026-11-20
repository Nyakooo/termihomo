//go:build !android

package core

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Nyakooo/termihomo/internal/platform"
)

func TestSystemProxyRestoresOnlyOwnedSettings(t *testing.T) {
	testSystemProxyRestoresOnlyOwnedSettings(t, "fake-session")
}

func TestSystemProxyWithoutBusEnvironment(t *testing.T) {
	testSystemProxyRestoresOnlyOwnedSettings(t, "")
}

func testSystemProxyRestoresOnlyOwnedSettings(t *testing.T, busAddress string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("XDG_CURRENT_DESKTOP", "ubuntu:GNOME")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", busAddress)
	t.Setenv("TEST_PROXY_DIR", dir)
	script := "#!/bin/sh\npath=\"$TEST_PROXY_DIR/$2.$3\"\ncase $1 in\nlist-recursively) for f in \"$TEST_PROXY_DIR\"/org.gnome.system.proxy.*; do name=${f##*/}; printf '%s %s %s\\n' \"${name%.*}\" \"${name##*.}\" \"$(cat \"$f\")\"; done;;\nget) cat \"$path\";;\nset) printf '%s' \"$4\" > \"$path\";;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "gsettings"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	mode := filepath.Join(dir, proxySchema+".mode")
	for _, setting := range proxySettings {
		if err := os.WriteFile(filepath.Join(dir, setting.schema+"."+setting.key), []byte("0"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setMode := func(value string) {
		t.Helper()
		if err := os.WriteFile(mode, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m := Manager{Paths: platform.Paths{
		Config: filepath.Join(dir, "config"),
		Data:   filepath.Join(dir, "data"),
		Logs:   filepath.Join(dir, "state", "logs"),
	}}
	if err := m.Paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	setMode("'none'")
	if err := os.WriteFile(filepath.Join(dir, proxySchema+".ignore-hosts"), []byte("['localhost']"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := []string{"localhost"}
	if state := m.SystemProxy(ctx); !slices.Equal(state.Exclusions, original) {
		t.Fatalf("expected desktop exclusions, got %+v", state)
	}
	if err := m.enableSystemProxy(ctx, "127.0.0.1:7890", false); err != nil {
		t.Fatal(err)
	}
	if state := m.SystemProxy(ctx); !state.Enabled || !state.Owned {
		t.Fatalf("expected owned proxy, got %+v", state)
	}
	added := []string{"localhost", "*.example.com"}
	edited := []string{"localhost", "*.example.net"}
	if err := m.SetProxyExclusions(ctx, original, added); err != nil {
		t.Fatal(err)
	}
	if err := m.SetProxyExclusions(ctx, original, nil); err == nil {
		t.Fatal("a stale write should not overwrite desktop exclusions")
	}
	if err := m.SetProxyExclusions(ctx, added, edited); err != nil {
		t.Fatal(err)
	}
	if state := m.SystemProxy(ctx); !state.Owned {
		t.Fatalf("editing exclusions should preserve proxy ownership: %+v", state)
	}
	if err := m.disableSystemProxy(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if state := m.SystemProxy(ctx); state.Enabled || !slices.Equal(state.Exclusions, edited) {
		t.Fatalf("stop should restore the disabled setting and retain the edited exclusions: %+v", state)
	}
	if err := m.SetProxyExclusions(ctx, edited, original); err != nil {
		t.Fatal(err)
	}
	if state := m.SystemProxy(ctx); !slices.Equal(state.Exclusions, original) {
		t.Fatalf("removed exclusion still present: %+v", state)
	}

	if err := m.enableSystemProxy(ctx, "127.0.0.1:7890", false); err != nil {
		t.Fatal(err)
	}
	setMode("'auto'")
	if err := m.disableSystemProxy(ctx, ""); err != nil {
		t.Fatal(err)
	}
	external := m.SystemProxy(ctx)
	if !external.Enabled || external.Owned {
		t.Fatalf("stop should preserve the external proxy, got %+v", external)
	}
	if err := m.enableSystemProxy(ctx, "127.0.0.1:7890", false); err != nil {
		t.Fatal(err)
	}
	if m.SystemProxy(ctx).Revision != external.Revision {
		t.Fatal("start should preserve the external settings")
	}
	if err := m.disableSystemProxy(ctx, external.Revision); err != nil {
		t.Fatal(err)
	}
	if m.SystemProxy(ctx).Enabled {
		t.Fatal("explicitly confirmed external proxy should be disabled")
	}
}

func TestDisableUnownedSystemProxyWithoutDesktop(t *testing.T) {
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	m := Manager{Paths: platform.Paths{Data: t.TempDir()}}
	if err := m.disableSystemProxy(t.Context(), ""); err != nil {
		t.Fatalf("restoring a proxy never owned by Termihomo should be a no-op: %v", err)
	}
}

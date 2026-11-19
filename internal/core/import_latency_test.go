package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Nyakooo/termihomo/internal/platform"
)

func TestValidateSimpleProfileUsesBundledGeodata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	m := Manager{Paths: platform.Paths{Config: t.TempDir(), Data: t.TempDir()}}
	if err := os.MkdirAll(filepath.Dir(m.binary()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.binary(), []byte("#!/bin/sh\nfor f in geoip.metadb GeoIP.dat GeoSite.dat ASN.mmdb; do [ -s \"$2/$f\" ] || exit 1; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	// Race instrumentation makes first-time bundled database extraction slower.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := m.validateConfig(ctx, []byte("rules:\n  - MATCH,DIRECT\n")); err != nil {
		t.Fatal(err)
	}
}

// Run with TERMIHOMO_TEST_MIHOMO pointing to the supported real kernel.
// This exercises import, validation and persistence in fresh directories with
// every outbound HTTP request directed to an unreachable local proxy.
func TestImportWithRealMihomoOffline(t *testing.T) {
	binary := os.Getenv("TERMIHOMO_TEST_MIHOMO")
	if binary == "" {
		t.Skip("set TERMIHOMO_TEST_MIHOMO for real-core offline validation")
	}
	payload, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(key, "http://127.0.0.1:1")
	}
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	for _, mode := range []string{"false", "true"} {
		t.Run("geodata-mode="+mode, func(t *testing.T) {
			dir := t.TempDir()
			m := Manager{Paths: platform.Paths{Config: filepath.Join(dir, "config"), Data: filepath.Join(dir, "data"), Logs: filepath.Join(dir, "state", "logs")}}
			if err := m.Paths.Ensure(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(m.binary()), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(m.binary(), payload, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "profile.yaml")
			body := []byte("geodata-mode: " + mode + "\nrules:\n  - GEOSITE,cn,DIRECT\n  - GEOIP,CN,DIRECT\n  - IP-ASN,13335,DIRECT\n  - MATCH,DIRECT\n")
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			var stages []string
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			ctx = WithProfileProgress(ctx, func(stage string) { stages = append(stages, stage) })
			start := time.Now()
			if err := m.AddProfile(ctx, path, "Offline fixture", true); err != nil {
				t.Fatal(err)
			}
			info, err := m.Profile()
			if err != nil || info.Name != "Offline fixture" {
				t.Fatalf("profile not saved: %v", err)
			}
			t.Logf("fresh offline import: %s; stages=%v", time.Since(start), stages)
			start = time.Now()
			if err := m.CheckConfig(ctx); err != nil {
				t.Fatal(err)
			}
			t.Logf("cached validation: %s", time.Since(start))
		})
	}
}

func TestSlowSubscriptionCanCancelWithoutHoldingInstanceLock(t *testing.T) {
	for _, action := range []string{"add", "cli-add", "edit"} {
		t.Run(action, func(t *testing.T) { testSlowSubscriptionCancellation(t, action) })
	}
}

func testSlowSubscriptionCancellation(t *testing.T, action string) {
	dir := t.TempDir()
	m := Manager{Paths: platform.Paths{Config: filepath.Join(dir, "config"), Data: filepath.Join(dir, "data"), Logs: filepath.Join(dir, "state", "logs")}}
	if err := m.Paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(m.binary()), 0700); err != nil {
		t.Fatal(err)
	}
	// Cancellation occurs during download, before this fixture needs to execute.
	if err := os.WriteFile(m.binary(), []byte("unused kernel fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	// This test measures cancellation during HTTP download, independently of
	// first-time decompression (which is much slower under the race detector).
	if err := m.ensureGeodata(t.Context()); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	if action == "edit" {
		original, err := json.Marshal(profile{ProfileInfo: ProfileInfo{Source: "subscription", Name: "Existing"}, URL: "https://example.org/old", YAML: []byte("rules: [MATCH,DIRECT]\n")})
		if err != nil {
			t.Fatal(err)
		}
		if err := atomicWrite(m.profilePath(), original); err != nil {
			t.Fatal(err)
		}
	}
	go func() {
		switch action {
		case "cli-add":
			done <- m.AddSubscription(ctx, server.URL)
		case "edit":
			done <- m.EditProfile(ctx, "", "Existing", server.URL, ProfileSchedule{})
		default:
			done <- m.AddProfile(ctx, server.URL, "Slow subscription", false)
		}
	}()
	select {
	case <-started:
	case err := <-done:
		t.Fatalf("import failed before download: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("download did not start")
	}
	lock, err := m.Paths.LockInstance()
	if err != nil {
		t.Fatalf("subscription download held the instance lock: %v", err)
	}
	lock.Close()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("download ignored cancellation")
	}
	info, err := m.Profile()
	if err != nil || (action == "edit" && info.Name != "Existing") || (action != "edit" && info.Source != "") {
		t.Fatal("cancelled download replaced profile")
	}
}

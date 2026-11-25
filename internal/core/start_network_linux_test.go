//go:build !android

package core

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// Boot services invoke Start directly, without opening the dashboard. A saved
// TUN preference must reach the controller even when Mihomo is already running.
func TestStartRestoresSavedTUN(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[enabled], func(t *testing.T) {
			fakeDesktopProxy(t)
			m := networkTestManager(t)
			if err := m.saveNetworkPreference(tunPreference, enabled); err != nil {
				t.Fatal(err)
			}
			if err := m.saveNetworkPreference(systemProxyPreference, true); err != nil {
				t.Fatal(err)
			}
			var attempted atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/version":
					w.Write([]byte(`{"version":"` + Version + `"}`))
				case "/rules":
					w.Write([]byte(`{"rules":[{"type":"MATCH","proxy":"DIRECT"}]}`))
				case "/configs":
					attempted.Store(true)
					http.Error(w, "TUN restoration reached controller", http.StatusServiceUnavailable)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			identity, err := processIdentity(os.Getpid())
			if err != nil {
				t.Fatal(err)
			}
			if err := m.writeInstance(instance{PID: os.Getpid(), Identity: identity, Secret: "test-only", Controller: strings.TrimPrefix(server.URL, "http://"), Proxy: listener.Addr().String()}); err != nil {
				t.Fatal(err)
			}
			err = m.Start(t.Context())
			if attempted.Load() != enabled {
				t.Fatalf("saved TUN=%v, restoration attempted=%v, error=%v", enabled, attempted.Load(), err)
			}
			if enabled && (err == nil || !strings.Contains(err.Error(), "503")) {
				t.Fatalf("restoration failure must be reported: %v", err)
			}
			if !m.SystemProxy(t.Context()).Owned {
				t.Fatal("TUN restoration failure prevented system proxy restoration")
			}
			if !enabled && err != nil {
				t.Fatal(err)
			}
		})
	}
}

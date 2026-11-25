//go:build !android

package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Nyakooo/termihomo/internal/platform"
)

func networkTestManager(t *testing.T) Manager {
	t.Helper()
	root := t.TempDir()
	m := Manager{Paths: platform.Paths{Config: filepath.Join(root, "config"), Data: filepath.Join(root, "data"), Logs: filepath.Join(root, "state", "logs")}}
	if err := m.Paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(m.binary()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.binary(), nil, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(m.runDir(), 0700); err != nil {
		t.Fatal(err)
	}
	files, err := geodataFiles()
	if err != nil {
		t.Fatal(err)
	}
	manifest := map[string]int64{}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(m.runDir(), file.Name), []byte("cached"), 0600); err != nil {
			t.Fatal(err)
		}
		manifest[file.Name] = 6
	}
	writeManifest(t, m, manifest)
	return m
}

func fakeDesktopProxy(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	t.Setenv("TEST_PROXY_DIR", dir)
	script := `#!/bin/sh
[ -f "$TEST_PROXY_DIR/unavailable" ] && exit 1
path="$TEST_PROXY_DIR/$2.$3"
case $1 in
list-recursively) for f in "$TEST_PROXY_DIR"/org.gnome.system.proxy.*; do name=${f##*/}; printf '%s %s %s\n' "${name%.*}" "${name##*.}" "$(cat "$f")"; done;;
get) cat "$path";;
set) printf '%s' "$4" > "$path";;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "gsettings"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	for _, setting := range proxySettings {
		value := "0"
		if setting.key == "mode" {
			value = "'none'"
		}
		if setting.key == "ignore-hosts" {
			value = "['localhost']"
		}
		if err := os.WriteFile(filepath.Join(dir, setting.schema+"."+setting.key), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// This controller changes real TCP listener bindings so LAN verification uses
// the same socket checks as production, without changing host network settings.
func networkTestController(t *testing.T, m Manager) instance {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_, port, _ := net.SplitHostPort(address)
	var mu sync.Mutex
	config := map[string]any{"mixed-port": mustPort(t, port), "allow-lan": false, "tun": map[string]any{"enable": false}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/version":
			json.NewEncoder(w).Encode(map[string]string{"version": Version})
		case "/rules":
			w.Write([]byte(`{"rules":[{"type":"MATCH","proxy":"DIRECT"}]}`))
		case "/connections":
			w.Write([]byte(`{}`))
		case "/configs":
			if r.Method == http.MethodPatch {
				patch := map[string]any{}
				if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
					t.Error(err)
					return
				}
				for key, value := range patch {
					config[key] = value
				}
				listener.Close()
				if config["mixed-port"] != float64(0) {
					host := "127.0.0.1"
					if config["allow-lan"] == true {
						host = "0.0.0.0"
					}
					listener, err = net.Listen("tcp4", net.JoinHostPort(host, port))
					if err != nil {
						t.Error(err)
						http.Error(w, "bind failed", 500)
						return
					}
				}
			}
			json.NewEncoder(w).Encode(config)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() { server.Close(); listener.Close() })
	identity, err := processIdentity(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	state := instance{PID: os.Getpid(), Identity: identity, Secret: "test-only", Controller: strings.TrimPrefix(server.URL, "http://"), Proxy: address}
	if err := m.writeInstance(state); err != nil {
		t.Fatal(err)
	}
	return state
}

func mustPort(t *testing.T, port string) float64 {
	t.Helper()
	var value float64
	if _, err := fmt.Sscan(port, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestNetworkPreferencesAcrossStartupAndPause(t *testing.T) {
	for _, dashboard := range []bool{false, true} {
		for _, lan := range []bool{false, true} {
			for _, proxy := range []bool{false, true} {
				t.Run(fmt.Sprintf("dashboard=%v/lan=%v/proxy=%v", dashboard, lan, proxy), func(t *testing.T) {
					if addresses, _ := LocalProxyNetworks(); lan && len(addresses) == 0 {
						t.Skip("LAN activation requires a private IPv4 interface")
					}
					fakeDesktopProxy(t)
					m := networkTestManager(t)
					networkTestController(t, m)
					for key, value := range map[string]bool{lanPreference: lan, systemProxyPreference: proxy, tunPreference: false} {
						if err := m.saveNetworkPreference(key, value); err != nil {
							t.Fatal(err)
						}
					}
					start := m.Start
					if dashboard {
						start = m.StartDashboardProxy
					}
					if err := start(t.Context()); err != nil {
						t.Fatal(err)
					}
					check := func(wantLAN, wantProxy bool) {
						t.Helper()
						overview, err := m.Overview(t.Context())
						if err != nil {
							t.Fatal(err)
						}
						if overview.LAN != wantLAN {
							t.Fatalf("LAN=%v, want %v", overview.LAN, wantLAN)
						}
						if got := m.SystemProxy(t.Context()); got.Enabled != wantProxy || got.Owned != wantProxy {
							t.Fatalf("proxy=%+v, want enabled/owned=%v", got, wantProxy)
						}
					}
					check(lan, proxy)
					if err := m.PauseProxy(t.Context()); err != nil {
						t.Fatal(err)
					}
					check(false, false)
					if err := start(t.Context()); err != nil {
						t.Fatal(err)
					}
					check(lan, proxy)
					// Explicitly turning LAN off must replace the persisted preference.
					if err := m.SetLAN(t.Context(), false); err != nil {
						t.Fatal(err)
					}
					if err := m.PauseProxy(t.Context()); err != nil {
						t.Fatal(err)
					}
					if err := start(t.Context()); err != nil {
						t.Fatal(err)
					}
					check(false, proxy)
				})
			}
		}
	}
}

func TestLANPreferencePersistsAndCanBeDisabledWhilePaused(t *testing.T) {
	if addresses, _ := LocalProxyNetworks(); len(addresses) == 0 {
		t.Skip("LAN activation requires a private IPv4 interface")
	}
	m := networkTestManager(t)
	networkTestController(t, m)
	if err := m.SetLAN(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	settings, err := m.readToolSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings[lanPreference] != true {
		t.Fatal("LAN enable was not persisted")
	}
	if err := m.PauseProxy(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.SetLAN(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	overview, err := m.Overview(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if overview.LAN {
		t.Fatal("paused LAN disable was lost")
	}
}

func TestDesktopRestoresSystemProxyAndUpdatesOwnedPort(t *testing.T) {
	fakeDesktopProxy(t)
	m := networkTestManager(t)
	state := networkTestController(t, m)
	if err := m.saveNetworkPreference(systemProxyPreference, true); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	if err := m.StartBoot(t.Context()); !errors.Is(err, errNetworkPending) {
		t.Fatalf("pre-login network restoration must be deferred: %v", err)
	}
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	if err := m.RestoreDesktopNetwork(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !m.SystemProxy(t.Context()).Owned {
		t.Fatal("desktop login did not restore system proxy")
	}
	if err := m.enableSystemProxy(t.Context(), "127.0.0.1:1", false); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	owner, _ := m.proxyOwnership()
	_, port, _ := net.SplitHostPort(state.Proxy)
	if owner.Applied[1] != port {
		t.Fatalf("owned proxy port=%s, want %s", owner.Applied[1], port)
	}
	if err := m.PauseProxy(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreDesktopNetwork(t.Context()); err != nil {
		t.Fatal(err)
	}
	if m.SystemProxy(t.Context()).Enabled {
		t.Fatal("desktop retry resumed a paused proxy")
	}
}

func TestColdStartStopAndRestartKeepNetworkPreferences(t *testing.T) {
	if addresses, _ := LocalProxyNetworks(); len(addresses) == 0 {
		t.Skip("LAN activation requires a private IPv4 interface")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("fixture requires Python 3")
	}
	fakeDesktopProxy(t)
	m := networkTestManager(t)
	fixture, err := os.ReadFile("testdata/network_kernel.py")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.binary(), append([]byte("#!"+python+"\n"), fixture...), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_NETWORK_KERNEL_VERSION", Version)
	// Choose an unoccupied port rather than using the user's normal proxy port.
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	listener.Close()
	if err := m.SetToolSetting(t.Context(), "mixed-port", port); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]bool{lanPreference: true, systemProxyPreference: true, tunPreference: false} {
		if err := m.saveNetworkPreference(key, value); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if err := m.Stop(); err != nil {
			t.Error(err)
		}
	})
	check := func() {
		t.Helper()
		overview, err := m.Overview(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !overview.LAN || !m.SystemProxy(t.Context()).Owned {
			t.Fatalf("lost network preferences: overview=%+v proxy=%+v", overview, m.SystemProxy(t.Context()))
		}
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	check()
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	if m.SystemProxy(t.Context()).Owned {
		t.Fatal("stop left owned system proxy enabled")
	}
	if err := m.StartDashboardProxy(t.Context()); err != nil {
		t.Fatal(err)
	}
	check()
	if err := m.Reload(t.Context()); err != nil {
		t.Fatal(err)
	}
	check()
	if err := m.PauseProxy(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Also cover restarting an intentionally paused controller.
	if err := m.Restart(t.Context()); err != nil {
		t.Fatal(err)
	}
	status, err := m.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "Paused" {
		t.Fatalf("restart resumed paused core: %+v", status)
	}
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	check()
	if err := m.Restart(t.Context()); err != nil {
		t.Fatal(err)
	}
	check()
	// Stop removes instance.json, but must retain all saved enable preferences.
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := m.Restart(t.Context()); err != nil {
		t.Fatal(err)
	}
	check()
}

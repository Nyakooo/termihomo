//go:build linux && !android

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/platform"
	"github.com/Nyakooo/termihomo/internal/tray"
)

func TestSystemProxyExternalConfirmation(t *testing.T) {
	isolateCLI(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	t.Setenv("TEST_PROXY_MODE", filepath.Join(dir, "mode"))
	script := `#!/bin/sh
if [ "$1" = set ]; then
  printf '%s' "$4" > "$TEST_PROXY_MODE"
  exit 0
fi
cat <<'VALUES'
org.gnome.system.proxy.http host '127.0.0.1'
org.gnome.system.proxy.http port 1234
org.gnome.system.proxy.https host '127.0.0.1'
org.gnome.system.proxy.https port 1234
org.gnome.system.proxy.socks host '127.0.0.1'
org.gnome.system.proxy.socks port 1234
org.gnome.system.proxy.http use-authentication false
org.gnome.system.proxy autoconfig-url ''
org.gnome.system.proxy ignore-hosts ['localhost']
org.gnome.system.proxy use-same-proxy true
org.gnome.system.proxy.ftp host ''
org.gnome.system.proxy.ftp port 0
VALUES
printf 'org.gnome.system.proxy mode %s\n' "$(cat "$TEST_PROXY_MODE")"
`
	if err := os.WriteFile(filepath.Join(dir, "gsettings"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	modeFile := filepath.Join(dir, "mode")
	if err := os.WriteFile(modeFile, []byte("'manual'"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"sysproxy", "start"}, {"sysproxy", "stop"}} {
		var out, diagnostics bytes.Buffer
		if code := runCLI(args, &out, &diagnostics); code != 1 {
			t.Fatalf("external proxy changed without confirmation: %v, code=%d", args, code)
		}
		mode, _ := os.ReadFile(modeFile)
		if string(mode) != "'manual'" {
			t.Fatal("external proxy was overwritten")
		}
	}
	var out, diagnostics bytes.Buffer
	if code := runCLI([]string{"sysproxy", "status", "--json"}, &out, &diagnostics); code != 0 {
		t.Fatal(diagnostics.String())
	}
	var status struct{ Available, Enabled, Owned bool }
	if err := json.Unmarshal(out.Bytes(), &status); err != nil || !status.Available || !status.Enabled || status.Owned {
		t.Fatalf("unexpected external status: %s", &out)
	}
	out.Reset()
	if code := runCLI([]string{"sysproxy", "stop", "--yes"}, &out, &diagnostics); code != 0 {
		t.Fatal(diagnostics.String())
	}
	mode, _ := os.ReadFile(modeFile)
	if string(mode) != "'none'" {
		t.Fatalf("confirmed stop did not disable proxy: %s", mode)
	}
}

func TestModeCommandsApplyAndQueryController(t *testing.T) {
	isolateCLI(t)
	mode := "rule"
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/version" {
			_ = json.NewEncoder(w).Encode(map[string]string{"version": core.Version})
			return
		}
		if r.URL.Path == "/rules" {
			_, _ = w.Write([]byte(`{"rules":[{"type":"MATCH","proxy":"DIRECT"}]}`))
			return
		}
		if r.URL.Path != "/configs" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPatch {
			var patch struct{ Mode string }
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				t.Error(err)
			}
			if patch.Mode != "" {
				mode = patch.Mode
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"mode": mode})
	}))
	defer server.Close()
	paths, err := platform.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	// Use the test process identity so the real managed-instance validation runs.
	stat, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		t.Fatal(err)
	}
	state, _ := json.Marshal(map[string]any{"PID": os.Getpid(), "Identity": strings.TrimSpace(string(boot)) + ":" + fields[19], "Secret": "test-only", "Paused": true, "ResumeTUN": true, "ResumeSystemProxy": true, "Controller": strings.TrimPrefix(server.URL, "http://")})
	if err := os.WriteFile(filepath.Join(paths.Data, "instance.json"), state, 0600); err != nil {
		t.Fatal(err)
	}
	binary := (core.Manager{Paths: paths}).BinaryPath()
	if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, nil, 0700); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"global", "direct", "rule"} {
		var out, diagnostics bytes.Buffer
		if code := runCLI([]string{"mode", value}, &out, &diagnostics); code != 0 {
			t.Fatalf("mode %s failed: %s", value, &diagnostics)
		}
		out.Reset()
		if code := runCLI([]string{"mode", "status", "--json"}, &out, &diagnostics); code != 0 {
			t.Fatal(diagnostics.String())
		}
		var result struct {
			Mode string `json:"mode"`
		}
		if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Mode != value {
			t.Fatalf("wrong mode status: %s", &out)
		}
	}
	var out, diagnostics bytes.Buffer
	if code := runCLI([]string{"tun", "status", "--json"}, &out, &diagnostics); code != 0 || !strings.Contains(out.String(), `"paused":true`) {
		t.Fatalf("expected paused TUN: %s %s", &out, &diagnostics)
	}
	out.Reset()
	if code := runCLI([]string{"tun", "stop"}, &out, &diagnostics); code != 0 {
		t.Fatal(diagnostics.String())
	}
	out.Reset()
	if code := runCLI([]string{"tun", "status", "--json"}, &out, &diagnostics); code != 0 || strings.Contains(out.String(), `"paused":true`) {
		t.Fatalf("disabled TUN still paused: %s %s", &out, &diagnostics)
	}

	manager := core.Manager{Paths: paths}
	if err := manager.SetSystemProxy(t.Context(), false, ""); err != nil {
		t.Fatal(err)
	}
	status, err := manager.Status(t.Context())
	if err != nil || status.ResumeSystemProxy {
		t.Fatalf("disabled system proxy still scheduled for resume: %+v %v", status, err)
	}

}

func TestDownReportsProxyRestorationFailure(t *testing.T) {
	isolateCLI(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	if err := os.WriteFile(filepath.Join(dir, "gsettings"), []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	paths, err := platform.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.Data, "system-proxy.json"), []byte(`{"Before":["original"],"Applied":["owned"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	if code := runCLI([]string{"down"}, &out, &diagnostics); code != 1 {
		t.Fatalf("down hid proxy restoration failure: code=%d, %s", code, &diagnostics)
	}
}

func TestDownStopsManagedProcessDespiteInterfaceFailure(t *testing.T) {
	for _, interfaceFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(interfaceFailure), func(t *testing.T) {
			isolateCLI(t)
			paths, err := platform.ResolvePaths()
			if err != nil {
				t.Fatal(err)
			}
			if err := paths.Ensure(); err != nil {
				t.Fatal(err)
			}
			child := exec.Command("sleep", "60")
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			defer child.Process.Kill()
			stopped := make(chan struct{})
			go func() { _ = child.Wait(); close(stopped) }()
			stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", child.Process.Pid))
			if err != nil {
				t.Fatal(err)
			}
			fields := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))
			boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
			if err != nil {
				t.Fatal(err)
			}
			state, _ := json.Marshal(map[string]any{"PID": child.Process.Pid, "Identity": strings.TrimSpace(string(boot)) + ":" + fields[19], "Secret": "test-only"})
			if err := os.WriteFile(filepath.Join(paths.Data, "instance.json"), state, 0600); err != nil {
				t.Fatal(err)
			}
			notified := make(chan bool, 1)
			if interfaceFailure {
				if err := os.Mkdir(filepath.Join(paths.Data, "tray-interface.lock"), 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				lock, err := paths.RegisterTrayInterface()
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Close()
				trayLock, err := paths.LockTray()
				if err != nil {
					t.Fatal(err)
				}
				defer trayLock.Close()
				go func() {
					deadline := time.Now().Add(3 * time.Second)
					seen := false
					for time.Now().Before(deadline) {
						if tray.InterfaceToken(paths, tray.ExitInterface) != "" {
							seen = true
							break
						}
						time.Sleep(10 * time.Millisecond)
					}
					_ = lock.Close()
					_ = trayLock.Close()
					notified <- seen
				}()
			}
			var out, diagnostics bytes.Buffer
			want := 0
			if interfaceFailure {
				want = 1
			}
			if code := runCLI([]string{"down"}, &out, &diagnostics); code != want {
				t.Fatalf("down code=%d want=%d: %s", code, want, &diagnostics)
			}
			select {
			case <-stopped:
			case <-time.After(time.Second):
				t.Fatal("down left managed core running")
			}
			if !interfaceFailure && !<-notified {
				t.Fatal("down never requested interface shutdown")
			}
			if _, err := os.Stat(filepath.Join(paths.Data, "instance.json")); !os.IsNotExist(err) {
				t.Fatalf("managed state not removed: %v", err)
			}
		})
	}
}

package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/platform"
)

type runtimeStatus struct {
	State, Proxy, Controller string
	PID                      int
}

func profile(group string) string {
	return fmt.Sprintf("profile: {store-selected: false}\nproxy-groups:\n  - name: %s\n    type: select\n    proxies: [REJECT, DIRECT]\nrules: [\"MATCH,%s\"]\n", group, group)
}

func TestReleaseBinary(t *testing.T) {
	binary := os.Getenv("TERMIHOMO_SMOKE_BINARY")
	if binary == "" {
		t.Skip("set TERMIHOMO_SMOKE_BINARY to exercise the release binary")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	config := filepath.Join(root, "config")
	data := filepath.Join(root, "data")
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("TERMIHOMO_LANG", "en")
	t.Setenv("CLASH_OVERRIDE_EXTERNAL_CONTROLLER", "0.0.0.0:9090")

	run := func(input string, args ...string) (string, string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Stdin = strings.NewReader(input)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	}
	command := func(args ...string) string {
		t.Helper()
		out, diagnostic, err := run("", args...)
		if err != nil {
			t.Fatalf("%v: %v: %s", args, err, diagnostic)
		}
		return out
	}
	status := func() runtimeStatus {
		t.Helper()
		var s runtimeStatus
		if err := json.Unmarshal([]byte(command("status", "--json")), &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if s := status(); s.State != "NotInstalled" {
		t.Fatalf("initial status: %+v", s)
	}
	command("init")
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	overrides := filepath.Join(config, "termihomo", "overrides.yaml")
	if err := os.WriteFile(overrides, fmt.Appendf(nil, "mixed-port: %d\n", port), 0o600); err != nil {
		t.Fatal(err)
	}

	var subscription atomic.Value
	subscription.Store(profile("Pick"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sub" {
			w.Header().Set("Subscription-Userinfo", "upload=1024; download=2048; total=4096")
			_, _ = io.WriteString(w, subscription.Load().(string))
			return
		}
		_, _ = io.WriteString(w, "proxy-ok")
	}))
	defer server.Close()
	command("install")
	if s := status(); s.State != "Stopped" {
		t.Fatalf("after install: %+v", s)
	}
	t.Cleanup(func() {
		if _, diagnostic, err := run("", "stop", "--yes"); err != nil {
			t.Errorf("stop: %v: %s", err, diagnostic)
		}
	})
	const secret = "release-test-token"
	if _, diagnostic, err := run(server.URL+"/sub?token="+secret, "profile", "add", "-"); err != nil {
		t.Fatalf("import subscription: %v: %s", err, diagnostic)
	}
	command("config", "check")
	command("start")
	running := status()
	if running.State != "Running" || running.Proxy != address || !strings.HasPrefix(running.Controller, "127.0.0.1:") {
		t.Fatalf("after start: %+v", running)
	}
	if !strings.Contains(command("--lang", "zh-CN", "status"), "运行中") {
		t.Fatal("Chinese status missing")
	}
	paths, err := platform.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	manager := core.Manager{Paths: paths}
	if err := manager.PauseProxy(t.Context()); err != nil {
		t.Fatal(err)
	}
	if paused := status(); paused.State != "Paused" || paused.PID != running.PID || paused.Proxy != "" {
		t.Fatalf("dashboard pause changed the managed process: %+v", paused)
	}
	command("start")
	if resumed := status(); resumed != running {
		t.Fatalf("dashboard resume changed the managed process: %+v", resumed)
	}
	command("tun", "stop")
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{}}
	defer client.CloseIdleConnections()
	response, err := client.Get("http://" + running.Controller + "/version")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated controller returned %d", response.StatusCode)
	}

	checkProxy := func(proxy *url.URL, expected int) {
		t.Helper()
		transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
		response, err := client.Get(server.URL + "/echo")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != expected || expected == http.StatusOK && string(body) != "proxy-ok" {
			t.Fatalf("proxy response: status %d, body %q, error %v", response.StatusCode, body, err)
		}
	}
	localProxy := &url.URL{Scheme: "http", Host: running.Proxy}
	command("proxy", "select", "Pick", "DIRECT")
	checkProxy(localProxy, 200)
	stateFile := filepath.Join(data, "termihomo", "instance.json")
	stateBytes, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	var state struct{ Secret, ServiceProbe string }
	if err := json.Unmarshal(stateBytes, &state); err != nil {
		t.Fatal(err)
	}
	probe := &url.URL{Scheme: "http", Host: state.ServiceProbe}
	checkProxy(probe, http.StatusProxyAuthRequired)
	req, err := http.NewRequest(http.MethodPut, "http://"+running.Controller+"/proxies/__termihomo_service_probe", strings.NewReader(`{"name":"DIRECT"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+state.Secret)
	req.Header.Set("Content-Type", "application/json")
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode/100 != 2 {
		t.Fatalf("probe selector returned %d", response.StatusCode)
	}
	probe.User = url.UserPassword("termihomo", state.Secret)
	checkProxy(probe, 200)
	checkProxy(localProxy, 200)

	subscription.Store("broken: [\n")
	if _, diagnostic, err := run("", "profile", "update", "--yes"); err == nil || strings.Contains(diagnostic, secret) {
		t.Fatal("invalid profile update succeeded or disclosed the subscription token")
	}
	checkProxy(localProxy, 200)

	upstream := &url.URL{Scheme: "http", Host: running.Controller}
	forward := httputil.NewSingleHostReverseProxy(upstream)
	forward.Transport = &http.Transport{}
	rollbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/configs" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		forward.ServeHTTP(w, r)
	}))
	defer rollbackServer.Close()
	var record map[string]json.RawMessage
	if err := json.Unmarshal(stateBytes, &record); err != nil {
		t.Fatal(err)
	}
	record["Controller"], err = json.Marshal(rollbackServer.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	modified, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFile, modified, 0o600); err != nil {
		t.Fatal(err)
	}
	subscription.Store(profile("Rejected"))
	if _, diagnostic, err := run("", "profile", "update", "--yes"); err == nil ||
		!strings.Contains(diagnostic, "HTTP 503") || !strings.Contains(diagnostic, "restored previous configuration") {
		t.Fatalf("expected reload rollback: %v: %s", err, diagnostic)
	}
	restored := status()
	if restored.State != "Running" || restored.PID == running.PID || restored.Proxy != running.Proxy ||
		!strings.Contains(command("proxy", "list"), `"Pick"`) {
		t.Fatalf("rollback did not restore the previous profile: %+v", restored)
	}
	running = restored
	checkProxy(localProxy, 200)

	subscription.Store(profile("Next"))
	command("profile", "update", "--yes")
	command("proxy", "select", "Next", "DIRECT")
	if result := command("proxy", "test", "DIRECT", server.URL+"/echo"); !strings.Contains(result, " ms") {
		t.Fatalf("latency result: %q", result)
	}
	backupPath := strings.TrimSpace(command("config", "backup"))
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("backup permissions: %v", info.Mode())
	}
	if strings.Contains(command("logs"), secret) {
		t.Fatal("logs disclosed the subscription token")
	}

	requestConfig := func(method, payload string, result any) {
		t.Helper()
		saved, err := os.ReadFile(stateFile)
		if err != nil {
			t.Fatal(err)
		}
		var instance struct{ Secret, Controller string }
		if err := json.Unmarshal(saved, &instance); err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(method, "http://"+instance.Controller+"/configs", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+instance.Secret)
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode/100 != 2 {
			t.Fatalf("%s /configs returned %d", method, response.StatusCode)
		}
		if result != nil {
			if err := json.NewDecoder(response.Body).Decode(result); err != nil {
				t.Fatal(err)
			}
		}
	}
	requestConfig(http.MethodPatch, `{"mode":"direct"}`, nil)
	command("profile", "update", "--yes")
	var profileMode struct{ Mode string }
	requestConfig(http.MethodGet, "", &profileMode)
	if profileMode.Mode != "direct" || !strings.Contains(command("proxy", "list"), `"Next"`+"\tSelector\t"+`"DIRECT"`) {
		t.Fatal("profile update changed the live mode or selected node")
	}
	if err := os.WriteFile(overrides, fmt.Appendf(nil, "mixed-port: %d\nlog-level: debug\n", port), 0o600); err != nil {
		t.Fatal(err)
	}
	command("reload")
	if reloaded := status(); reloaded.State != "Running" || reloaded.PID != running.PID {
		t.Fatalf("reload did not keep the running process: %+v", reloaded)
	}
	if !strings.Contains(command("proxy", "list"), `"Next"`+"\tSelector\t"+`"DIRECT"`) {
		t.Fatal("reload did not preserve the selected node")
	}
	var applied struct {
		LogLevel string `json:"log-level"`
		Mode     string
	}
	requestConfig(http.MethodGet, "", &applied)
	if applied.LogLevel != "debug" || applied.Mode != "direct" {
		t.Fatalf("reload did not apply saved settings and preserve the live mode: %+v", applied)
	}
	checkProxy(localProxy, 200)

	blocked, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer blocked.Close()
	if err := os.WriteFile(overrides, fmt.Appendf(nil, "mixed-port: %d\n", blocked.Addr().(*net.TCPAddr).Port), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run("", "restart"); err == nil {
		t.Fatal("restart accepted an occupied port")
	}
	if unchanged := status(); unchanged.State != "Running" || unchanged.PID != running.PID || unchanged.Proxy != running.Proxy {
		t.Fatalf("restart with an occupied port disturbed the running core: %+v", unchanged)
	}
	checkProxy(localProxy, 200)
	if err := os.WriteFile(overrides, fmt.Appendf(nil, "mixed-port: %d\nlog-level: debug\n", port), 0o600); err != nil {
		t.Fatal(err)
	}
	command("restart")
	restarted := status()
	if restarted.State != "Running" || restarted.PID == running.PID {
		t.Fatalf("restart did not replace the process: %+v", restarted)
	}
	if !strings.Contains(command("proxy", "list"), `"Next"`+"\tSelector\t"+`"DIRECT"`) {
		t.Fatal("restart did not preserve the selected node")
	}
	checkProxy(&url.URL{Scheme: "http", Host: restarted.Proxy}, 200)
	process, err := os.FindProcess(restarted.PID)
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = process.Release()
	deadline := time.Now().Add(6 * time.Second)
	for s := status(); s.State != "Failed"; s = status() {
		if time.Now().After(deadline) {
			t.Fatalf("abnormal exit was not detected: %+v", s)
		}
		time.Sleep(50 * time.Millisecond)
	}
	command("stop")
	if s := status(); s.State != "Stopped" {
		t.Fatalf("after stop: %+v", s)
	}
}

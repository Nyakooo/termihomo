package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nyakooo/termihomo/internal/platform"
	"go.yaml.in/yaml/v3"
)

const routingSource = "proxy-groups:\n  - name: \"\\U0001F530 \\u9009\\u62e9\\u8282\\u70b9\"\n    type: select\n    proxies: [DIRECT]\nrules:\n  - DOMAIN,original.example,DIRECT\n  - MATCH,DIRECT\n"

func gmailRules() []CustomRule {
	return []CustomRule{{Type: "DOMAIN", Value: "imap.gmail.com", Target: "🔰 选择节点"}, {Type: "DOMAIN", Value: "smtp.gmail.com", Target: "🔰 选择节点"}}
}

func TestCustomRulesOrderAndUnicode(t *testing.T) {
	p := profile{YAML: []byte(routingSource), CustomRules: append(gmailRules(), CustomRule{Type: "DST-PORT", Value: "993", Target: "DIRECT", Disabled: true})}
	rules, err := profileRules(p)
	if err != nil || !reflect.DeepEqual(rules.Targets, []string{"DIRECT", "REJECT", "🔰 选择节点"}) {
		t.Fatalf("targets=%v err=%v", rules.Targets, err)
	}
	body, err := renderProfileWithSettings(p, validationInstance(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var config struct{ Rules []string }
	if err := yaml.Unmarshal(body, &config); err != nil {
		t.Fatal(err)
	}
	want := []string{"DOMAIN,imap.gmail.com,🔰 选择节点", "DOMAIN,smtp.gmail.com,🔰 选择节点", "DOMAIN,original.example,DIRECT", "MATCH,DIRECT"}
	if !reflect.DeepEqual(config.Rules, want) {
		t.Fatalf("rules=%q want=%q", config.Rules, want)
	}
	if string(p.YAML) != routingSource {
		t.Fatal("original source mutated")
	}
	p.CustomRules[0], p.CustomRules[1] = p.CustomRules[1], p.CustomRules[0]
	body, err = renderProfileWithSettings(p, validationInstance(), nil)
	if err != nil {
		t.Fatal(err)
	}
	yaml.Unmarshal(body, &config)
	if config.Rules[0] != want[1] {
		t.Fatal("priority lost")
	}
}

func TestValidateCustomRules(t *testing.T) {
	targets := []string{"DIRECT", "REJECT", "🔰 选择节点", "中文 group 🚀"}
	for _, r := range []CustomRule{
		{Type: "DOMAIN", Value: "imap.gmail.com", Target: targets[2]},
		{Type: "DOMAIN-SUFFIX", Value: "gmail.com", Target: targets[3]},
		{Type: "IP-CIDR", Value: "192.0.2.0/24", Target: "DIRECT"},
		{Type: "DST-PORT", Value: "65535", Target: "REJECT"},
	} {
		if err := ValidateCustomRules([]CustomRule{r}, targets); err != nil {
			t.Errorf("%+v: %v", r, err)
		}
	}
	for _, r := range []CustomRule{
		{Type: "DOMAIN", Value: "imap.gmail.com:993", Target: "DIRECT"},
		{Type: "DOMAIN", Value: "a,,b", Target: "DIRECT"},
		{Type: "DOMAIN", Value: "a b", Target: "DIRECT"},
		{Type: "DOMAIN", Value: "-bad.com", Target: "DIRECT"},
		{Type: "DOMAIN-SUFFIX", Value: ".gmail.com", Target: "DIRECT"},
		{Type: "IP-CIDR", Value: "1.2.3.4", Target: "DIRECT"},
		{Type: "IP-CIDR", Value: "1.2.3.4/99", Target: "DIRECT"},
		{Type: "DST-PORT", Value: "0", Target: "DIRECT"},
		{Type: "DST-PORT", Value: "65536", Target: "DIRECT"},
		{Type: "DST-PORT", Value: "0993", Target: "DIRECT"},
		{Type: "MATCH", Value: "anything", Target: "DIRECT"},
		{Type: "DOMAIN", Value: "a.com", Target: "missing", Disabled: true},
	} {
		if err := ValidateCustomRules([]CustomRule{r}, targets); err == nil {
			t.Errorf("accepted %+v", r)
		}
	}
	for _, rules := range [][]CustomRule{
		{{Type: "DOMAIN", Value: "A.com", Target: "DIRECT"}, {Type: "DOMAIN", Value: "a.com", Target: "REJECT", Disabled: true}},
		{{Type: "IP-CIDR", Value: "192.0.2.1/24", Target: "DIRECT"}, {Type: "IP-CIDR", Value: "192.0.2.0/24", Target: "DIRECT"}},
	} {
		if err := ValidateCustomRules(rules, targets); err == nil {
			t.Fatal("accepted duplicates")
		}
	}
}

func routingTestManager(t *testing.T) Manager {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	m := routingTestPaths(t)
	if err := os.MkdirAll(filepath.Dir(m.binary()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.binary(), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return m
}

func routingTestPaths(t *testing.T) Manager {
	t.Helper()
	root := t.TempDir()
	m := Manager{Paths: platform.Paths{Config: filepath.Join(root, "config"), Data: filepath.Join(root, "data"), Logs: filepath.Join(root, "state", "logs")}}
	if err := m.Paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	return m
}

func saveRoutingProfile(t *testing.T, m Manager, p profile) {
	t.Helper()
	record, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(m.profilePath(), record); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingPersistenceUpdateAndIsolation(t *testing.T) {
	m := routingTestManager(t)
	first := profile{ProfileInfo: ProfileInfo{ID: strings.Repeat("a", 32), Source: "subscription", Updated: time.Now()}, YAML: []byte(routingSource)}
	saveRoutingProfile(t, m, first)
	draft, _ := m.RoutingRules()
	draft.Custom = gmailRules()
	if err := m.SaveRoutingRules(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	first, _ = m.readProfile()
	if !reflect.DeepEqual(first.CustomRules, gmailRules()) {
		t.Fatal("save lost rules")
	}
	if err := m.saveLibraryProfile(first); err != nil {
		t.Fatal(err)
	}
	next := profile{ProfileInfo: first.ProfileInfo, YAML: []byte(routingSource + "# subscription update\n")}
	if err := m.updateProfileCandidate(t.Context(), first, next, true); err != nil {
		t.Fatal(err)
	}
	current, _ := m.readProfile()
	if !reflect.DeepEqual(current.CustomRules, gmailRules()) {
		t.Fatal("subscription update lost rules")
	}
	second := profile{ProfileInfo: ProfileInfo{ID: strings.Repeat("b", 32), Source: "file"}, YAML: []byte("rules:\n  - MATCH,DIRECT\n")}
	if err := m.saveLibraryProfile(second); err != nil {
		t.Fatal(err)
	}
	if err := m.ActivateProfile(t.Context(), second.ID); err != nil {
		t.Fatal(err)
	}
	rules, err := m.RoutingRules()
	if err != nil || len(rules.Custom) != 0 {
		t.Fatalf("rules leaked to second profile: %+v %v", rules, err)
	}
	if err := m.SaveRoutingRules(t.Context(), draft); err == nil {
		t.Fatal("accepted stale draft from other profile")
	}
	if err := m.ActivateProfile(t.Context(), first.ID); err != nil {
		t.Fatal(err)
	}
	// New Manager simulates a restart without any cached state.
	restarted := Manager{Paths: m.Paths}
	rules, err = restarted.RoutingRules()
	if err != nil || !reflect.DeepEqual(rules.Custom, gmailRules()) {
		t.Fatalf("rules lost after switch/restart: %+v %v", rules, err)
	}
	// Inactive subscription updates also retain the configuration's own rules.
	first, _, _ = m.findProfile(first.ID)
	if err := m.ActivateProfile(t.Context(), second.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.updateProfileCandidate(t.Context(), first, next, false); err != nil {
		t.Fatal(err)
	}
	saved, active, err := m.findProfile(first.ID)
	if err != nil || active || !reflect.DeepEqual(saved.CustomRules, gmailRules()) {
		t.Fatal("inactive update lost rules", err)
	}
}

func TestRoutingValidationFailureLeavesFilesUntouched(t *testing.T) {
	m := routingTestManager(t)
	p := profile{ProfileInfo: ProfileInfo{ID: "test", Source: "file"}, YAML: []byte(routingSource)}
	saveRoutingProfile(t, m, p)
	original, _ := os.ReadFile(m.profilePath())
	if err := os.MkdirAll(m.runDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(m.configPath(), []byte("previous generated config")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.binary(), []byte("#!/bin/sh\necho 'level=error msg=\"invalid candidate fixture\"'\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	draft, _ := m.RoutingRules()
	draft.Custom = gmailRules()
	err := m.SaveRoutingRules(t.Context(), draft)
	if err == nil || !strings.Contains(err.Error(), "invalid candidate fixture") {
		t.Fatalf("validation error=%v", err)
	}
	record, _ := os.ReadFile(m.profilePath())
	generated, _ := os.ReadFile(m.configPath())
	if string(record) != string(original) || string(generated) != "previous generated config" {
		t.Fatal("validation failure changed saved files")
	}
	if _, err := os.Stat(m.pendingPath()); !os.IsNotExist(err) {
		t.Fatal("validation failure began application")
	}
	draft.Custom[0].Target = "missing"
	if err := m.SaveRoutingRules(t.Context(), draft); err == nil {
		t.Fatal("accepted unknown target")
	}
}

func TestRoutingUpdateMissingTargetRejected(t *testing.T) {
	m := routingTestManager(t)
	p := profile{ProfileInfo: ProfileInfo{Source: "subscription"}, YAML: []byte(routingSource), CustomRules: gmailRules()}
	saveRoutingProfile(t, m, p)
	next := profile{YAML: []byte("rules:\n  - MATCH,DIRECT\n")}
	if err := m.updateProfileCandidate(t.Context(), p, next, true); err == nil {
		t.Fatal("update removed custom target")
	}
	got, _ := m.readProfile()
	if !reflect.DeepEqual(got, p) {
		t.Fatal("failed update changed profile")
	}
}

func TestRoutingApplyFailureHotRollsBackWithoutDisconnecting(t *testing.T) {
	t.Run("rejected", func(t *testing.T) { testRoutingApplyFailure(t, false) })
	t.Run("late completion", func(t *testing.T) { testRoutingApplyFailure(t, true) })
}

func testRoutingApplyFailure(t *testing.T, late bool) {
	m := routingTestManager(t)
	process := exec.Command("sleep", "60")
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { process.Process.Kill(); process.Wait() }()
	identity, err := processIdentity(process.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var mu sync.Mutex
	var liveBody string
	reloads, disconnects, selections := 0, 0, 0
	selected := "HK"
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	release := make(chan struct{})
	lateDone := make(chan struct{})
	unblock := func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}
	defer unblock()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/version":
			fmt.Fprintf(w, `{"version":%q}`, Version)
		case "/rules":
			fmt.Fprint(w, `{"rules":[{"type":"MATCH","proxy":"DIRECT"}]}`)
		case "/configs":
			if r.Method == http.MethodGet {
				fmt.Fprint(w, `{"mode":"direct","allow-lan":false}`)
				return
			}
			var payload struct{ Payload string }
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			liveBody = payload.Payload
			reloads++
			if strings.Contains(payload.Payload, "imap.gmail.com") {
				selected = "DIRECT"
				if late {
					mu.Unlock()
					cancel()
					<-release
					mu.Lock()
					liveBody = payload.Payload
					close(lateDone)
				}
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case "/proxies":
			fmt.Fprintf(w, `{"proxies":{"🔰 选择节点":{"type":"Selector","now":%q,"all":["HK","DIRECT"]}}}`, selected)
		case "/proxies/🔰 选择节点":
			if r.Method == http.MethodPut {
				selections++
				var choice struct{ Name string }
				json.NewDecoder(r.Body).Decode(&choice)
				selected = choice.Name
			}
			fmt.Fprint(w, `{"type":"Selector","now":"HK","all":["HK","DIRECT"]}`)
		case "/connections":
			disconnects++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer func() { unblock(); server.Close() }()
	state := instance{PID: process.Process.Pid, Identity: identity, Secret: "test-secret", Controller: strings.TrimPrefix(server.URL, "http://"), Proxy: listener.Addr().String()}
	if err := m.writeInstance(state); err != nil {
		t.Fatal(err)
	}
	p := profile{ProfileInfo: ProfileInfo{Source: "file"}, YAML: []byte(routingSource)}
	saveRoutingProfile(t, m, p)
	if err := os.MkdirAll(m.runDir(), 0700); err != nil {
		t.Fatal(err)
	}
	previous, err := m.renderProfile(p, state)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(m.configPath(), previous); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(m.profilePath())
	draft, _ := m.RoutingRules()
	draft.Custom = gmailRules()
	err = m.SaveRoutingRules(ctx, draft)
	if late {
		if err == nil || !strings.Contains(err.Error(), "running state is unconfirmed") {
			t.Fatalf("late application was reported restored: %v", err)
		}
		unblock()
		<-lateDone
	} else if err == nil || !strings.Contains(err.Error(), "restored previous configuration") {
		t.Fatalf("apply error=%v", err)
	}
	after, _ := os.ReadFile(m.profilePath())
	generated, _ := os.ReadFile(m.configPath())
	if string(before) != string(after) || string(generated) != string(previous) {
		t.Fatal("rollback did not restore files")
	}
	if _, err := os.Stat(m.pendingPath()); !os.IsNotExist(err) {
		t.Fatal("rollback journal not cleared")
	}
	owned, err := state.openProcess()
	if err != nil {
		t.Fatal("rollback replaced original process", err)
	}
	owned.close()
	mu.Lock()
	defer mu.Unlock()
	if reloads != 2 || disconnects != 0 || selections != 1 || (!late && strings.Contains(liveBody, "imap.gmail.com")) || !strings.Contains(liveBody, "mode: direct") {
		t.Fatalf("reloads=%d disconnects=%d selections=%d live=%s", reloads, disconnects, selections, liveBody)
	}
}

func realRoutingManager(t *testing.T) Manager {
	t.Helper()
	binary := os.Getenv("TERMIHOMO_TEST_MIHOMO")
	if binary == "" {
		t.Skip("set TERMIHOMO_TEST_MIHOMO for real-core routing verification")
	}
	m := routingTestPaths(t)
	if err := os.MkdirAll(filepath.Dir(m.binary()), 0700); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.binary(), body, 0700); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRoutingWithRealMihomo(t *testing.T) {
	m := realRoutingManager(t)
	// A local HTTP proxy represents the selected HK node. No Gmail account or
	// remote connection is used: its CONNECT destination proves rule selection.
	destinations := make(chan string, 4)
	hk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		destinations <- r.Host
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		conn.SetDeadline(time.Now().Add(10 * time.Second))
		io.Copy(conn, conn)
	}))
	defer hk.Close()
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(hk.URL, "http://"))
	source := fmt.Sprintf("proxies:\n  - name: 香港 HK\n    type: http\n    server: %s\n    port: %s\nproxy-groups:\n  - name: \"\\U0001F530 \\u9009\\u62e9\\u8282\\u70b9\"\n    type: select\n    proxies: [DIRECT, 香港 HK]\nrules:\n  - MATCH,DIRECT\n", host, port)
	p := profile{ProfileInfo: ProfileInfo{ID: "real-test", Source: "file"}, YAML: []byte(source)}
	saveRoutingProfile(t, m, p)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := m.startOn(ctx, "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	state, err := m.runningInstance()
	if err != nil {
		t.Fatal(err)
	}
	if err := state.selectProxy(ctx, "🔰 选择节点", "香港 HK"); err != nil {
		t.Fatal(err)
	}
	draft, err := m.RoutingRules()
	if err != nil {
		t.Fatal(err)
	}
	draft.Custom = gmailRules()
	if err := m.SaveRoutingRules(ctx, draft); err != nil {
		t.Fatal(err)
	}
	after, _ := m.runningInstance()
	if after.PID != state.PID {
		t.Fatal("save restarted Mihomo")
	}
	choices, err := state.proxies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, choice := range choices {
		if choice.Name == "🔰 选择节点" && choice.Now == "香港 HK" {
			found = true
		}
	}
	if !found {
		t.Fatal("save lost HK selection")
	}
	var existing net.Conn
	for _, address := range []string{"imap.gmail.com:993", "smtp.gmail.com:465"} {
		conn := routingConnect(t, state.Proxy, address)
		select {
		case got := <-destinations:
			if got != address {
				t.Fatalf("HK destination %s want %s", got, address)
			}
		case <-ctx.Done():
			t.Fatal("Gmail did not use HK")
		}
		if existing == nil {
			existing = conn
		} else {
			conn.Close()
		}
	}
	defer existing.Close()
	// A hot apply must leave an already established Gmail tunnel usable.
	draft, err = m.RoutingRules()
	if err != nil {
		t.Fatal(err)
	}
	draft.Custom = append(draft.Custom, CustomRule{Type: "DST-PORT", Value: "993", Target: "DIRECT", Disabled: true})
	if err := m.SaveRoutingRules(ctx, draft); err != nil {
		t.Fatal(err)
	}
	if _, err := existing.Write([]byte("x")); err != nil {
		t.Fatal("existing connection closed during apply", err)
	}
	var echo [1]byte
	if _, err := io.ReadFull(existing, echo[:]); err != nil || echo[0] != 'x' {
		t.Fatal("existing connection was interrupted", err)
	}
	existing.Close()

	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "direct") }))
	defer direct.Close()
	conn := routingConnect(t, state.Proxy, strings.TrimPrefix(direct.URL, "http://"))
	conn.Close()
	select {
	case got := <-destinations:
		t.Fatalf("other service incorrectly used HK: %s", got)
	default:
	}
	rules, err := state.rules(ctx)
	if err != nil || len(rules) != 3 || rules[0].Payload != "imap.gmail.com" || rules[1].Payload != "smtp.gmail.com" || rules[2].Proxy != "DIRECT" {
		t.Fatalf("runtime rules=%+v err=%v", rules, err)
	}
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := m.startOn(ctx, "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	restarted, _ := m.runningInstance()
	rules, err = restarted.rules(ctx)
	if err != nil || len(rules) != 3 || rules[0].Payload != "imap.gmail.com" {
		t.Fatalf("restart lost custom rules: %+v %v", rules, err)
	}

}

func routingConnect(t *testing.T, proxy, address string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", proxy, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", address, address)
	response, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		conn.Close()
		t.Fatalf("CONNECT %s: %s", address, response.Status)
	}
	return conn
}

func TestCustomRuleTypesWithRealMihomo(t *testing.T) {
	m := realRoutingManager(t)
	p := profile{YAML: []byte(routingSource), CustomRules: []CustomRule{
		{Type: "DOMAIN", Value: "imap.gmail.com", Target: "🔰 选择节点"},
		{Type: "DOMAIN-SUFFIX", Value: "gmail.com", Target: "DIRECT"},
		{Type: "IP-CIDR", Value: "192.0.2.0/24", Target: "REJECT"},
		{Type: "DST-PORT", Value: "465", Target: "🔰 选择节点"},
	}}
	body, err := m.renderProfile(p, validationInstance())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.validateConfig(t.Context(), body); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingCopyPreservesOptions(t *testing.T) {
	original := "IP-CIDR,192.0.2.0/24,🔰 选择节点,no-resolve"
	rule, err := ParseCustomRule(original)
	if err != nil || !rule.NoResolve || rule.String() != original {
		t.Fatalf("copy=%+v %v", rule, err)
	}
	p := profile{YAML: []byte(routingSource), CustomRules: []CustomRule{rule}}
	body, err := renderProfileWithSettings(p, validationInstance(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var config struct{ Rules []string }
	if err := yaml.Unmarshal(body, &config); err != nil {
		t.Fatal(err)
	}
	if config.Rules[0] != original {
		t.Fatalf("render lost option: %s", config.Rules[0])
	}
	for _, text := range []string{"DOMAIN,a.example,DIRECT,no-resolve", "IP-CIDR,192.0.2.0/24,DIRECT,unknown-option", "IP-CIDR,192.0.2.0/24,DIRECT,no-resolve,extra"} {
		if _, err := ParseCustomRule(text); err == nil {
			t.Fatalf("unsupported options silently accepted: %s", text)
		}
	}
}

func TestRoutingSaveKeepsSubscriptionUpdatedTime(t *testing.T) {
	m := routingTestManager(t)
	updated := time.Now().Add(-2 * time.Hour).UTC()
	p := profile{ProfileInfo: ProfileInfo{Source: "subscription", Updated: updated, Added: updated, Schedule: ProfileSchedule{Minutes: 60}}, YAML: []byte(routingSource)}
	saveRoutingProfile(t, m, p)
	draft, _ := m.RoutingRules()
	draft.Custom = gmailRules()
	if err := m.SaveRoutingRules(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	saved, err := m.readProfile()
	if err != nil || !saved.Updated.Equal(updated) || !saved.UpdateDue(time.Now()) {
		t.Fatalf("rule save postponed subscription update: %v %v", saved.Updated, err)
	}
}

func TestRoutingValidationDoesNotHoldLockAndCanCancel(t *testing.T) {
	m := routingTestManager(t)
	p := profile{ProfileInfo: ProfileInfo{Source: "file"}, YAML: []byte(routingSource)}
	saveRoutingProfile(t, m, p)
	draft, _ := m.RoutingRules()
	draft.Custom = gmailRules()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	validated := false
	ctx = WithProfileProgress(ctx, func(stage string) {
		if stage == "Validating profile" {
			validated = true
			if err := m.withLock(func() error { return nil }); err != nil {
				t.Errorf("validation held instance lock: %v", err)
			}
			cancel()
		}
	})
	err := m.SaveRoutingRules(ctx, draft)
	if !validated || !errors.Is(err, context.Canceled) {
		t.Fatalf("validation did not cancel: %v", err)
	}
	saved, _ := m.readProfile()
	if len(saved.CustomRules) != 0 {
		t.Fatal("cancelled validation saved rules")
	}
}

func TestRoutingValidationRejectsChangedProfile(t *testing.T) {
	m := routingTestManager(t)
	p := profile{ProfileInfo: ProfileInfo{Source: "file", Name: "first"}, YAML: []byte(routingSource)}
	saveRoutingProfile(t, m, p)
	draft, _ := m.RoutingRules()
	draft.Custom = gmailRules()
	ctx := WithProfileProgress(t.Context(), func(stage string) {
		if stage == "Validating profile" {
			p.Name = "externally changed"
			saveRoutingProfile(t, m, p)
		}
	})
	err := m.SaveRoutingRules(ctx, draft)
	if err == nil || !strings.Contains(err.Error(), "Profile changed") {
		t.Fatalf("accepted stale validation: %v", err)
	}
	saved, _ := m.readProfile()
	if len(saved.CustomRules) != 0 || saved.Name != "externally changed" {
		t.Fatal("stale save overwrote profile")
	}
}

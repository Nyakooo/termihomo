package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Nyakooo/termihomo/internal/platform"
)

func TestBatchDelayIncludesFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("url") != "https://example.org/check" {
			t.Error("missing test URL")
		}
		switch r.URL.Path {
		case "/proxies/Node A/delay":
			_, _ = w.Write([]byte(`{"delay":45}`))
		case "/proxies/Node B/delay":
			w.WriteHeader(http.StatusGatewayTimeout)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	state := instance{Controller: strings.TrimPrefix(server.URL, "http://")}
	results, err := state.testNodes(t.Context(), []string{"Node A", "Node B"}, "https://example.org/check", 3*time.Second, nil)
	if err != nil || len(results) != 2 || results["Node A"] != (DelayResult{Milliseconds: 45}) || !results["Node B"].Failed {
		t.Fatalf("results: %+v, error: %v", results, err)
	}
}

func TestProxyGroupsFollowGeneratedConfigOrder(t *testing.T) {
	items := []Proxy{{Name: "GLOBAL"}, {Name: "OneDrive"}, {Name: "选择节点", All: []string{"香港", "日本"}}}
	parseProxyGroupOrder([]byte("proxy-groups:\n- name: 选择节点\n- name: OneDrive\n")).apply(items)
	if items[0].Name != "GLOBAL" || !items[0].Hidden || items[1].Name != "选择节点" || items[2].Name != "OneDrive" {
		t.Fatalf("unexpected group order or visibility: %+v", items)
	}
	if strings.Join(items[1].All, ",") != "香港,日本" {
		t.Fatalf("unexpected member order: %v", items[1].All)
	}
	parseProxyGroupOrder([]byte("proxy-groups:\n- name: 选择节点\n- name: GLOBAL\n- name: OneDrive\n")).apply(items)
	if items[0].Name != "选择节点" || items[1].Name != "GLOBAL" {
		t.Fatalf("explicit GLOBAL should follow its declared position: %+v", items)
	}
}

func TestConfiguredProxiesWithoutMihomo(t *testing.T) {
	m := Manager{Paths: platform.Paths{Config: t.TempDir()}}
	body, err := json.Marshal(profile{YAML: []byte(`
proxies:
  - {name: Hong Kong, type: ss}
  - {name: Japan, type: ss}
proxy-groups:
  - name: Select node
    type: select
    proxies: [Japan, Hong Kong, DIRECT]
  - name: Provider
    type: url-test
    use: [remote]
`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.profilePath(), body, 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := m.ConfiguredProxies()
	if err != nil || len(items) != 6 {
		t.Fatalf("configured proxies: %+v, %v", items, err)
	}
	if items[0].Name != "Select node" || strings.Join(items[0].All, ",") != "Japan,Hong Kong,DIRECT" ||
		!items[1].IsGroup() || items[2].Type != "Shadowsocks" {
		t.Fatalf("unexpected group order or node types: %+v", items)
	}
}

func TestBatchDelayCancelsRequestsAndStopsDispatch(t *testing.T) {
	started := make(chan struct{}, 32)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	state := instance{Controller: strings.TrimPrefix(server.URL, "http://")}
	names := make([]string, 32)
	for i := range names {
		names[i] = fmt.Sprintf("Node %d", i)
	}
	done := make(chan error, 1)
	go func() {
		_, err := state.testNodes(ctx, names, "https://example.org/check", 3*time.Second, func(string, *DelayResult) {
			t.Error("a cancelled request should not publish progress")
		})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("delay requests never reached the controller")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("batch did not finish promptly")
	}
	if got := len(started) + 1; got > 8 {
		t.Fatalf("dispatched more than one wave after cancellation: %d", got)
	}
}

func TestRuntimeGroupOrderCacheFollowsReplacedConfig(t *testing.T) {
	m := Manager{Paths: platform.Paths{Data: t.TempDir()}}.WithRuntimeCache()
	if err := os.MkdirAll(m.runDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, config := range []struct{ yaml, first string }{
		{"proxy-groups: [{name: One}, {name: Two}]", "One"},
		{"proxy-groups: [{name: Two}, {name: One}]", "Two"},
	} {
		if err := atomicWrite(m.configPath(), []byte(config.yaml)); err != nil {
			t.Fatal(err)
		}
		proxies := []Proxy{{Name: "One"}, {Name: "Two"}}
		m.proxyGroupOrder().apply(proxies)
		if proxies[0].Name != config.first {
			t.Fatalf("first group should be %s after replacement: %+v", config.first, proxies)
		}
	}
}

package core

import (
	"strings"
	"testing"

	"github.com/Nyakooo/termihomo/internal/platform"
	"go.yaml.in/yaml/v3"
)

func TestToolOverridesMergeWithoutOpeningImportedListeners(t *testing.T) {
	source := []byte(`dns:
  enable: true
  listen: 0.0.0.0:53
  default-nameserver: [9.9.9.9]
  nameserver: [8.8.8.8]
ipv6: true
`)
	state := instance{Controller: "127.0.0.1:9090", Proxy: "127.0.0.1:7890", Secret: "test"}
	body, err := renderConfigWithSettings(source, state, map[string]any{
		"dns.listen":        "127.0.0.1:1053",
		"dns.nameserver":    []string{"1.1.1.1"},
		"dns.append-system": true,
		"ipv6":              false,
		"app.test-timeout":  2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		DNS struct {
			Listen     string
			Nameserver []string
			Default    []string `yaml:"default-nameserver"`
		}
		IPv6 bool
	}
	if err := yaml.Unmarshal(body, &config); err != nil {
		t.Fatal(err)
	}
	if config.IPv6 || config.DNS.Listen != "127.0.0.1:1053" ||
		strings.Join(config.DNS.Nameserver, ",") != "1.1.1.1,system://" ||
		strings.Join(config.DNS.Default, ",") != "9.9.9.9" ||
		strings.Contains(string(body), "app.test-timeout") {
		t.Fatalf("unexpected override merge: %s", body)
	}
	if _, err := renderConfigWithSettings(source, state, map[string]any{"dns.listen": "0.0.0.0:53"}); err == nil {
		t.Fatal("public DNS binds should be rejected")
	}
}

func TestApplicationToolSettingsRoundTrip(t *testing.T) {
	root := t.TempDir()
	m := Manager{Paths: platform.Paths{Config: root + "/config", Data: root + "/data", Logs: root + "/state/logs"}}
	if err := m.SetToolSetting(t.Context(), "app.test-timeout", "1520"); err != nil {
		t.Fatal(err)
	}
	if _, delay := m.delaySettings(); delay.Milliseconds() != 1520 {
		t.Fatalf("saved timeout has no effect: %v", delay)
	}
	if err := m.SetToolSetting(t.Context(), "app.test-timeout", "oops"); err == nil {
		t.Fatal("invalid values should be rejected")
	}
	if err := m.SetToolSetting(t.Context(), "app.test-timeout", ""); err != nil {
		t.Fatal(err)
	}
	if _, delay := m.delaySettings(); delay.Milliseconds() != 3000 {
		t.Fatalf("reset did not recover the default timeout: %v", delay)
	}
}

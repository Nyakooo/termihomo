package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"go.yaml.in/yaml/v3"
)

func (m Manager) renderConfig(source []byte, state instance) ([]byte, error) {
	settings, err := m.readToolSettings()
	if err != nil {
		return nil, err
	}
	return renderConfigWithSettings(source, state, settings)
}

func renderConfigWithSettings(source []byte, state instance, settings map[string]any) ([]byte, error) {
	config := make(map[string]any)
	if len(source) > 0 {
		decoder := yaml.NewDecoder(bytes.NewReader(source))
		if err := decoder.Decode(&config); err != nil || config == nil || len(config) == 0 {
			return nil, i18n.Message("profile must contain a Mihomo YAML mapping")
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			return nil, i18n.Message("profile must contain a single YAML document")
		}
	}
	for key, value := range settings {
		if child, ok := strings.CutPrefix(key, "dns."); ok {
			dns, _ := config["dns"].(map[string]any)
			if dns == nil {
				dns = make(map[string]any)
				config["dns"] = dns
			}
			if child != "listen" && child != "append-system" {
				dns[child] = value
			}
		} else {
			switch key {
			case "interface-name":
				if value == "" {
					delete(config, key)
				} else {
					config[key] = value
				}
			case "find-process-mode", "log-level", "global-ua", "hosts", "ipv6", "tcp-concurrent", "unified-delay", "geodata-loader":
				config[key] = value
			}
		}
	}
	for _, key := range []string{
		"port", "socks-port", "redir-port", "tproxy-port", "listeners", "tun", "iptables",
		"external-controller-tls", "external-controller-unix", "external-controller-pipe",
		"external-ui", "external-ui-url", "external-ui-name", "external-doh-server",
		"tunnels", "tuic-server", "ss-config", "vmess-config", "tcptun-config", "udptun-config",
	} {
		delete(config, key)
	}
	if dns, ok := config["dns"].(map[string]any); ok {
		delete(dns, "listen")
		if listen, ok := settings["dns.listen"].(string); ok {
			if value, err := parseToolSetting(ToolField{Type: "loopback"}, listen); err == nil {
				dns["listen"] = value
			} else {
				return nil, err
			}
		}
		if appendSystem, _ := settings["dns.append-system"].(bool); appendSystem {
			var nameservers []any
			switch current := dns["nameserver"].(type) {
			case []any:
				nameservers = current
			case []string:
				for _, server := range current {
					nameservers = append(nameservers, server)
				}
			}
			if !slices.ContainsFunc(nameservers, func(server any) bool { value, _ := server.(string); return value == "system://" }) {
				dns["nameserver"] = append(nameservers, "system://")
			}
		}
	}
	_, port, err := net.SplitHostPort(state.Proxy)
	if err != nil {
		return nil, i18n.Message("invalid managed proxy address")
	}
	mixed, err := strconv.Atoi(port)
	if err != nil {
		return nil, i18n.Message("invalid managed proxy port")
	}
	if state.Paused {
		mixed = 0
	}
	config["mixed-port"] = mixed
	config["allow-lan"] = false
	config["bind-address"] = "127.0.0.1"
	config["external-controller"] = state.Controller
	config["secret"] = state.Secret
	if state.ServiceProbe != "" {
		groups, _ := config["proxy-groups"].([]any)
		config["proxy-groups"] = append(groups, map[string]any{
			"name": serviceProbeGroup, "type": "select", "hidden": true,
			"proxies": []string{"REJECT", "DIRECT"}, "include-all": true,
		})
		_, probePort, err := net.SplitHostPort(state.ServiceProbe)
		if err != nil {
			return nil, err
		}
		config["listeners"] = []any{map[string]any{
			"name": serviceProbeGroup, "type": "http", "listen": "127.0.0.1",
			"port": probePort, "proxy": serviceProbeGroup,
			"users": []any{map[string]string{"username": "termihomo", "password": state.Secret}},
		}}
	}
	if _, ok := config["mode"]; !ok {
		config["mode"] = "rule"
	}
	rules, ok := config["rules"].([]any)
	if config["rules"] != nil && !ok {
		return nil, i18n.Message("profile rules must be a YAML sequence")
	}
	if len(rules) == 0 {
		config["rules"] = []string{"MATCH,DIRECT"}
	}
	body, err := yaml.Marshal(config)
	if err != nil {
		return nil, i18n.Message("cannot generate managed Mihomo configuration")
	}
	return body, nil
}

func (m Manager) prepareConfig(ctx context.Context, state instance) error {
	profile, err := m.readProfile()
	if err != nil {
		return err
	}
	body, err := m.renderConfig(profile.YAML, state)
	if err != nil {
		return err
	}
	if err := m.validateConfig(ctx, body); err != nil {
		return err
	}
	return atomicWrite(m.configPath(), body)
}

func (m Manager) validateConfig(ctx context.Context, body []byte) error {
	check, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := m.ensureGeodata(check); err != nil {
		return err
	}
	reportProfileProgress(ctx, "Validating profile")
	file, err := os.CreateTemp(m.runDir(), ".config-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(body); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	cmd := exec.CommandContext(check, m.binary(), "-d", m.runDir(), "-t", "-f", file.Name())
	cmd.Env = mihomoEnv()
	cmd.WaitDelay = time.Second
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(check.Err(), context.DeadlineExceeded) {
			return i18n.Message("Mihomo configuration validation timed out; check GeoIP/GeoSite and rule provider downloads")
		}
		return i18n.Errorf("Mihomo configuration validation failed: %s", validationDetail(output.String(), body, file.Name()))
	}
	return nil
}

var (
	detailURLPattern    = regexp.MustCompile(`https?://[^\s"'<>]+`)
	detailSecretPattern = regexp.MustCompile(`(?m)^secret:[ \t]*([^ \t\r\n]+)`)
)

// validationDetail extracts Mihomo's failure reason while redacting the API
// secret and any URLs (a subscription URL can appear in proxy-provider errors)
// and hiding the random temporary config path.
func validationDetail(output string, body []byte, configName string) string {
	var secret string
	if match := detailSecretPattern.FindSubmatch(body); match != nil {
		secret = string(match[1])
	}
	var parts []string
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "level=error") && !strings.Contains(line, "test failed") {
			continue
		}
		if i := strings.Index(line, "msg="); i >= 0 {
			line = strings.Trim(line[i+4:], "\"")
		}
		parts = append(parts, strings.TrimSpace(line))
	}
	detail := strings.Join(parts, "; ")
	if detail == "" {
		detail = strings.TrimSpace(output)
	}
	if secret != "" {
		detail = strings.ReplaceAll(detail, secret, "***")
	}
	detail = detailURLPattern.ReplaceAllString(detail, "[url]")
	detail = strings.ReplaceAll(detail, configName, "config")
	if len(detail) > 400 {
		detail = detail[:400] + "…"
	}
	return detail
}

func validationInstance() instance {
	return instance{Controller: "127.0.0.1:9090", Proxy: "127.0.0.1:7890", Secret: "validation-only"}
}

func (m Manager) CheckConfig(ctx context.Context) error {
	return m.withLock(func() error {
		current, err := m.readProfile()
		if err != nil {
			return err
		}
		body, err := m.renderConfig(current.YAML, validationInstance())
		if err != nil {
			return err
		}
		return m.validateConfig(ctx, body)
	})
}

// Reload applies the active profile and saved settings without replacing Mihomo.
func (m Manager) Reload(ctx context.Context) error {
	return m.withLock(func() error {
		state, err := m.runningInstance()
		if err != nil {
			return err
		}
		live, err := state.captureLiveConfig(ctx)
		if err != nil {
			return err
		}
		profile, err := m.readProfile()
		if err != nil {
			return err
		}
		candidate, err := m.renderConfig(profile.YAML, state)
		if err != nil {
			return err
		}
		previous, err := os.ReadFile(m.configPath())
		if err != nil {
			return err
		}
		err = m.applyLiveConfig(ctx, state, candidate, live)
		if err == nil {
			err = atomicWrite(m.configPath(), candidate)
		}
		if err != nil {
			recover, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			if restoreErr := m.applyLiveConfig(recover, state, previous, live); restoreErr != nil {
				return errors.Join(err, i18n.Wrap("Could not restore the previous running configuration: %s", restoreErr))
			}
			return i18n.Wrap("Reload failed; restored previous configuration: %s", err)
		}
		return nil
	})
}

type liveConfig struct {
	Mode    string
	TUN     map[string]any
	LAN     bool `json:"allow-lan"`
	Choices []Proxy
}

func (state instance) captureLiveConfig(ctx context.Context) (liveConfig, error) {
	var live liveConfig
	if err := state.request(ctx, http.MethodGet, "/configs", nil, &live); err != nil {
		return live, err
	}
	if live.TUN["enable"] != true || live.TUN["device"] != tunDevice {
		live.TUN = nil
	}
	var err error
	live.Choices, err = state.proxies(ctx)
	return live, err
}

func (m Manager) applyLiveConfig(ctx context.Context, state instance, body []byte, live liveConfig) error {
	payload, err := hotReloadPayload(body, live.Mode, live.TUN)
	if err != nil {
		return err
	}
	if err := m.reloadConfig(ctx, state, payload); err != nil {
		return err
	}
	if live.TUN != nil {
		overview, err := state.overview(ctx)
		if err != nil {
			return err
		}
		if !overview.TUN {
			return i18n.Message("TUN did not change state; check Mihomo logs and network privileges")
		}
	}
	if live.LAN {
		if err := state.setLAN(ctx, true); err != nil {
			return err
		}
	}
	return state.restoreAvailableSelectors(ctx, live.Choices)
}

func hotReloadPayload(body []byte, mode string, tun map[string]any) ([]byte, error) {
	var config map[string]any
	if err := yaml.Unmarshal(body, &config); err != nil || config == nil {
		return nil, i18n.Message("cannot read managed Mihomo configuration")
	}
	if mode != "" {
		config["mode"] = strings.ToLower(mode)
	}
	// Startup YAML omits TUN. Preserve its settings in the controller payload.
	if tun != nil {
		config["tun"] = tun
	}
	body, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"payload": string(body)})
}

func (m Manager) BackupConfig() (string, error) {
	backup := m.configPath() + ".manual.bak"
	err := m.withLock(func() error {
		body, err := os.ReadFile(m.configPath())
		if err != nil {
			return i18n.Message("no generated configuration is available to back up")
		}
		return atomicWrite(backup, body)
	})
	return backup, err
}

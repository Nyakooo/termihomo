package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"go.yaml.in/yaml/v3"
)

const startOnBootKey = "app.start-on-boot"
const SystemTrayKey = "app.system-tray"

type ToolField struct {
	Key, Label, Type string
}

var GeneralToolFields = []ToolField{
	{"app.auto-run", "Auto-run", "bool"},
	{startOnBootKey, "Start on boot", "bool"},
	{"mixed-port", "Port", "port"},
	{"app.close-connections", "Close connections on node change", "bool"},
	{"app.test-url", "Test URL", "url"},
	{"app.test-timeout", "Test timeout (ms)", "timeout"},
}

func init() {
	if runtime.GOOS != "android" {
		GeneralToolFields = append(GeneralToolFields, ToolField{SystemTrayKey, "System tray", "bool"})
	}
}

var advancedToolFields = []ToolField{
	{"find-process-mode", "Process detection", "always|strict|off"},
	{"log-level", "Log level", "silent|error|warning|info|debug"},
	{"global-ua", "User-Agent", "text"},
	{"hosts", "Hosts", "map"},
	{"dns.append-system", "Append system DNS", "bool"},
	{"ipv6", "IPv6", "bool"},
	{"tcp-concurrent", "Concurrent TCP", "bool"},
	{"unified-delay", "Unified delay", "bool"},
	{"geodata-loader", "GeoData loader", "standard|memconservative"},
}

var DNSToolFields = []ToolField{
	{"dns.enable", "DNS enabled", "bool"},
	{"dns.listen", "DNS listen", "loopback"},
	{"dns.ipv6", "IPv6", "bool"},
	{"dns.enhanced-mode", "DNS mode", "normal|fake-ip|redir-host"},
	{"dns.default-nameserver", "Default nameservers", "list"},
	{"dns.nameserver", "Nameservers", "list"},
	{"dns.proxy-server-nameserver", "Proxy server nameservers", "list"},
	{"dns.direct-nameserver", "Direct nameservers", "list"},
	{"dns.nameserver-policy", "Nameserver policy", "map"},
	{"dns.proxy-server-nameserver-policy", "Proxy server nameserver policy", "map"},
	{"dns.direct-nameserver-follow-policy", "Direct nameservers follow policy", "bool"},
	{"dns.fallback", "Fallback nameservers", "list"},
	{"dns.fallback-filter", "Fallback filter", "map"},
	{"dns.fake-ip-range", "Fake-IP range", "text"},
	{"dns.fake-ip-range6", "Fake-IP IPv6 range", "text"},
	{"dns.fake-ip-filter", "Fake-IP filter", "list"},
	{"dns.fake-ip-filter-mode", "Fake-IP filter mode", "blacklist|whitelist|rule"},
	{"dns.cache-algorithm", "Cache algorithm", "lru|arc"},
	{"dns.cache-max-size", "Cache size", "number"},
	{"dns.respect-rules", "Respect DNS rules", "bool"},
	{"dns.use-hosts", "Use hosts", "bool"},
	{"dns.use-system-hosts", "Use system hosts", "bool"},
	{"dns.prefer-h3", "Prefer HTTP/3", "bool"},
}

var NetworkToolFields = []ToolField{
	{"app.tun-stack", "Stack mode", "system|gvisor|mixed"},
	{"interface-name", "Outbound interface", "interface"},
}

func (m Manager) toolSettingsPath() string {
	return filepath.Join(m.Paths.Config, "overrides.yaml")
}

func (m Manager) readToolSettings() (map[string]any, error) {
	settings := make(map[string]any)
	body, err := os.ReadFile(m.toolSettingsPath())
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(body, &settings); err != nil {
		return nil, i18n.Message("Invalid settings YAML")
	}
	if settings == nil {
		settings = make(map[string]any)
	}
	return settings, nil
}

func (m Manager) TrayEnabled() bool {
	settings, err := m.readToolSettings()
	return err == nil && settings[SystemTrayKey] == true
}

func (m Manager) ToolSettings() (map[string]string, error) {
	raw, err := m.readToolSettings()
	if err != nil {
		return nil, err
	}
	values := make(map[string]string, len(raw))
	for key, value := range raw {
		switch v := value.(type) {
		case string:
			values[key] = v
		default:
			data, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			values[key] = string(data)
		}
	}
	if runtime.GOOS == "android" {
		values[startOnBootKey] = "Unavailable"
		return values, nil
	}
	enabled, err := m.startOnBoot()
	if err != nil {
		return values, err
	}
	if err := m.syncDesktopStartup(enabled); err != nil {
		return values, err
	}
	values[startOnBootKey] = strconv.FormatBool(enabled)
	return values, nil
}

func (m Manager) SetToolSetting(ctx context.Context, key, input string) error {
	var field ToolField
	for _, fields := range [][]ToolField{GeneralToolFields, advancedToolFields, DNSToolFields, NetworkToolFields} {
		for _, candidate := range fields {
			if candidate.Key == key {
				field = candidate
			}
		}
	}
	if field.Key == "" {
		return i18n.Message("Unknown setting")
	}
	input = strings.TrimSpace(input)
	var value any
	if input != "" {
		var err error
		value, err = parseToolSetting(field, input)
		if err != nil {
			return err
		}
	}
	return m.withLock(func() error {
		if key == startOnBootKey {
			if err := m.setStartOnBoot(ctx, value == true); err != nil {
				return err
			}
			return m.syncDesktopStartup(value == true)
		}
		settings, err := m.readToolSettings()
		if err != nil {
			return err
		}
		if key == "interface-name" && input == "" {
			settings[key] = ""
		} else if input == "" {
			delete(settings, key)
		} else {
			settings[key] = value
		}
		if !strings.HasPrefix(key, "app.") {
			current, err := m.readProfile()
			if err != nil {
				return err
			}
			body, err := renderConfigWithSettings(current.YAML, instance{
				Controller: "127.0.0.1:9090", Proxy: "127.0.0.1:7890", Secret: "validation-only",
			}, settings)
			if err != nil {
				return err
			}
			if err := m.validateConfig(ctx, body); err != nil {
				return err
			}
		}
		return m.writeToolSettings(settings)
	})
}

func (m Manager) writeToolSettings(settings map[string]any) error {
	data, err := yaml.Marshal(settings)
	if err != nil {
		return err
	}
	return atomicWrite(m.toolSettingsPath(), data)
}

func parseToolSetting(field ToolField, input string) (any, error) {
	invalid := i18n.Message("Invalid setting value")
	switch field.Type {
	case "interface":
		if input == tunDevice {
			return nil, invalid
		}
		if _, err := net.InterfaceByName(input); err != nil {
			return nil, i18n.Message("Network interface not found")
		}
		return input, nil
	case "text":
		return input, nil
	case "loopback":
		host, port, err := net.SplitHostPort(input)
		ip := net.ParseIP(host)
		n, portErr := strconv.Atoi(port)
		if err != nil || ip == nil || !ip.IsLoopback() || portErr != nil || n < 1 || n > 65535 {
			return nil, invalid
		}
		return input, nil
	case "url":
		address, err := url.Parse(input)
		if err != nil || address.Host == "" || (address.Scheme != "http" && address.Scheme != "https") {
			return nil, invalid
		}
		return input, nil
	case "port", "timeout", "number":
		n, err := strconv.Atoi(input)
		if err != nil || n < 0 || field.Type == "port" && (n == 0 || n > 65535) ||
			field.Type == "timeout" && (n < 100 || n > 30000) {
			return nil, invalid
		}
		return n, nil
	case "bool":
		switch input {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
	case "list":
		var items []string
		if err := yaml.Unmarshal([]byte(input), &items); err == nil && items != nil {
			return items, nil
		}
	case "map":
		var items map[string]any
		if err := yaml.Unmarshal([]byte(input), &items); err == nil && items != nil {
			return items, nil
		}
	default:
		for _, allowed := range strings.Split(field.Type, "|") {
			if input == allowed {
				return input, nil
			}
		}
	}
	return nil, invalid
}

func (m Manager) delaySettings() (string, time.Duration) {
	settings, _ := m.readToolSettings()
	target, _ := settings["app.test-url"].(string)
	if target == "" {
		target = "https://www.gstatic.com/generate_204"
	}
	ms, _ := settings["app.test-timeout"].(int)
	if ms < 100 || ms > 30000 {
		ms = 3000
	}
	return target, time.Duration(ms) * time.Millisecond
}

type DNSAnswer struct {
	Name string
	Type uint16
	TTL  uint32
	Data string
}

type DNSResult struct {
	Status    int
	Answer    []DNSAnswer
	Authority []DNSAnswer
}

func (m Manager) QueryDNS(ctx context.Context, name, queryType string) (DNSResult, error) {
	var result DNSResult
	if name == "" || len(name) > 253 || strings.ContainsAny(name, " /\t\r\n") {
		return result, i18n.Message("Invalid DNS name")
	}
	switch queryType {
	case "A", "AAAA", "CNAME", "MX", "TXT", "NS", "PTR", "SOA", "SRV", "HTTPS", "SVCB":
	default:
		return result, i18n.Message("Unsupported DNS record type")
	}
	state, err := m.runningInstance()
	if err != nil {
		return result, err
	}
	params := url.Values{"name": {name}, "type": {queryType}}
	err = state.requestWithin(ctx, 8*time.Second, http.MethodGet, "/dns/query?"+params.Encode(), nil, &result)
	return result, err
}

func (m Manager) FlushDNS(ctx context.Context, fakeIP bool) error {
	state, err := m.runningInstance()
	if err != nil {
		return err
	}
	cache := "dns"
	if fakeIP {
		cache = "fakeip"
	}
	return state.request(ctx, http.MethodPost, fmt.Sprintf("/cache/%s/flush", cache), nil, nil)
}

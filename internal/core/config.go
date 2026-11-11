package core

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"

	"go.yaml.in/yaml/v3"
)

func renderConfig(source []byte, state instance) ([]byte, error) {
	config := make(map[string]any)
	if len(source) > 0 {
		decoder := yaml.NewDecoder(bytes.NewReader(source))
		if err := decoder.Decode(&config); err != nil || config == nil || len(config) == 0 {
			return nil, errors.New("profile must contain a Mihomo YAML mapping")
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			return nil, errors.New("profile must contain a single YAML document")
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
	}
	_, port, err := net.SplitHostPort(state.Proxy)
	if err != nil {
		return nil, errors.New("invalid managed proxy address")
	}
	mixed, err := strconv.Atoi(port)
	if err != nil {
		return nil, errors.New("invalid managed proxy port")
	}
	config["mixed-port"] = mixed
	config["allow-lan"] = false
	config["bind-address"] = "127.0.0.1"
	config["external-controller"] = state.Controller
	config["secret"] = state.Secret
	if _, ok := config["mode"]; !ok {
		config["mode"] = "rule"
	}
	rules, ok := config["rules"].([]any)
	if config["rules"] != nil && !ok {
		return nil, errors.New("profile rules must be a YAML sequence")
	}
	if len(rules) == 0 {
		config["rules"] = []string{"MATCH,DIRECT"}
	}
	body, err := yaml.Marshal(config)
	if err != nil {
		return nil, errors.New("cannot generate managed Mihomo configuration")
	}
	return body, nil
}

func (m Manager) prepareConfig(ctx context.Context, state instance) error {
	profile, err := m.readProfile()
	if err != nil {
		return err
	}
	body, err := renderConfig(profile.YAML, state)
	if err != nil {
		return err
	}
	if err := m.validateConfig(ctx, body); err != nil {
		return err
	}
	return atomicWrite(m.configPath(), body)
}

func (m Manager) validateConfig(ctx context.Context, body []byte) error {
	if err := os.MkdirAll(m.runDir(), 0o700); err != nil {
		return err
	}
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
	check, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(check, m.binary(), "-d", m.runDir(), "-t", "-f", file.Name())
	cmd.Env = mihomoEnv()
	if err := cmd.Run(); err != nil {
		return errors.New("Mihomo configuration validation failed")
	}
	return nil
}

func (m Manager) CheckConfig(ctx context.Context) error {
	return m.withLock(func() error {
		current, err := m.readProfile()
		if err != nil {
			return err
		}
		body, err := renderConfig(current.YAML, instance{
			Controller: "127.0.0.1:9090", Proxy: "127.0.0.1:7890", Secret: "validation-only",
		})
		if err != nil {
			return err
		}
		return m.validateConfig(ctx, body)
	})
}

func (m Manager) BackupConfig() (string, error) {
	backup := m.configPath() + ".manual.bak"
	err := m.withLock(func() error {
		body, err := os.ReadFile(m.configPath())
		if err != nil {
			return errors.New("no generated configuration is available to back up")
		}
		return atomicWrite(backup, body)
	})
	return backup, err
}

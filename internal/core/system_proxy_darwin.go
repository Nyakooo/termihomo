package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

var macProxyTypes = []struct{ read, write, toggle string }{
	{"-getwebproxy", "-setwebproxy", "-setwebproxystate"},
	{"-getsecurewebproxy", "-setsecurewebproxy", "-setsecurewebproxystate"},
	{"-getsocksfirewallproxy", "-setsocksfirewallproxy", "-setsocksfirewallproxystate"},
}

func networkSetup(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "/usr/sbin/networksetup", args...).Output()
	return strings.TrimSpace(string(out)), err
}

func macNetworkService(ctx context.Context) (string, error) {
	for _, family := range []string{"IPv4", "IPv6"} {
		show := "show State:/Network/Global/" + family + "\nquit\n"
		cmd := exec.CommandContext(ctx, "/usr/sbin/scutil")
		cmd.Stdin = strings.NewReader(show)
		out, err := cmd.Output()
		if err != nil {
			return "", err
		}
		device := macProperty(string(out), "PrimaryInterface")
		if device == "" {
			continue
		}
		services, err := networkSetup(ctx, "-listnetworkserviceorder")
		if err != nil {
			return "", err
		}
		name := ""
		for line := range strings.SplitSeq(services, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "(Hardware Port:") {
				if strings.HasSuffix(line, "Device: "+device+")") && name != "" {
					return name, nil
				}
			} else if _, candidate, ok := strings.Cut(line, ") "); ok && strings.HasPrefix(line, "(") {
				name = candidate
			}
		}
	}
	return "", i18n.Message("System proxy settings unavailable")
}

func macProperty(body, key string) string {
	for line := range strings.SplitSeq(body, "\n") {
		if k, value, ok := strings.Cut(strings.TrimSpace(line), ":"); ok && strings.TrimSpace(k) == key {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func macProxySnapshot(ctx context.Context) ([]string, []string, bool, error) {
	out, err := exec.CommandContext(ctx, "/usr/sbin/scutil", "--proxy").Output()
	if err != nil {
		return nil, nil, false, err
	}
	body := string(out)
	var values, exclusions []string
	enabled := false
	for _, prefix := range []string{"HTTP", "HTTPS", "SOCKS", "ProxyAutoConfig", "ProxyAutoDiscovery"} {
		on := macProperty(body, prefix+"Enable")
		enabled = enabled || on == "1"
		values = append(values, on, macProperty(body, prefix+"Proxy"), macProperty(body, prefix+"Port"))
	}
	values = append(values, macProperty(body, "ProxyAutoConfigURLString"))
	inside := false
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ExceptionsList :") {
			inside = true
			continue
		}
		if inside && line == "}" {
			break
		}
		if inside {
			index, host, ok := strings.Cut(line, ":")
			if _, err := strconv.Atoi(strings.TrimSpace(index)); ok && err == nil {
				exclusions = append(exclusions, strings.TrimSpace(host))
			}
		}
	}
	return values, exclusions, enabled, nil
}

func (m Manager) SystemProxy(ctx context.Context) SystemProxy {
	values, exclusions, enabled, err := macProxySnapshot(ctx)
	if err != nil {
		return SystemProxy{}
	}
	var exclusionsErr error
	if exclusions == nil {
		var service string
		service, exclusionsErr = macNetworkService(ctx)
		if exclusionsErr == nil {
			exclusions, exclusionsErr = macProxyExclusions(ctx, service)
		}
	}
	owner, _ := m.proxyOwnership()
	return SystemProxy{Available: true, Enabled: enabled, Owned: slices.Equal(values, owner.Applied),
		Revision: proxyRevision(values), Exclusions: exclusions, ExclusionsErr: exclusionsErr}
}

func macProxyExclusions(ctx context.Context, service string) ([]string, error) {
	out, err := networkSetup(ctx, "-getproxybypassdomains", service)
	if err != nil || out == "" || strings.HasPrefix(out, "There aren't any bypass domains set") {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

func macSetupCommand(args ...string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	return "/usr/sbin/networksetup " + strings.Join(quoted, " ")
}

func macChanges(ctx context.Context, commands []string, rollback ...string) error {
	if len(commands) == 0 {
		return nil
	}
	script := strings.Join(commands, " && ")
	if len(rollback) > 0 {
		script = "if " + script + "; then exit 0; else result=$?; fi\n" +
			strings.Join(rollback, "\n") + "\nexit \"$result\""
	}
	return runMacAdmin(ctx, script)
}

func (m Manager) SetProxyExclusions(ctx context.Context, before, after []string) error {
	return m.withLock(func() error {
		service, err := macNetworkService(ctx)
		if err != nil {
			return err
		}
		current, err := macProxyExclusions(ctx, service)
		if err != nil {
			return err
		}
		if !slices.Equal(current, before) {
			return i18n.Message("System proxy exclusions changed. Review the list and try again.")
		}
		if len(after) == 0 {
			after = []string{"Empty"}
		}
		return macChanges(ctx, []string{macSetupCommand(append([]string{"-setproxybypassdomains", service}, after...)...)})
	})
}

func readMacProxy(ctx context.Context, service string) ([]string, error) {
	before := []string{service}
	for _, setting := range macProxyTypes {
		out, err := networkSetup(ctx, setting.read, service)
		if err != nil {
			return nil, err
		}
		if macProperty(out, "Authenticated Proxy Enabled") == "1" {
			return nil, i18n.Message("System proxy settings unavailable")
		}
		before = append(before, macProperty(out, "Enabled"), macProperty(out, "Server"), macProperty(out, "Port"))
	}
	return before, nil
}

func restoreMacCommands(before []string) []string {
	service := before[0]
	var commands []string
	for i, setting := range macProxyTypes {
		enabled, host, port := before[1+i*3], before[2+i*3], before[3+i*3]
		if host != "" && port != "" && port != "0" {
			commands = append(commands, macSetupCommand(setting.write, service, host, port))
		}
		state := "off"
		if enabled == "Yes" {
			state = "on"
		}
		commands = append(commands, macSetupCommand(setting.toggle, service, state))
	}
	return commands
}

func (m Manager) enableSystemProxy(ctx context.Context, address string, replace bool) error {
	service, err := macNetworkService(ctx)
	if err != nil {
		return err
	}
	values, _, enabled, err := macProxySnapshot(ctx)
	if err != nil {
		return err
	}
	owner, path := m.proxyOwnership()
	if enabled && !replace {
		return nil
	}
	before, err := readMacProxy(ctx, service)
	if err != nil {
		return err
	}
	if slices.Equal(values, owner.Applied) && len(owner.Before) == len(before) && owner.Before[0] == service {
		before = owner.Before
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	var commands []string
	for _, setting := range macProxyTypes {
		commands = append(commands, macSetupCommand(setting.write, service, host, port))
	}
	if err := macChanges(ctx, commands, restoreMacCommands(before)...); err != nil {
		return err
	}
	after, _, _, err := macProxySnapshot(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(proxyOwnership{Before: before, Applied: after})
	if err == nil {
		err = atomicWrite(path, body)
	}
	if err != nil {
		return errors.Join(err, macChanges(ctx, restoreMacCommands(before)))
	}
	return nil
}

func (m Manager) disableSystemProxy(ctx context.Context, revision string) error {
	owner, path := m.proxyOwnership()
	if len(owner.Before) == 0 && revision == "" {
		return nil
	}
	service, err := macNetworkService(ctx)
	if err != nil {
		return err
	}
	values, _, _, err := macProxySnapshot(ctx)
	if err != nil {
		return err
	}
	if revision != "" && revision != proxyRevision(values) {
		return i18n.Message("System proxy changed; check its current settings and try again")
	}
	var commands []string
	if len(owner.Before) == 1+3*len(macProxyTypes) && owner.Before[0] == service && slices.Equal(values, owner.Applied) {
		commands = restoreMacCommands(owner.Before)
	} else if revision != "" {
		for _, setting := range macProxyTypes {
			commands = append(commands, macSetupCommand(setting.toggle, service, "off"))
		}
		commands = append(commands, macSetupCommand("-setautoproxystate", service, "off"), macSetupCommand("-setproxyautodiscovery", service, "off"))
	} else if len(owner.Before) > 0 && owner.Before[0] != service {
		return fmt.Errorf("the active network service changed since Termihomo enabled the system proxy")
	}
	if err := macChanges(ctx, commands); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

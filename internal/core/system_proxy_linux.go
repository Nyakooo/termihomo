package core

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

const proxySchema = "org.gnome.system.proxy"
const proxyExclusionsIndex = 8

type proxySetting struct{ schema, key string }

var proxySettings = []proxySetting{
	{proxySchema + ".http", "host"}, {proxySchema + ".http", "port"},
	{proxySchema + ".https", "host"}, {proxySchema + ".https", "port"},
	{proxySchema + ".socks", "host"}, {proxySchema + ".socks", "port"},
	{proxySchema + ".http", "use-authentication"},
	{proxySchema, "autoconfig-url"}, {proxySchema, "ignore-hosts"}, {proxySchema, "use-same-proxy"},
	{proxySchema + ".ftp", "host"}, {proxySchema + ".ftp", "port"}, {proxySchema, "mode"},
}

func proxyCommand(ctx context.Context, verb string, setting proxySetting, values ...string) (string, error) {
	args := append([]string{verb, setting.schema, setting.key}, values...)
	out, err := exec.CommandContext(ctx, "gsettings", args...).Output()
	return strings.TrimSpace(string(out)), err
}

func readSystemProxy(ctx context.Context) ([]string, error) {
	if runtime.GOOS == "android" || os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" || !strings.Contains(strings.ToLower(os.Getenv("XDG_CURRENT_DESKTOP")), "gnome") {
		return nil, os.ErrNotExist
	}
	out, err := exec.CommandContext(ctx, "gsettings", "list-recursively", proxySchema).Output()
	if err != nil {
		return nil, err
	}
	values := make([]string, len(proxySettings))
	for line := range strings.SplitSeq(string(out), "\n") {
		schema, rest, _ := strings.Cut(line, " ")
		key, value, _ := strings.Cut(rest, " ")
		for i, setting := range proxySettings {
			if setting.schema == schema && setting.key == key {
				values[i] = value
				break
			}
		}
	}
	for _, value := range values {
		if value == "" {
			return nil, i18n.Message("System proxy settings unavailable")
		}
	}
	return values, nil
}

func (m Manager) SystemProxy(ctx context.Context) SystemProxy {
	values, err := readSystemProxy(ctx)
	if err != nil {
		return SystemProxy{}
	}
	owner, _ := m.proxyOwnership()
	exclusions, exclusionsErr := parseProxyExclusions(values[proxyExclusionsIndex])
	return SystemProxy{Available: true, Enabled: values[len(values)-1] != "'none'", Owned: ownsProxy(values, owner.Applied), Revision: proxyRevision(values),
		Exclusions: exclusions, ExclusionsErr: exclusionsErr}
}

// Exclusions belong to the desktop and may be edited while Termihomo owns the proxy.
func ownsProxy(current, applied []string) bool {
	return len(current) == len(proxySettings) && len(applied) == len(proxySettings) &&
		slices.Equal(current[:proxyExclusionsIndex], applied[:proxyExclusionsIndex]) &&
		slices.Equal(current[proxyExclusionsIndex+1:], applied[proxyExclusionsIndex+1:])
}

func parseProxyExclusions(value string) ([]string, error) {
	value = strings.TrimSpace(strings.TrimPrefix(value, "@as "))
	invalid := i18n.Message("Could not read system proxy exclusions")
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil, invalid
	}
	var hosts []string
	for value = strings.TrimSpace(value[1 : len(value)-1]); value != ""; {
		quote := value[0]
		if quote != '\'' && quote != '"' {
			return nil, invalid
		}
		value = value[1:]
		var host strings.Builder
		for value != "" && value[0] != quote {
			r, _, rest, err := strconv.UnquoteChar(value, quote)
			if err != nil {
				return nil, invalid
			}
			host.WriteRune(r)
			value = rest
		}
		if value == "" {
			return nil, invalid
		}
		hosts = append(hosts, host.String())
		value = strings.TrimSpace(value[1:])
		if value != "" {
			if value[0] != ',' {
				return nil, invalid
			}
			value = strings.TrimSpace(value[1:])
		}
	}
	return hosts, nil
}

func (m Manager) SetProxyExclusions(ctx context.Context, before, after []string) error {
	return m.withLock(func() error {
		values, err := readSystemProxy(ctx)
		if err != nil {
			return i18n.Message("System proxy settings unavailable")
		}
		current, err := parseProxyExclusions(values[proxyExclusionsIndex])
		if err != nil {
			return err
		}
		if !slices.Equal(current, before) {
			return i18n.Message("System proxy exclusions changed. Review the list and try again.")
		}
		quoted := make([]string, len(after))
		for i, host := range after {
			quoted[i] = strconv.Quote(host)
		}
		setting := proxySettings[proxyExclusionsIndex]
		if _, err = proxyCommand(ctx, "set", setting, "["+strings.Join(quoted, ", ")+"]"); err != nil {
			return err
		}
		observed, err := proxyCommand(ctx, "get", setting)
		if err != nil {
			return err
		}
		saved, err := parseProxyExclusions(observed)
		if err != nil {
			return err
		}
		if !slices.Equal(saved, after) {
			return i18n.Message("Could not save system proxy exclusions")
		}
		return nil
	})
}

func equalProxy(a, b []string) bool {
	return len(a) == len(proxySettings) && len(b) == len(proxySettings) && slices.Equal(a, b)
}

func writeSystemProxy(ctx context.Context, before, after []string) error {
	var changed []int
	for i, setting := range proxySettings {
		if before[i] == after[i] {
			continue
		}
		if _, err := proxyCommand(ctx, "set", setting, after[i]); err != nil {
			var rollback error
			for j := len(changed) - 1; j >= 0; j-- {
				index := changed[j]
				_, restore := proxyCommand(ctx, "set", proxySettings[index], before[index])
				rollback = errors.Join(rollback, restore)
			}
			return errors.Join(err, rollback)
		}
		changed = append(changed, i)
	}
	return nil
}

func (m Manager) enableSystemProxy(ctx context.Context, address string, replace bool) error {
	before, err := readSystemProxy(ctx)
	if err != nil {
		return err
	}
	owner, path := m.proxyOwnership()
	owned := ownsProxy(before, owner.Applied)
	if !owned && !replace && before[len(before)-1] != "'none'" {
		return nil
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	host = "'" + host + "'"
	applied := slices.Clone(before)
	copy(applied, []string{host, port, host, port, host, port, "false"})
	applied[len(applied)-1] = "'manual'"
	original := before
	if owned && len(owner.Before) == len(proxySettings) {
		original = owner.Before
	}
	if err := writeSystemProxy(ctx, before, applied); err != nil {
		return err
	}
	observed, err := readSystemProxy(ctx)
	if err != nil {
		return err
	}
	if !equalProxy(observed, applied) {
		return i18n.Message("System proxy changed; check its current settings and try again")
	}
	body, err := json.Marshal(proxyOwnership{Before: original, Applied: applied})
	if err == nil {
		err = atomicWrite(path, body)
	}
	if err != nil {
		return errors.Join(err, writeSystemProxy(ctx, applied, before))
	}
	return nil
}

func (m Manager) disableSystemProxy(ctx context.Context, externalRevision string) error {
	owner, path := m.proxyOwnership()
	current, err := readSystemProxy(ctx)
	if err != nil {
		return err
	}
	if externalRevision != "" && proxyRevision(current) != externalRevision {
		return i18n.Message("System proxy changed; check its current settings and try again")
	}
	expected := slices.Clone(current)
	owned := ownsProxy(current, owner.Applied) && len(owner.Before) == len(proxySettings)
	if owned {
		copy(expected, owner.Before)
		expected[proxyExclusionsIndex] = current[proxyExclusionsIndex]
		if err := writeSystemProxy(ctx, current, expected); err != nil {
			return err
		}
	} else if externalRevision != "" {
		expected[len(expected)-1] = "'none'"
		if _, err := proxyCommand(ctx, "set", proxySettings[len(proxySettings)-1], "'none'"); err != nil {
			return err
		}
	}
	if owned || externalRevision != "" {
		observed, err := readSystemProxy(ctx)
		if err != nil {
			return err
		}
		if !equalProxy(observed, expected) {
			return i18n.Message("System proxy changed; check its current settings and try again")
		}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

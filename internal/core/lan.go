package core

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

// LocalProxyNetworks returns private IPv4 addresses and their on-link networks.
func LocalProxyNetworks() (addresses, networks []string) {
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			prefix, err := netip.ParsePrefix(addr.String())
			if err != nil || !prefix.Addr().Is4() || !prefix.Addr().IsPrivate() {
				continue
			}
			network := prefix.Masked()
			// An unusually broad mask must never permit public client addresses.
			bits := 16
			if prefix.Addr().As4()[0] == 10 {
				bits = 8
			} else if prefix.Addr().As4()[0] == 172 {
				bits = 12
			}
			if network.Bits() < bits {
				continue
			}
			addresses = append(addresses, prefix.Addr().String())
			networks = append(networks, network.String())
		}
	}
	return
}

func (m Manager) SetLAN(ctx context.Context, enabled bool) error {
	return m.withLock(func() error {
		state, err := m.runningInstance()
		if err != nil {
			return err
		}
		if state.Paused && enabled {
			return i18n.Message("Start Mihomo to apply changes")
		}
		if !state.Paused {
			if err := state.setLAN(ctx, enabled); err != nil {
				return err
			}
		}
		if err := m.saveNetworkPreference(lanPreference, enabled); err != nil {
			return err
		}
		if state.Paused {
			state.ResumeLAN = enabled
			return m.writeInstance(state)
		}
		return nil
	})
}

func (state instance) setLAN(ctx context.Context, enabled bool) error {
	addresses, networks := LocalProxyNetworks()
	if enabled && len(addresses) == 0 {
		return i18n.Message("No private IPv4 network is available")
	}
	check := func(ctx context.Context, address string) bool {
		conn, err := (&net.Dialer{Timeout: 300 * time.Millisecond}).DialContext(ctx, "tcp4", address)
		if err != nil {
			return false
		}
		conn.Close()
		return true
	}
	_, port, err := net.SplitHostPort(state.Proxy)
	if err != nil {
		return err
	}
	apply := func(ctx context.Context, on bool) error {
		bind := "127.0.0.1"
		allowed := []string{"127.0.0.0/8"}
		if on {
			bind = "*"
			allowed = append(allowed, networks...)
		}
		err := state.updateRuntime(ctx, map[string]any{
			"allow-lan": on, "bind-address": bind,
			"lan-allowed-ips": allowed, "lan-disallowed-ips": []string{},
		}, func(actual Overview) bool { return actual.LAN == on })
		if err != nil {
			return err
		}
		if !check(ctx, state.Proxy) {
			return i18n.Message("The local proxy listener is unavailable")
		}
		for _, ip := range addresses {
			if check(ctx, net.JoinHostPort(ip, port)) != on {
				return i18n.Message("The LAN proxy listener did not change state")
			}
		}
		return nil
	}
	err = apply(ctx, enabled)
	if err != nil && enabled {
		rollback, cancel := context.WithTimeout(context.WithoutCancel(ctx), 4*time.Second)
		defer cancel()
		return errors.Join(err, apply(rollback, false))
	}
	return err
}

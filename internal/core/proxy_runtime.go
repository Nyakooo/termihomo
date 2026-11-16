package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

const (
	tunPreference         = "app.tun-enabled"
	systemProxyPreference = "app.system-proxy-enabled"
)

func (m Manager) saveNetworkPreference(key string, enabled bool) error {
	settings, err := m.readToolSettings()
	if err != nil {
		return err
	}
	if value, ok := settings[key].(bool); ok && value == enabled {
		return nil
	}
	settings[key] = enabled
	return m.writeToolSettings(settings)
}

func (m Manager) applyNetworkPreferences(state *instance) error {
	settings, err := m.readToolSettings()
	if err != nil {
		return err
	}
	if enabled, ok := settings[tunPreference].(bool); ok {
		state.ResumeTUN = enabled
	}
	if enabled, ok := settings[systemProxyPreference].(bool); ok {
		state.ResumeSystemProxy = enabled
	}
	return nil
}

// EnsureProxyController makes the active configuration usable without opening
// an inbound proxy listener.
func (m Manager) EnsureProxyController(ctx context.Context) error {
	return m.withLock(func() error {
		return m.startOnMode(ctx, "", true)
	})
}

// PauseProxy releases inbound proxy listeners while retaining the live proxies,
// provider data and selector choices inside Mihomo.
func (m Manager) PauseProxy(ctx context.Context) error {
	return m.withLock(func() error {
		state, err := m.runningInstance()
		if err != nil {
			return err
		}
		if !state.Paused {
			overview, err := state.overview(ctx)
			if err != nil {
				return err
			}
			state.ResumeTUN = overview.TUNConfigured
			owner, _ := m.proxyOwnership()
			state.ResumeSystemProxy = len(owner.Applied) > 0 && m.SystemProxy(ctx).Owned
		}
		if err := m.setProxyPaused(ctx, state, true); err != nil {
			return err
		}
		return state.request(ctx, http.MethodDelete, "/connections", nil, nil)
	})
}

func (m Manager) setProxyPaused(ctx context.Context, state instance, paused bool) error {
	if state.Paused == paused {
		return nil
	}
	_, portText, err := net.SplitHostPort(state.Proxy)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return err
	}
	patch := func(ctx context.Context, target instance, suspend bool) error {
		values := map[string]any{"mixed-port": port}
		if suspend {
			values["mixed-port"] = 0
			values["allow-lan"] = false
			values["bind-address"] = "127.0.0.1"
			values["tun"] = map[string]bool{"enable": false}
		}
		body, err := json.Marshal(values)
		if err != nil {
			return err
		}
		if err := target.request(ctx, http.MethodPatch, "/configs", bytes.NewReader(body), nil); err != nil {
			return err
		}
		var config struct {
			Mixed int `json:"mixed-port"`
			TUN   struct{ Enable bool }
		}
		if err := target.request(ctx, http.MethodGet, "/configs", nil, &config); err != nil {
			return err
		}
		if (suspend && (config.Mixed != 0 || config.TUN.Enable)) || (!suspend && config.Mixed != port) {
			return i18n.Message("Mihomo did not change the proxy listener state")
		}
		return nil
	}
	next := state
	next.Paused = paused
	var proxyResumed bool
	if !paused {
		if err := m.applyNetworkPreferences(&next); err != nil {
			return err
		}
	}
	err = patch(ctx, state, paused)
	if err == nil {
		if paused && state.ResumeSystemProxy {
			err = m.disableSystemProxy(ctx, "")
		} else if !paused {
			proxyResumed, err = m.resumePausedNetwork(ctx, &next)
			next.ResumeTUN, next.ResumeSystemProxy = false, false
		}
	}
	if err == nil {
		err = m.writeInstance(next)
	}
	if err != nil {
		rollback, cancel := context.WithTimeout(context.WithoutCancel(ctx), 4*time.Second)
		defer cancel()
		rollbackErr := patch(rollback, next, state.Paused)
		if paused && rollbackErr == nil {
			_, rollbackErr = m.resumePausedNetwork(rollback, &state)
		} else if proxyResumed {
			rollbackErr = errors.Join(rollbackErr, m.disableSystemProxy(rollback, ""))
		}
		if rollbackErr == nil && next.PID != state.PID {
			next.Paused = state.Paused
			next.ResumeTUN, next.ResumeSystemProxy = state.ResumeTUN, state.ResumeSystemProxy
			rollbackErr = m.writeInstance(next)
		}
		return errors.Join(err, rollbackErr)
	}
	return nil
}

func (m Manager) resumePausedNetwork(ctx context.Context, state *instance) (bool, error) {
	if state.ResumeTUN {
		if err := m.setTUN(ctx, state, true); err != nil {
			return false, err
		}
	}
	if state.ResumeSystemProxy && !m.SystemProxy(ctx).Enabled {
		return true, m.enableSystemProxy(ctx, state.Proxy, false)
	}
	return false, nil
}

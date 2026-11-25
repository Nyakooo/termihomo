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
	lanPreference         = "app.lan-enabled"
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
	if enabled, ok := settings[lanPreference].(bool); ok {
		state.ResumeLAN = enabled
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
			state.ResumeLAN = overview.LAN
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
			next.ResumeLAN, next.ResumeTUN, next.ResumeSystemProxy = false, false, false
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
			next.ResumeLAN, next.ResumeTUN, next.ResumeSystemProxy = state.ResumeLAN, state.ResumeTUN, state.ResumeSystemProxy
			rollbackErr = m.writeInstance(next)
		}
		return errors.Join(err, rollbackErr)
	}
	return nil
}

// resumePausedNetwork restores each preference independently so a failed TUN
// authorization cannot prevent LAN or system proxy restoration.
func (m Manager) resumePausedNetwork(ctx context.Context, state *instance) (bool, error) {
	var lanErr, tunErr, proxyErr error
	if state.ResumeLAN {
		lanErr = state.setLAN(ctx, true)
	}
	if state.ResumeTUN {
		tunErr = m.setTUN(ctx, state, true)
	}
	proxyResumed := false
	if state.ResumeSystemProxy {
		current := m.SystemProxy(ctx)
		if current.Available {
			// Reapply even an enabled owned proxy: its port may have changed.
			proxyResumed = !current.Enabled
			proxyErr = m.enableSystemProxy(ctx, state.Proxy, false)
		} else {
			// Pre-login services may not have access to desktop proxy settings.
			// The desktop login entry retries without requiring the tray to be enabled.
			m.recordLifecycle("System proxy restoration deferred until desktop login")
		}
	}
	return proxyResumed, errors.Join(lanErr, tunErr, proxyErr)
}

// RestoreDesktopNetwork retries saved preferences after the desktop session
// becomes available. It never starts or resumes a stopped/paused proxy.
func (m Manager) RestoreDesktopNetwork(ctx context.Context) error {
	return m.withLock(func() error {
		status, err := m.Status(ctx)
		if err != nil || status.State != "Running" {
			return err
		}
		state, err := m.runningInstance()
		if err != nil {
			return err
		}
		if err := m.applyNetworkPreferences(&state); err != nil {
			return err
		}
		_, err = m.resumePausedNetwork(ctx, &state)
		if err != nil {
			m.recordLifecycle("Could not restore desktop network preferences: " + err.Error())
		}
		return err
	})
}

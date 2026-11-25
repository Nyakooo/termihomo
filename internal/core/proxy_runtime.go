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
	"github.com/Nyakooo/termihomo/internal/platform"
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

func (m Manager) loadNetworkPreferences(state *instance) error {
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
		if err := m.loadNetworkPreferences(&next); err != nil {
			return err
		}
	}
	err = patch(ctx, state, paused)
	if err == nil {
		if paused && state.ResumeSystemProxy {
			err = m.disableSystemProxy(ctx, "")
		} else if !paused {
			proxyResumed, err = m.restoreNetwork(ctx, &next)
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
			_, rollbackErr = m.restoreNetwork(rollback, &state)
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

// NetworkRestoreError means the core started, but a saved network setting
// could not be restored. Boot launchers must retain the running core.
type NetworkRestoreError struct{ Err error }

func (e *NetworkRestoreError) Error() string { return e.Err.Error() }
func (e *NetworkRestoreError) Unwrap() error { return e.Err }

var errNetworkPending = i18n.Message("Network settings are not ready")

func (m Manager) restoreNetwork(ctx context.Context, state *instance) (bool, error) {
	return m.restoreNetworkWithAuthorization(ctx, state, true)
}

// restoreNetworkWithAuthorization attempts all requested settings independently.
// Boot and login restoration disable interactive TUN authorization.
func (m Manager) restoreNetworkWithAuthorization(ctx context.Context, state *instance, authorize bool) (bool, error) {
	var lanErr, tunErr, proxyErr error
	if state.ResumeLAN {
		current, err := state.overview(ctx)
		lanErr = err
		if err == nil && !current.LAN {
			lanErr = state.setLAN(ctx, true)
		}
		if errors.Is(lanErr, errLANUnavailable) {
			lanErr = errors.Join(errNetworkPending, lanErr)
		}
	}
	if state.ResumeTUN {
		tunErr = m.setTUNWithAuthorization(ctx, state, true, authorize)
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
			proxyErr = errors.Join(errNetworkPending, i18n.Message("System proxy settings unavailable"))
		}
	}
	return proxyResumed, errors.Join(lanErr, tunErr, proxyErr)
}

// RestoreDesktopNetwork waits briefly for normal boot/login readiness races.
// Every attempt releases the instance lock before waiting. It never starts a
// stopped core, resumes a paused core, or requests interactive authorization.
func (m Manager) RestoreDesktopNetwork(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return m.restoreDesktopNetwork(ctx, time.Second)
}

func (m Manager) restoreDesktopNetwork(ctx context.Context, interval time.Duration) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := m.restoreDesktopNetworkOnce(ctx)
		if !errors.Is(err, platform.ErrInstanceBusy) && !errors.Is(err, errNetworkPending) {
			if err != nil {
				m.recordLifecycle("Could not restore desktop network preferences: " + err.Error())
			}
			return err
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			m.recordLifecycle("Desktop network restoration timed out: " + err.Error())
			return errors.Join(ctx.Err(), err)
		case <-timer.C:
		}
	}
}

func (m Manager) restoreDesktopNetworkOnce(ctx context.Context) error {
	return m.withLock(func() error {
		preferences := instance{}
		if err := m.loadNetworkPreferences(&preferences); err != nil {
			return err
		}
		if !preferences.ResumeLAN && !preferences.ResumeTUN && !preferences.ResumeSystemProxy {
			return nil
		}
		status, err := m.Status(ctx)
		if err != nil {
			return err
		}
		switch status.State {
		case "NotInstalled", "Paused":
			return nil
		case "Running":
		default:
			return errNetworkPending
		}
		state, err := m.runningInstance()
		if err != nil {
			return err
		}
		if err := m.loadNetworkPreferences(&state); err != nil {
			return err
		}
		_, err = m.restoreNetworkWithAuthorization(ctx, &state, false)
		return err
	})
}

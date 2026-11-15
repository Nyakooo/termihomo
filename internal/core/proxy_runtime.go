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
		if err := m.setProxyPaused(ctx, state, true); err != nil {
			return err
		}
		m.restoreSystemProxy()
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
	patch := func(ctx context.Context, suspend bool) error {
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
		if err := state.request(ctx, http.MethodPatch, "/configs", bytes.NewReader(body), nil); err != nil {
			return err
		}
		var config struct {
			Mixed int `json:"mixed-port"`
			TUN   struct{ Enable bool }
		}
		if err := state.request(ctx, http.MethodGet, "/configs", nil, &config); err != nil {
			return err
		}
		if (suspend && (config.Mixed != 0 || config.TUN.Enable)) || (!suspend && config.Mixed != port) {
			return i18n.Message("Mihomo did not change the proxy listener state")
		}
		return nil
	}
	err = patch(ctx, paused)
	if err == nil {
		next := state
		next.Paused = paused
		err = m.writeInstance(next)
	}
	if err != nil {
		rollback, cancel := context.WithTimeout(context.WithoutCancel(ctx), 4*time.Second)
		defer cancel()
		return errors.Join(err, patch(rollback, state.Paused))
	}
	return nil
}

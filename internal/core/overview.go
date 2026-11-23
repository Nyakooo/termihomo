package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"runtime"

	"strings"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

type Overview struct {
	Mode              string
	OutboundInterface string
	LAN               bool
	TUN               bool
	TUNUnavailable    bool
	TUNConfigured     bool
	Connections       int
	ResidentBytes     int64
}

func (m Manager) Overview(ctx context.Context) (Overview, error) {
	state, err := m.runningInstance()
	if err != nil {
		return Overview{}, err
	}
	return state.overview(ctx)
}

func (state instance) overview(ctx context.Context) (Overview, error) {
	var config struct {
		Mode              string
		OutboundInterface string `json:"interface-name"`
		LAN               bool   `json:"allow-lan"`
		Tun               struct {
			Enable bool
			Device string
		}
	}
	if err := state.request(ctx, http.MethodGet, "/configs", nil, &config); err != nil {
		return Overview{}, err
	}
	configured := config.Tun.Enable && isManagedTUN(config.Tun.Device)
	var active bool
	if configured {
		iface, err := net.InterfaceByName(config.Tun.Device)
		active = err == nil && iface.Flags&net.FlagUp != 0
	}
	return Overview{Mode: strings.ToLower(config.Mode), OutboundInterface: config.OutboundInterface, TUN: active, LAN: config.LAN, ResidentBytes: ResidentBytes(state.PID),
		TUNConfigured:  configured,
		TUNUnavailable: !active && state.checkTUNPermission() != nil}, nil
}

func (m Manager) SetMode(ctx context.Context, mode string) error {
	switch mode {
	case "rule", "global", "direct":
	default:
		return i18n.Message("invalid routing mode")
	}
	return m.updateRuntime(ctx, map[string]any{"mode": mode}, func(actual Overview) bool {
		return actual.Mode == mode
	})
}

func (m Manager) SetTUN(ctx context.Context, enable bool) error {
	if runtime.GOOS == "android" {
		return i18n.Message("TUN requires an Android VPN app")
	}
	return m.withLock(func() error {
		state, err := m.runningInstance()
		if err != nil {
			return err
		}
		if err := m.setTUN(ctx, &state, enable); err != nil {
			return err
		}
		if err := m.saveNetworkPreference(tunPreference, enable); err != nil {
			return err
		}
		if !enable && state.ResumeTUN {
			state.ResumeTUN = false
			return m.writeInstance(state)
		}
		return nil
	})
}

func (m Manager) setTUN(ctx context.Context, state *instance, enable bool) error {
	if enable && state.Paused {
		return i18n.Message("Start Mihomo to apply changes")
	}
	current, err := state.overview(ctx)
	if err != nil {
		return err
	}
	stack, detectInterface := "system", true
	if enable {
		if current.TUN {
			return nil
		}
		settings, err := m.readToolSettings()
		if err != nil {
			return err
		}
		if saved, ok := settings["app.tun-stack"].(string); ok && saved != "" {
			stack = saved
		}
		detectInterface = current.OutboundInterface == ""
		if _, err := net.InterfaceByName(tunDevice); err == nil {
			return i18n.Message("The termihomo network interface is already in use")
		}
		if err := state.checkTUNPermission(); errors.Is(err, errTUNCapability) {
			authorized, err := m.authorizeTUN(ctx, *state, current)
			if authorized.PID != 0 {
				authorized.ResumeTUN = state.ResumeTUN
				authorized.ResumeSystemProxy = state.ResumeSystemProxy
				*state = authorized
			}
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	patch := map[string]any{"tun": map[string]any{
		"enable": enable, "device": tunDevice, "stack": stack,
		"auto-route": true, "auto-detect-interface": detectInterface,
		"dns-mode": "disabled",
	}}
	return state.updateRuntime(ctx, patch, func(actual Overview) bool {
		return actual.TUN == enable && actual.TUNConfigured == enable
	})
}

func (m Manager) updateRuntime(ctx context.Context, patch map[string]any, applied func(Overview) bool) error {
	return m.withLock(func() error {
		state, err := m.runningInstance()
		if err != nil {
			return err
		}
		return state.updateRuntime(ctx, patch, applied)
	})
}

func (state instance) updateRuntime(ctx context.Context, patch map[string]any, applied func(Overview) bool) error {
	body, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	if err := state.request(ctx, http.MethodPatch, "/configs", bytes.NewReader(body), nil); err != nil {
		return err
	}
	actual, err := state.overview(ctx)
	if err != nil {
		return err
	}
	if !applied(actual) {
		if _, tun := patch["tun"]; tun {
			return i18n.Message("TUN did not change state; check Mihomo logs and network privileges")
		}
		if _, lan := patch["allow-lan"]; lan {
			return i18n.Message("The LAN proxy listener did not change state")
		}
		return i18n.Message("Mihomo did not apply the routing mode")
	}
	return nil
}

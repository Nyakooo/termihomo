package tui

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/i18n"
	"github.com/Nyakooo/termihomo/internal/platform"
	"github.com/Nyakooo/termihomo/internal/tray"
)

type trayNodesMsg struct{}
type trayExitMsg struct{}

func watchTrayInterface(ctx context.Context, paths platform.Paths, exitToken, nodesToken string, send func(tea.Msg)) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if token := tray.InterfaceToken(paths, tray.ExitInterface); token != "" && token != exitToken {
				send(trayExitMsg{})
				return
			}
			if token := tray.InterfaceToken(paths, tray.OpenNodes); token != "" && token != nodesToken {
				nodesToken = token
				send(trayNodesMsg{})
			}
		}
	}
}

func (m *model) updateTray() {
	if m.snapshot.toolSettings[core.SystemTrayKey] != "true" {
		return
	}
	if !m.trayAvailable {
		m.trayAvailable = tray.Available()
	}
	if m.trayAvailable &&
		tray.InterfaceToken(m.manager.Paths, tray.ExitInterface) == m.trayExitToken {
		_ = tray.Start(m.manager.Paths)
	}
}

func TrayDesktopState(paths platform.Paths) tray.DesktopState {
	manager := (core.Manager{Paths: paths}).WithRuntimeCache()
	const delayInterval = time.Minute
	var probeNode string
	var probePID int
	var lastProbe time.Time
	var delays map[string]core.DelayResult
	var pending <-chan core.DelayResult
	var cancelProbe context.CancelFunc
	activeOverview := func(ctx context.Context) (core.Overview, error) {
		status, err := manager.Status(ctx)
		if err != nil {
			return core.Overview{}, err
		}
		if status.State != "Running" {
			return core.Overview{}, i18n.Message("Start Mihomo to apply changes")
		}
		return manager.Overview(ctx)
	}
	return tray.DesktopState{
		Enabled: manager.TrayEnabled,
		Read: func(ctx context.Context, test bool, refresh func()) tray.Summary {
			readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			var runtime core.RuntimeSnapshot
			var proxy core.SystemProxy
			var reads sync.WaitGroup
			reads.Go(func() { runtime = manager.ReadTrayRuntime(readCtx) })
			reads.Go(func() { proxy = manager.SystemProxy(readCtx) })
			reads.Wait()
			cancel()
			m := model{
				lang: i18n.Saved(paths.Config),
				snapshot: snapshot{
					status: runtime.Status, overview: runtime.Overview,
					proxies: runtime.Proxies, rules: runtime.Rules, proxyErr: runtime.ProxyErr,
					systemProxy: proxy,
				},
			}
			node := ""
			if runtime.Status.State == "Running" {
				switch selected := serviceNode(m.snapshot); selected {
				case "REJECT", "REJECT-DROP", "PASS":
				default:
					node = selected
				}
			}
			if node != probeNode || runtime.Status.PID != probePID {
				if cancelProbe != nil {
					cancelProbe()
				}
				probeNode, probePID = node, runtime.Status.PID
				lastProbe, pending, delays = time.Time{}, nil, nil
				cancelProbe = nil
			}
			select {
			case result := <-pending:
				delays = map[string]core.DelayResult{node: result}
				pending = nil
				cancelProbe()
				cancelProbe = nil
			default:
			}
			if node != "" && pending == nil && (test || (node != "DIRECT" && time.Since(lastProbe) >= delayInterval)) {
				probeCtx, cancel := context.WithCancel(ctx)
				result := make(chan core.DelayResult, 1)
				pending, cancelProbe, lastProbe = result, cancel, time.Now()
				go func() {
					delay, err := manager.Delay(probeCtx, node, "")
					result <- core.NewDelayResult(delay, err)
					refresh()
				}()
			}
			m.delays = delays
			summary := m.traySummary()
			summary.CanTest = node != "" && pending == nil
			if pending != nil {
				summary.Lines[5] = m.lang.Text("Latency") + " · " + m.lang.Text("Testing…")
			}
			return summary
		},
		CycleMode: func(ctx context.Context) error {
			overview, err := activeOverview(ctx)
			if err != nil {
				return err
			}
			next := "rule"
			switch overview.Mode {
			case "rule":
				next = "global"
			case "global":
				next = "direct"
			}
			return manager.SetMode(ctx, next)
		},
		ToggleTUN: func(ctx context.Context) error {
			overview, err := activeOverview(ctx)
			if err != nil {
				return err
			}
			return manager.SetTUN(ctx, !overview.TUN && !overview.TUNConfigured)
		},
		ToggleSystemProxy: func(ctx context.Context) error {
			proxy := manager.SystemProxy(ctx)
			if !proxy.Available {
				return i18n.Message("System proxy settings unavailable")
			}
			return manager.SetSystemProxy(ctx, !proxy.Enabled, "")
		},
		ToggleLAN: func(ctx context.Context) error {
			overview, err := activeOverview(ctx)
			if err != nil {
				return err
			}
			return manager.SetLAN(ctx, !overview.LAN)
		},
		Pause: func(ctx context.Context) error {
			status, err := manager.Status(ctx)
			if err != nil {
				return err
			}
			if status.State == "Running" || status.State == "Degraded" {
				return manager.PauseProxy(ctx)
			}
			return nil
		},
	}
}

func (m *model) traySummary() tray.Summary {
	node, delay := "—", "—"
	mode := orDash(m.lang.Text(m.snapshot.overview.Mode))
	lan := "off"
	if m.snapshot.overview.LAN {
		lan = "on"
	}
	if m.snapshot.status.State == "Running" {
		selected := serviceNode(m.snapshot)
		node = orDash(clip(safe(selected), 120))
		if i := slices.IndexFunc(m.snapshot.proxies, func(p core.Proxy) bool { return p.Name == selected }); i >= 0 {
			if ms := m.proxyDelay(m.snapshot.proxies[i]); ms > 0 {
				delay = fmt.Sprintf("%d ms", ms)
			}
		}
	}
	proxy := m.snapshot.systemProxy
	proxyLabel := "Unavailable"
	if proxy.Available {
		proxyLabel = "off"
		if proxy.Enabled {
			proxyLabel = "on"
			if !proxy.Owned {
				proxyLabel = "on (external)"
			}
		}
	}
	tun := "off"
	switch m.tunLabel() {
	case "TUN on":
		tun = "on"
	case "TUN paused":
		tun = "paused"
	case "TUN inactive":
		tun = "inactive"
	case "TUN unavailable":
		tun = "Unavailable"
	}
	return tray.Summary{
		Active:               m.kernelActive(),
		Running:              m.snapshot.status.State == "Running",
		CanToggleSystemProxy: proxy.Available && ((proxy.Enabled && proxy.Owned) || (!proxy.Enabled && m.snapshot.status.State == "Running")),
		Lines: [6]string{
			m.lang.Text("Current node") + " · " + node,
			m.lang.Text("Routing mode") + " · " + mode,
			m.lang.Text("System proxy") + " · " + m.lang.Text(proxyLabel),
			"TUN · " + m.lang.Text(tun),
			m.lang.Text("LAN proxy") + " · " + m.lang.Text(lan),
			m.lang.Text("Latency") + " · " + delay,
		},
	}
}

package tui

import (
	"fmt"
	"slices"

	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/tray"
)

func (m *model) updateTray() {
	if m.tray == nil {
		return
	}
	enabled := m.snapshot.toolSettings[core.SystemTrayKey] == "true"
	if !enabled {
		m.tray.Set(false, tray.Summary{})
		return
	}
	m.tray.Set(true, m.traySummary())
}

func (m *model) traySummary() tray.Summary {
	node, delay := "—", "—"
	mode := orDash(m.lang.Text(m.snapshot.overview.Mode))
	if m.snapshot.status.State == "Running" {
		selected := serviceNode(m.snapshot)
		node = orDash(clip(safe(selected), 120))
		if i := slices.IndexFunc(m.snapshot.proxies, func(p core.Proxy) bool { return p.Name == selected }); i >= 0 {
			if ms := m.proxyDelay(m.snapshot.proxies[i]); ms > 0 {
				delay = fmt.Sprintf("%d ms", ms)
			}
		}
	}
	return tray.Summary{
		Active: m.kernelActive(),
		Lines: [4]string{
			m.lang.Text("Current node") + " · " + node,
			m.lang.Text("Routing mode") + " · " + mode,
			m.lang.Text(m.tunLabel()),
			m.lang.Text("Latency") + " · " + delay,
		},
	}
}

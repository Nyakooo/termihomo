package tui

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/Nyakooo/termihomo/internal/core"
)

const (
	networkSystemProxy = iota
	networkExclusions
	networkTUN
	networkStack
	networkInterface
	networkRowCount
)

func (m *model) networkValue(index int) string {
	switch index {
	case networkStack:
		if stack := m.snapshot.toolSettings[core.NetworkToolFields[0].Key]; stack != "" {
			return stack
		}
		return "system"
	case networkInterface:
		if name, ok := m.snapshot.toolSettings[core.NetworkToolFields[1].Key]; ok {
			return name
		}
		return m.snapshot.overview.OutboundInterface
	}
	return ""
}

func (m *model) networkPageView(height int) []string {
	proxy := m.snapshot.systemProxy
	if m.showProxyExclusions {
		rows := make([]string, len(proxy.Exclusions))
		for i, host := range proxy.Exclusions {
			rows[i] = safe(host)
		}
		empty := m.lang.Text("No proxy exclusions")
		if !proxy.Available {
			empty = m.lang.Text("System proxy settings unavailable")
		} else if proxy.ExclusionsErr != nil {
			empty = m.lang.Error(proxy.ExclusionsErr)
		}
		return m.listPanel(m.lang.Text("Proxy exclusions"), nil, rows, m.cursor, height, 0, m.width, empty, hitRow)
	}
	proxyStatus, exclusions := "Unavailable", m.lang.Text("Unavailable")
	if proxy.Available {
		proxyStatus = "off"
		if proxy.Enabled {
			proxyStatus = "on"
		}
		if proxy.ExclusionsErr == nil {
			exclusions = fmt.Sprintf(m.lang.Text("%d entries"), len(proxy.Exclusions))
		}
	}
	outbound := m.networkValue(networkInterface)
	if outbound == "" {
		outbound = m.lang.Text("Automatic")
	}
	labels := []string{"System proxy", "Proxy exclusions", "Virtual network interface", "Stack mode", "Outbound interface"}
	values := []string{m.lang.Text(proxyStatus), exclusions, m.lang.Text(m.tunLabel()), m.networkValue(networkStack), safe(outbound)}
	rows := make([]string, 0, networkRowCount)
	for i, label := range labels {
		rows = append(rows, fit(m.lang.Text(label), min(28, m.width/2))+values[i])
	}
	return m.listPanel(m.lang.Text("Network"), []string{m.style.dim(m.lang.Text("Enter to change network settings")),
		m.style.dim(m.lang.Text("TUN requires network administration privileges"))}, rows, m.cursor, height, 0, m.width, "", hitRow)
}

func (m *model) networkPageKey(key string) tea.Cmd {
	if m.showProxyExclusions {
		return m.proxyExclusionsKey(key)
	}
	if key == "ctrl+r" {
		m.confirmLabel = m.lang.Text("Apply settings and restart")
		m.confirm = func() error { return m.manager.Restart(m.ctx) }
		return nil
	}
	if key != "enter" {
		return nil
	}
	switch m.cursor {
	case networkSystemProxy:
		return m.toggleSystemProxy()
	case networkExclusions:
		if !m.snapshot.systemProxy.Available {
			m.feedback = m.lang.Text("System proxy settings unavailable")
			return nil
		}
		m.showProxyExclusions, m.cursor = true, 0
	case networkTUN:
		return m.toggleTUN()
	case networkStack, networkInterface:
		field := core.NetworkToolFields[m.cursor-networkStack]
		m.toolField = field
		m.openForm(field.Label, m.networkValue(m.cursor))
	}
	return nil
}

func (m *model) proxyExclusionsKey(key string) tea.Cmd {
	if key == "esc" {
		m.showProxyExclusions, m.cursor = false, networkExclusions
		return nil
	}
	proxy := m.snapshot.systemProxy
	if !proxy.Available || proxy.ExclusionsErr != nil {
		return nil
	}
	hosts := proxy.Exclusions
	if key == "a" {
		m.exclusionIndex = -1
		m.openForm("Add proxy exclusion", "")
		return nil
	}
	if len(hosts) == 0 {
		return nil
	}
	index := min(m.cursor, len(hosts)-1)
	switch key {
	case "enter":
		m.exclusionIndex, m.exclusionOriginal = index, hosts[index]
		m.openForm("Edit proxy exclusion", hosts[index])
	case "d":
		next := slices.Delete(slices.Clone(hosts), index, index+1)
		m.confirmLabel = m.lang.Text("Delete") + " · " + safe(hosts[index])
		m.confirm = func() error { return m.manager.SetProxyExclusions(m.ctx, hosts, next) }
	}
	return nil
}

func (m *model) saveProxyExclusion() tea.Cmd {
	input := strings.TrimSpace(string(m.input))
	if input == "" || strings.ContainsFunc(input, unicode.IsSpace) || strings.Contains(input, ",") {
		m.formError = m.lang.Text("Enter one domain, IP address, or network range")
		return nil
	}
	index := m.exclusionIndex
	hosts := m.snapshot.systemProxy.Exclusions
	if index >= 0 && (index >= len(hosts) || hosts[index] != m.exclusionOriginal) {
		m.formError = m.lang.Text("System proxy exclusions changed. Review the list and try again.")
		return nil
	}
	for i, host := range hosts {
		if i != index && strings.EqualFold(host, input) {
			m.formError = m.lang.Text("This proxy exclusion already exists")
			return nil
		}
	}
	next := slices.Clone(hosts)
	if index < 0 {
		next = append(next, input)
		m.cursor = len(next) - 1
	} else {
		next[index] = input
	}
	m.retryForm, m.retryInput = m.form, []rune(input)
	m.closeForm()
	return m.act(m.lang.Text("Save setting"), func() error {
		return m.manager.SetProxyExclusions(m.ctx, hosts, next)
	})
}

func (m *model) toggleSystemProxy() tea.Cmd {
	proxy := m.snapshot.systemProxy
	if !proxy.Available {
		m.feedback = m.lang.Text("System proxy settings unavailable")
		return nil
	}
	if proxy.Enabled && !proxy.Owned {
		m.confirmLabel = m.lang.Text("Turn off the system proxy configured outside Termihomo")
		m.confirm = func() error { return m.manager.SetSystemProxy(m.ctx, false, proxy.Revision) }
		return nil
	}
	if !proxy.Enabled && m.snapshot.status.State != "Running" {
		m.feedback = m.lang.Text("Start Mihomo to apply changes")
		return nil
	}
	return m.act(m.lang.Text("System proxy"), func() error { return m.manager.SetSystemProxy(m.ctx, !proxy.Enabled, "") })
}

func (m *model) toggleTUN() tea.Cmd {
	if m.snapshot.status.State != "Running" {
		m.feedback = m.lang.Text("Start Mihomo to apply changes")
		return nil
	}
	enable := !m.snapshot.overview.TUN && !m.snapshot.overview.TUNConfigured
	label := "Disable TUN"
	if enable {
		label = "Enable TUN automatic routing"
	}
	return m.act(m.lang.Text(label), func() error { return m.manager.SetTUN(m.ctx, enable) })
}

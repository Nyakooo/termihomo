package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Nyakooo/termihomo/internal/core"
)

type dnsQueryDone struct {
	name     string
	kind     string
	duration time.Duration
	result   core.DNSResult
	err      error
}

func (m *model) toolPage(height int) []string {
	fields := core.GeneralToolFields
	entries := make([]string, 0, len(fields))
	for _, field := range fields {
		value := safe(m.snapshot.toolSettings[field.Key])
		if value == "Unavailable" {
			value = m.style.dim(m.lang.Text(value))
		}
		if value == "" {
			value = m.style.dim(m.lang.Text("Use profile defaults"))
			if field.Key == "mixed-port" {
				value = fmt.Sprintf(m.lang.Text("%d (default)"), core.DefaultProxyPort)
			}
			if defaultValue := map[string]string{"app.auto-run": "false", "app.test-url": "https://www.gstatic.com/generate_204", "app.test-timeout": "3000", "app.close-connections": "false"}[field.Key]; defaultValue != "" {
				value = defaultValue
			}
		}
		labelWidth := min(34, m.width/2-4)
		entries = append(entries, fit(clip(m.lang.Text(field.Label), labelWidth-1), labelWidth)+value)
	}
	header := []string{
		m.style.dim(m.lang.Text("Enter edit · d reset")),
		m.style.dim(m.lang.Text("Core settings apply after restart")),
	}
	if m.snapshot.toolSettingsErr != nil {
		header = append(header, m.style.err(m.lang.Error(m.snapshot.toolSettingsErr)))
	}
	if m.page == generalPage && m.snapshot.status.Proxy != "" {
		header = append(header, m.lang.Text("Proxy")+" "+m.snapshot.status.Proxy+" · "+m.lang.Text("LAN proxy")+" "+m.boolMark(m.snapshot.overview.LAN))
	}
	return m.listPanel(m.lang.Text("General"), header, entries, m.cursor, height, 0, m.width, "", hitRow)
}

func (m *model) editToolField() {
	fields := core.GeneralToolFields
	field := fields[min(m.cursor, len(fields)-1)]
	m.toolField = field
	m.openForm(field.Label, m.snapshot.toolSettings[field.Key])
}

func (m *model) toolPageKey(key string) tea.Cmd {
	switch key {
	case "enter":
		m.editToolField()
	case "d":
		field := core.GeneralToolFields[min(m.cursor, len(core.GeneralToolFields)-1)]
		m.confirmLabel = m.lang.Text("Reset setting") + " · " + m.lang.Text(field.Label)
		m.confirm = func() error { return m.manager.SetToolSetting(m.ctx, field.Key, "") }
	case "ctrl+r":
		m.confirmLabel = m.lang.Text("Apply settings and restart")
		m.confirm = func() error { return m.manager.Restart(m.ctx) }
	case "w":
		if m.page == generalPage {
			if m.snapshot.status.State != "Running" {
				m.feedback = m.lang.Text("Start Mihomo to apply changes")
				return nil
			}
			if m.snapshot.overview.LAN {
				return m.act(m.lang.Text("LAN proxy off"), func() error { return m.manager.SetLAN(m.ctx, false) })
			}
			m.confirmLabel = m.lang.Text("Allow devices on local subnets to use this proxy")
			m.confirm = func() error { return m.manager.SetLAN(m.ctx, true) }
		}
	}
	return nil
}

func (m *model) queryDNS(input string) tea.Cmd {
	values := strings.Fields(input)
	if len(values) < 1 || len(values) > 2 {
		m.openForm("DNS query", input)
		m.formError = m.lang.Text("Enter a domain and optional record type")
		return nil
	}
	kind := "A"
	if len(values) == 2 {
		kind = strings.ToUpper(values[1])
	}
	return func() tea.Msg {
		started := time.Now()
		result, err := m.manager.QueryDNS(m.testContext(), values[0], kind)
		return dnsQueryDone{name: values[0], kind: kind, duration: time.Since(started), result: result, err: err}
	}
}

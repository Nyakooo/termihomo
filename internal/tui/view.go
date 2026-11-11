package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/charmbracelet/x/ansi"
)

func (m *model) View() tea.View {
	title := "Termihomo  /  " + m.lang.Text(pages[m.page])
	header := []string{title}
	if m.width >= 75 {
		tabs := make([]string, len(pages))
		for index, label := range pages {
			tabs[index] = fmt.Sprintf("%d %s", index+1, m.lang.Text(label))
			if page(index) == m.page {
				tabs[index] = "[" + tabs[index] + "]"
			}
		}
		header = append(header, strings.Join(tabs, "  "))
	}
	header = append(header, strings.Repeat("─", max(0, m.width)))
	footer := fmt.Sprintf(m.lang.Text("Tab pages  ·  %s help  ·  %s quit"), m.settings.key("help"), m.settings.key("quit"))
	if m.feedback != "" {
		footer = m.feedback
	}
	switch {
	case m.confirm != nil:
		footer = m.confirmLabel + "?  [y/N]"
	case m.form == "Subscription URL":
		footer = fmt.Sprintf(m.lang.Text("Subscription URL (hidden, %d chars)  ·  Enter save / Esc cancel"), len(m.input))
	case m.form != "":
		footer = fmt.Sprintf(m.lang.Text("%s › %s  ·  Enter save / Esc cancel"), m.lang.Text(m.form), string(m.input))
	}

	available := max(0, m.height-len(header)-1)
	body := m.body(available)
	if len(body) > available {
		body = body[:available]
	}
	lines := append(header, body...)
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	lines = append(lines, footer)
	if len(lines) > m.height {
		lines = lines[:max(0, m.height)]
	}
	for index := range lines {
		lines[index] = ansi.Truncate(safe(lines[index]), m.width, "")
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}

func (m *model) body(height int) []string {
	if m.help {
		return []string{
			m.lang.Text("Tab / ← →   Switch page"), m.lang.Text("1–7         Open page"), m.lang.Text("↑ ↓ / j k   Move selection"),
			m.settings.key("refresh") + m.lang.Text("           Refresh"), m.settings.key("search") + m.lang.Text("           Search proxies or groups"),
			m.settings.key("sort") + " / " + m.settings.key("test") + m.lang.Text("       Sort by delay / Test selected proxy"),
			m.lang.Text("Enter       Open Selector or choose option"), m.lang.Text("Esc         Close selection or clear filter"),
			m.lang.Text("Profile     a subscription · i import · u update"), m.lang.Text("Core        i install · a start · x stop · R restart"),
			m.lang.Text("Logs        ") + m.settings.key("log-level") + m.lang.Text(" cycle minimum log level"), m.settings.key("quit") + m.lang.Text("           Quit and keep Mihomo running"),
		}
	}
	s := m.snapshot.status
	switch m.page {
	case dashboard:
		state := s.State
		if state == "" {
			state = "Loading"
		}
		profile := m.snapshot.profile.Source
		if profile == "" {
			profile = "Built-in DIRECT"
		}
		rows := []string{"Mihomo       " + m.lang.Text(state) + "  " + s.Installed, m.lang.Text("Profile      ") + m.lang.Text(profile)}
		if s.State == "Running" {
			rows = append(rows,
				m.lang.Text("Uptime       ")+time.Since(s.Started).Truncate(time.Second).String(),
				m.lang.Text("Proxy        ")+s.Proxy,
				m.lang.Text("Mode         ")+m.lang.Text(m.snapshot.overview.Mode),
				fmt.Sprintf(m.lang.Text("Connections %d"), m.snapshot.overview.Connections),
			)
			for _, p := range m.snapshot.proxies {
				if p.Type == "Selector" && p.Name != "GLOBAL" {
					rows = append(rows, m.lang.Text("Selected     ")+p.Name+" → "+p.Now)
					break
				}
			}
		}
		if m.snapshot.err != nil {
			rows = append(rows, m.lang.Text("Error        ")+m.lang.Error(m.snapshot.err))
		}
		return append(rows, "", m.trafficLine())
	case proxies, groups:
		return m.proxyRows(height)
	case profiles:
		p := m.snapshot.profile
		rows := []string{m.lang.Text("a  Add subscription    i  Import local YAML    u  Update")}
		if p.Source == "" {
			return append(rows, "", m.lang.Text("No profile configured. The default routes directly."))
		}
		return append(rows, "", m.lang.Text("Source    ")+m.lang.Text(p.Source), m.lang.Text("Updated   ")+p.Updated.Format(time.DateTime))
	case traffic:
		return []string{
			m.trafficLine(), "",
			m.lang.Text("Upload    ") + m.chart(true),
			m.lang.Text("Download  ") + m.chart(false),
		}
	case logs:
		return m.logRows(height)
	case corePage:
		rows := []string{m.lang.Text("i  Install    a  Start    x  Stop    R  Restart"), "", m.lang.Text("State       ") + m.lang.Text(s.State), m.lang.Text("Version     ") + s.Installed}
		if s.PID != 0 {
			rows = append(rows, fmt.Sprintf("PID         %d", s.PID), m.lang.Text("Controller  ")+s.Controller, m.lang.Text("Proxy       ")+s.Proxy)
		}
		if m.snapshot.err != nil {
			rows = append(rows, m.lang.Text("Error       ")+m.lang.Error(m.snapshot.err))
		}
		return rows
	}
	return nil
}

func (m *model) visibleProxies() []core.Proxy {
	var filtered []core.Proxy
	for _, item := range m.snapshot.proxies {
		isGroup := len(item.All) > 0
		if (m.page == groups) != isGroup || !strings.Contains(strings.ToLower(item.Name), m.filter) {
			continue
		}
		filtered = append(filtered, item)
	}
	if m.sortByDelay && m.page == proxies {
		sort.SliceStable(filtered, func(i, j int) bool {
			left, right := lastDelay(filtered[i]), lastDelay(filtered[j])
			return left > 0 && (right <= 0 || left < right)
		})
	}
	return filtered
}

func lastDelay(p core.Proxy) int {
	if len(p.History) == 0 {
		return 0
	}
	return p.History[len(p.History)-1].Delay
}

func (m *model) proxyRows(height int) []string {
	items := m.visibleProxies()
	if m.groupName != "" {
		for _, item := range items {
			if item.Name != m.groupName {
				continue
			}
			hint := m.lang.Text(" · Enter select / Esc back")
			if item.Type != "Selector" {
				hint = m.lang.Text(" · Automatic selection / Esc back")
			}
			rows := []string{item.Name + hint}
			options := make([]string, len(item.All))
			for i, name := range item.All {
				options[i] = name
				if name == item.Now {
					options[i] += "  ✓"
				}
			}
			return append(rows, viewport(options, m.choice, height-1)...)
		}
	}
	rows := []string{fmt.Sprintf(m.lang.Text("%s search  ·  %s sort by delay  ·  %s test"), m.settings.key("search"), m.settings.key("sort"), m.settings.key("test"))}
	if m.page == groups {
		rows[0] = m.settings.key("search") + m.lang.Text(" search  ·  Enter open group")
	}
	if m.filter != "" {
		rows = append(rows, m.lang.Text("Filter  ")+m.filter)
	}
	if len(items) == 0 {
		if m.snapshot.status.State == "Running" {
			return append(rows, m.lang.Text("No matching entries"))
		}
		return append(rows, m.lang.Text("Start Mihomo to load entries"))
	}
	var entries []string
	for _, item := range items {
		line := fmt.Sprintf("%s  ·  %s", item.Name, item.Type)
		if item.Now != "" {
			line += "  →  " + item.Now
		} else if delay := lastDelay(item); delay > 0 {
			line += fmt.Sprintf("  ·  %d ms", delay)
		} else if !item.Alive {
			line += m.lang.Text("  ·  unavailable")
		}
		entries = append(entries, line)
	}
	return append(rows, viewport(entries, m.cursor, height-len(rows))...)
}

func viewport(lines []string, cursor, height int) []string {
	if height < 1 || len(lines) == 0 {
		return nil
	}
	cursor = min(cursor, len(lines)-1)
	start := max(0, cursor-height+1)
	result := make([]string, 0, min(height, len(lines)))
	for index := start; index < min(len(lines), start+height); index++ {
		marker := "  "
		if index == cursor {
			marker = "› "
		}
		result = append(result, marker+lines[index])
	}
	return result
}

func (m *model) trafficLine() string {
	if len(m.rates) == 0 {
		return m.lang.Text("Traffic      Waiting for samples")
	}
	current := m.rates[len(m.rates)-1]
	line := fmt.Sprintf(m.lang.Text("↑ %s/s  ↓ %s/s  ·  total ↑ %s ↓ %s"),
		bytesLabel(current.Up), bytesLabel(current.Down), bytesLabel(current.UpTotal), bytesLabel(current.DownTotal))
	if !m.trafficLive {
		line = m.lang.Text("Stale · ") + line
	}
	return line
}

func bytesLabel(bytes int64) string {
	value := float64(bytes)
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	i := 0
	for value >= 1024 && i < len(units)-1 {
		value /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", value, units[i])
}

func (m *model) chart(up bool) string {
	blocks := []rune("▁▂▃▄▅▆▇█")
	samples := m.rates[max(0, len(m.rates)-max(0, m.width-11)):]
	rate := func(sample core.Traffic) int64 {
		if up {
			return sample.Up
		}
		return sample.Down
	}
	var highest int64 = 1
	for _, sample := range samples {
		highest = max(highest, rate(sample))
	}
	var chart strings.Builder
	for _, sample := range samples {
		level := int(float64(max(0, rate(sample))) / float64(highest) * float64(len(blocks)-1))
		chart.WriteRune(blocks[level])
	}
	return chart.String()
}

func (m *model) logRows(height int) []string {
	var entries []string
	for _, entry := range m.logEntries {
		for i, level := range levels {
			if i >= m.logLevel && strings.EqualFold(entry.Type, level) {
				entries = append(entries, "["+m.lang.Text(entry.Type)+"] "+entry.Payload)
			}
		}
	}
	header := m.lang.Text("Mihomo logs  ·  ") + m.lang.Text(levels[m.logLevel]) + "+  ·  " + m.settings.key("log-level") + m.lang.Text(" filter")
	if !m.logsLive {
		header += m.lang.Text("  ·  Waiting for live logs")
	}
	if len(entries) > 0 && !m.logsLive {
		header += m.lang.Text(" (showing stale data)")
	}
	return append([]string{header}, entries[max(0, len(entries)-max(0, height-1)):]...)
}

func safe(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
}

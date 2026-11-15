package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Nyakooo/termihomo/internal/core"
)

type dnsRow struct {
	at                                                   time.Time
	name, kind, initiator, result, source, status, rcode string
	cache, cacheKnown                                    bool
	elapsed                                              time.Duration
	timed                                                bool
}

type dnsFeed struct {
	rows    []dnsRow
	pending map[core.DNSEvent]time.Time
	filters map[string]bool
	detail  dnsRow
}

func (m *model) recordDNSEvent(entry core.LogEntry) {
	if entry.DNS == nil {
		return
	}
	event := *entry.DNS
	key := core.DNSEvent{Domain: event.Domain, Type: event.Type, Source: event.Source}
	if event.Started {
		if len(m.dnsFeed.pending) >= 256 {
			for item, started := range m.dnsFeed.pending {
				if entry.Time.Sub(started) > 15*time.Second {
					delete(m.dnsFeed.pending, item)
				}
			}
		}
		if len(m.dnsFeed.pending) < 256 {
			if m.dnsFeed.pending == nil {
				m.dnsFeed.pending = make(map[core.DNSEvent]time.Time)
			}
			m.dnsFeed.pending[key] = entry.Time
		}
		return
	}
	start, found := m.dnsFeed.pending[key]
	delete(m.dnsFeed.pending, key)
	elapsed := entry.Time.Sub(start)
	m.addDNSRow(dnsRow{at: entry.Time, name: event.Domain, kind: event.Type,
		result: event.Result, source: event.Source, cache: event.Cached, cacheKnown: true,
		elapsed: elapsed, timed: found && elapsed >= 0 && elapsed <= 15*time.Second})
}

func (m *model) recordManualDNS(msg dnsQueryDone) {
	row := dnsRow{at: time.Now(), name: msg.name, kind: msg.kind, initiator: "Manual",
		elapsed: msg.duration, timed: true}
	if msg.err != nil {
		row.status = "Error"
		row.result = m.lang.Error(msg.err)
	} else {
		codes := [...]string{"NOERROR", "FORMERR", "SERVFAIL", "NXDOMAIN", "NOTIMP", "REFUSED"}
		if code := msg.result.Status; code >= 0 && code < len(codes) {
			row.rcode = codes[code]
		} else {
			row.rcode = fmt.Sprintf("RCODE %d", msg.result.Status)
		}
		if msg.result.Status != 0 {
			row.status = row.rcode
		}
	}
	var answers []string
	for _, record := range msg.result.Answer {
		answers = append(answers, record.Data)
	}
	if len(answers) > 0 {
		row.result = strings.Join(answers, ", ")
	}
	m.addDNSRow(row)
}

func (m *model) addDNSRow(row dnsRow) {
	m.dnsFeed.rows = append(m.dnsFeed.rows, row)
	const maxDNSRows = 1000
	m.dnsFeed.rows = m.dnsFeed.rows[max(0, len(m.dnsFeed.rows)-maxDNSRows):]
	if m.page == dnsPage && m.cursor > 0 && m.matchesDNS(row) {
		m.cursor = min(m.cursor+1, len(m.dnsFeed.rows)-1)
	}
}

func (m *model) matchesDNS(row dnsRow) bool {
	if m.filter != "" {
		query := strings.ToLower(strings.Join([]string{row.name, row.kind, row.result, row.source, row.status}, " "))
		if !strings.Contains(query, m.filter) {
			return false
		}
	}
	if len(m.dnsFeed.filters) == 0 {
		return true
	}
	return m.dnsFeed.filters[row.kind] || (row.cache && m.dnsFeed.filters["Cached"]) ||
		(row.status != "" && m.dnsFeed.filters[row.status]) || (row.initiator != "" && m.dnsFeed.filters[row.initiator])
}

func (m *model) visibleDNSRows() []dnsRow {
	var rows []dnsRow
	for i := len(m.dnsFeed.rows) - 1; i >= 0; i-- {
		row := m.dnsFeed.rows[i]
		if m.matchesDNS(row) {
			rows = append(rows, row)
		}
	}
	return rows
}

func (m *model) dnsFilters() []string {
	values := make(map[string]bool)
	for _, row := range m.dnsFeed.rows {
		values[row.kind], values[row.status], values[row.initiator] = true, true, true
		if row.cache {
			values["Cached"] = true
		}
	}
	for key := range m.dnsFeed.filters {
		values[key] = true
	}
	delete(values, "")
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func (m *model) toggleDNSFilter(value string) {
	if m.dnsFeed.filters == nil {
		m.dnsFeed.filters = make(map[string]bool)
	}
	if m.dnsFeed.filters[value] {
		delete(m.dnsFeed.filters, value)
	} else {
		m.dnsFeed.filters[value] = true
	}
	m.cursor = 0
}

func (m *model) dnsPageKey(key string) tea.Cmd {
	switch key {
	case "a":
		m.openForm("DNS query", "")
	case "c", "x":
		label, fake := "Flush DNS cache", key == "x"
		if fake {
			label = "Flush Fake-IP cache"
		}
		m.confirmLabel = m.lang.Text(label)
		m.confirm = func() error { return m.manager.FlushDNS(m.ctx, fake) }
	case "d":
		m.dnsFeed.rows, m.dnsFeed.pending, m.dnsFeed.filters = nil, nil, nil
		m.cursor = 0
	case "enter":
		if rows := m.visibleDNSRows(); len(rows) != 0 {
			m.dnsFeed.detail = rows[min(m.cursor, len(rows)-1)]
			m.detail = true
		}
	}
	return nil
}

func (m *model) dnsDetails(w int) []string {
	row := m.dnsFeed.detail
	stamp := ""
	if !row.at.IsZero() {
		stamp = row.at.Local().Format("2006-01-02 15:04:05")
	}
	cache := ""
	if row.cacheKnown || row.cache {
		cache = m.lang.Text("no")
		if row.cache {
			cache = m.lang.Text("yes")
		}
	}
	fields := [][2]string{
		{"Timestamp", stamp},
		{"Domain", row.name},
		{"Record type", row.kind},
		{"Initiator", m.lang.Text(row.initiator)},
		{"Source", row.source},
		{"Cache", cache},
		{"Response code", row.rcode},
		{"Latency", m.dnsElapsed(row)},
		{"Result", row.result},
	}
	labelWidth := 0
	for _, field := range fields {
		labelWidth = max(labelWidth, width(m.lang.Text(field[0]))+2)
	}
	lines := []string{m.style.bold(m.lang.Text("Basic information"))}
	for _, field := range fields {
		label := m.style.dim(fit(m.lang.Text(field[0]), labelWidth))
		for _, value := range wrapText(safe(orDash(field[1])), w-labelWidth) {
			lines = append(lines, label+value)
			label = strings.Repeat(" ", labelWidth)
		}
	}
	return lines
}

func (m *model) dnsRowResult(row dnsRow) string {
	var values []string
	if row.cache {
		values = append(values, m.lang.Text("Cached"))
	}
	if row.status != "" {
		values = append(values, m.lang.Text(row.status))
	}
	if row.result != "" {
		values = append(values, row.result)
	}
	return orDash(strings.Join(values, " · "))
}

func (m *model) dnsElapsed(row dnsRow) string {
	if !row.timed {
		return "—"
	}
	return fmt.Sprintf("%d ms", row.elapsed.Milliseconds())
}

func (m *model) dnsQueryPage(height int) []string {
	s := m.style
	toolbar := m.controls([]control{
		{"c", "Flush DNS cache"}, {"a", "DNS query"}, {m.settings.key("search"), "Search"}, {"d", "Clear records"},
	}, 0, 3, m.width)
	lines := []string{toolbar}
	var chips string
	for _, key := range m.dnsFilters() {
		label := "[" + safe(m.lang.Text(key)) + "]"
		if width(chips)+width(label) > m.width {
			lines = append(lines, chips)
			chips = ""
		}
		if len(lines) >= 3 {
			break
		}
		x, y := width(chips), 3+len(lines)
		if m.dnsFeed.filters[key] {
			label = s.bold(s.accent(label))
		} else {
			label = s.dim(label)
		}
		m.hits = append(m.hits, hit{kind: hitDNSFilter, key: key, x0: x, x1: min(m.width, x+width(label)), y0: y, y1: y + 1})
		chips += label + " "
	}
	if chips != "" {
		lines = append(lines, chips)
	}
	if m.filter != "" {
		lines = append(lines, s.dim(m.lang.Text("Filter  "))+safe(m.filter))
	}
	widths := []int{8, 16, 5, 7, 11, 9, 8}
	spare := max(0, m.width-71)
	widths[1] += spare / 3
	widths[4] += spare / 3
	widths[5] += spare - 2*(spare/3)
	// Keep the answer and timing columns visible in compact terminals.
	if m.width < 71 {
		widths[1] -= min(8, (71-m.width+1)/2)
		widths[4] -= min(5, (71-m.width)/2)
	}
	format := func(cells []string) string {
		var columns []string
		for i, cell := range cells {
			columns = append(columns, fit(clip(safe(cell), widths[i]), widths[i]))
		}
		return strings.Join(columns, " ")
	}
	header := []string{"Timestamp", "Domain", "Type", "From", "Result", "Source", "Duration"}
	for i := range header {
		header[i] = m.lang.Text(header[i])
	}
	lines = append(lines, " "+s.bold(s.dim(format(header))))
	rows := m.visibleDNSRows()
	if len(rows) == 0 {
		label := "Waiting for DNS queries"
		if len(m.dnsFeed.rows) > 0 {
			label = "No matching entries"
		}
		return append(lines, s.dim(m.lang.Text(label)))
	}
	visible := height - len(lines)
	cursor := min(m.cursor, len(rows)-1)
	start := max(0, cursor-visible+1)
	for i := start; i < min(len(rows), start+visible); i++ {
		row := rows[i]
		y := 3 + len(lines)
		m.hits = append(m.hits, hit{kind: hitRow, index: i, x1: m.width, y0: y, y1: y + 1})
		prefix := " "
		if i == cursor {
			prefix = s.accent("›")
		}
		lines = append(lines, prefix+format([]string{row.at.Format("15:04:05"), row.name, row.kind,
			orDash(m.lang.Text(row.initiator)), m.dnsRowResult(row), orDash(row.source), m.dnsElapsed(row)}))
	}
	return lines
}

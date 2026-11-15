package tui

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Nyakooo/termihomo/internal/core"
)

type requestRate struct{ down, up int64 }

type requestState struct {
	rates            map[string]requestRate
	protocols, rules map[string]bool
	category         int
	column           int
	menu             string
	option           int
}

var requestCategories = []string{"None", "Application", "Host", "Rule", "Exit"}

var requestColumns = []struct {
	label, compact string
	width, narrow  int
}{
	{"Process", "Process", 14, 10}, {"Destination", "Destination", 19, 15},
	{"Protocol", "Proto", 8, 5}, {"Rule", "Rule", 16, 11}, {"Exit", "Exit", 13, 9},
	{"Download", "↓/s", 9, 7}, {"Upload", "↑/s", 9, 7},
	{"Download total", "↓ Total", 10, 8}, {"Upload total", "↑ Total", 10, 8},
	{"Duration", "Time", 8, 6},
}

func (m *model) updateRequestSamples(next snapshot) {
	r := &m.requests
	r.rates = nil
	old := m.snapshot
	elapsed := next.connectionsAt.Sub(old.connectionsAt).Seconds()
	if old.connectionsAt.IsZero() || next.connectionsAt.IsZero() || elapsed <= 0 ||
		old.connectionErr != nil || next.connectionErr != nil || old.status.PID != next.status.PID ||
		old.status.State != "Running" || next.status.State != "Running" {
		return
	}
	previous := make(map[string]core.Connection, len(old.connections))
	for _, c := range old.connections {
		previous[c.ID] = c
	}
	r.rates = make(map[string]requestRate, len(next.connections))
	for _, c := range next.connections {
		if before, ok := previous[c.ID]; ok {
			r.rates[c.ID] = requestRate{
				down: int64(float64(max(0, c.Download-before.Download)) / elapsed),
				up:   int64(float64(max(0, c.Upload-before.Upload)) / elapsed),
			}
		}
	}
}

func requestProcess(c core.Connection) string {
	name := c.Metadata.Process
	if name == "" && c.Metadata.ProcessPath != "" {
		name = filepath.Base(c.Metadata.ProcessPath)
	}
	if c.Metadata.UID != 0 {
		if name == "" {
			return fmt.Sprint(c.Metadata.UID)
		}
		return fmt.Sprintf("%s(%d)", name, c.Metadata.UID)
	}
	return orDash(name)
}

func requestExit(c core.Connection) string {
	if len(c.Chains) > 0 {
		return c.Chains[0]
	}
	return "—"
}

func requestRule(c core.Connection) string {
	return orDash(strings.TrimSpace(c.Rule + " " + c.RulePayload))
}

func requestCategory(c core.Connection, category int) string {
	switch category {
	case 1:
		return requestProcess(c)
	case 2:
		return orDash(cmp.Or(c.Metadata.Host, c.Metadata.DestinationIP))
	case 3:
		return requestRule(c)
	case 4:
		return requestExit(c)
	}
	return ""
}

func (m *model) visibleConnections() []core.Connection {
	var items []core.Connection
	for _, c := range m.snapshot.connections {
		protocol := strings.ToUpper(c.Metadata.Network)
		if len(m.requests.protocols) > 0 && !m.requests.protocols[protocol] ||
			len(m.requests.rules) > 0 && !m.requests.rules[c.Rule] {
			continue
		}
		if m.filter != "" {
			hay := strings.ToLower(strings.Join([]string{connectionHost(c), c.Metadata.DestinationIP, protocol, c.Metadata.Type,
				requestProcess(c), c.Metadata.ProcessPath, requestRule(c), strings.Join(c.Chains, " ")}, " "))
			if !strings.Contains(hay, m.filter) {
				continue
			}
		}
		items = append(items, c)
	}
	slices.SortStableFunc(items, func(a, b core.Connection) int {
		if m.requests.category != 0 {
			if order := cmp.Compare(requestCategory(a, m.requests.category), requestCategory(b, m.requests.category)); order != 0 {
				return order
			}
		}
		return cmp.Or(cmp.Compare(b.Download+b.Upload, a.Download+a.Upload), b.Start.Compare(a.Start))
	})
	return items
}

func (m *model) requestFilters(protocol bool) []string {
	var values []string
	for _, c := range m.snapshot.connections {
		value := c.Rule
		if protocol {
			value = strings.ToUpper(c.Metadata.Network)
		}
		if value != "" {
			values = append(values, value)
		}
	}
	slices.Sort(values)
	return slices.Compact(values)
}

func (m *model) pruneRequestFilters() {
	for _, protocol := range []bool{true, false} {
		selection := *m.requestSelection(protocol)
		if len(selection) == 0 {
			continue
		}
		active := m.requestFilters(protocol)
		for value := range selection {
			if _, ok := slices.BinarySearch(active, value); !ok {
				delete(selection, value)
			}
		}
	}
}

func (m *model) requestSelection(protocol bool) *map[string]bool {
	if protocol {
		return &m.requests.protocols
	}
	return &m.requests.rules
}

func (m *model) toggleRequestFilter(key string) {
	kind, value, _ := strings.Cut(key, ":")
	selection := m.requestSelection(kind == "protocol")
	if (*selection)[value] {
		delete(*selection, value)
	} else {
		if *selection == nil {
			*selection = make(map[string]bool)
		}
		(*selection)[value] = true
	}
	m.resetRequestSelection()
}

func (m *model) resetRequestSelection() {
	m.cursor = 0
	m.connectionSelection = nil
}

func (m *model) requestOptions() ([]string, int) {
	switch m.requests.menu {
	case "Category":
		return requestCategories, m.requests.category

	}
	protocol := m.requests.menu == "Protocol"
	values := m.requestFilters(protocol)
	selected := *m.requestSelection(protocol)
	index := slices.IndexFunc(values, func(value string) bool { return selected[value] })
	return append([]string{"All"}, values...), index + 1
}

func (m *model) openRequestMenu(name string) {
	m.requests.menu = name
	_, m.requests.option = m.requestOptions()
}

func (m *model) requestMenuKey(key string) {
	options, _ := m.requestOptions()
	m.requests.option = min(m.requests.option, len(options)-1)
	switch key {
	case "esc", "c", "ctrl+p", "ctrl+t":
		m.requests.menu = ""
	case "j", "down":
		m.requests.option = (m.requests.option + 1) % len(options)
	case "k", "up":
		m.requests.option = (m.requests.option + len(options) - 1) % len(options)
	case "enter", "space":
		m.chooseRequestOption(m.requests.option)
	}
}

func (m *model) chooseRequestOption(index int) {
	options, _ := m.requestOptions()
	if index >= len(options) {
		return
	}
	switch m.requests.menu {
	case "Category":
		m.requests.category = index

	case "Protocol", "Rule":
		protocol := m.requests.menu == "Protocol"
		if index == 0 {
			*m.requestSelection(protocol) = nil
			m.resetRequestSelection()
		} else {
			kind := "rule"
			if protocol {
				kind = "protocol"
			}
			m.toggleRequestFilter(kind + ":" + options[index])
		}
		m.requests.option = index
		return
	}
	m.requests.menu = ""
	m.resetRequestSelection()
}

func (m *model) confirmCloseAllRequests() {
	if m.busy || len(m.snapshot.connections) == 0 {
		return
	}
	ids := make([]string, 0, len(m.snapshot.connections))
	for _, c := range m.snapshot.connections {
		ids = append(ids, c.ID)
	}
	m.confirmLabel = fmt.Sprintf(m.lang.Text("Close all %d active connections"), len(ids))
	m.confirm = func() error { return m.manager.CloseConnections(m.ctx, ids) }
}

func (m *model) requestMenu(body []string, height int) []string {
	options, current := m.requestOptions()
	w := 26
	x := m.width - w
	available := height - 3
	focus := min(m.requests.option, len(options)-1)
	first := max(0, focus-available+1)
	rows := make([]string, 0, min(available, len(options))+1)
	for i := first; i < min(first+available, len(options)); i++ {
		text := "  " + safe(m.lang.Text(options[i]))
		selected := i == current
		if m.requests.menu == "Protocol" || m.requests.menu == "Rule" {
			selection := *m.requestSelection(m.requests.menu == "Protocol")
			selected = (i == 0 && len(selection) == 0) || (i > 0 && selection[options[i]])
		}
		if selected {
			text += " ✓"
		}
		if i == focus {
			text = m.style.accent("› " + strings.TrimPrefix(text, "  "))
		}
		y := 5 + len(rows)
		m.hits = append(m.hits, hit{kind: hitRequestOption, index: i, x0: x + 1, x1: x + w - 1, y0: y, y1: y + 1})
		rows = append(rows, text)
	}
	out := clampLines(body, height)
	for i, line := range m.style.panel(m.lang.Text(m.requests.menu), rows, w) {
		out[i+1] = fit(out[i+1], x) + line
	}
	return out
}

func (m *model) requestCells(c core.Connection) []string {
	rate := m.requests.rates[c.ID]
	duration := "—"
	if !c.Start.IsZero() {
		duration = max(0, time.Since(c.Start)).Truncate(time.Second).String()
	}
	return []string{
		requestProcess(c), connectionHost(c), strings.ToUpper(orDash(c.Metadata.Network)), requestRule(c), requestExit(c),
		requestRateLabel(rate.down), requestRateLabel(rate.up), bytesLabel(c.Download), bytesLabel(c.Upload), duration,
	}
}

func requestRateLabel(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB/s", bytes)
	}
	return strings.ReplaceAll(bytesLabel(bytes), " ", "") + "/s"
}

func (m *model) requestTableLayout() (first, last int, widths []int, compact bool) {
	widths = make([]int, len(requestColumns))
	total := len(widths) - 1
	for i, col := range requestColumns {
		widths[i] = max(col.width, width(m.lang.Text(col.label)))
		total += widths[i]
	}
	available := m.width - 2
	compact = total > available
	if compact {
		total = len(widths) - 1
		for i, col := range requestColumns {
			widths[i] = max(col.narrow, width(m.lang.Text(col.compact)))
			total += widths[i]
		}
	}
	if total <= available {
		spare := available - total
		widths[1] += (spare + 1) / 2
		widths[3] += spare / 2
		return 0, len(widths), widths, compact
	}
	maxFirst, used := len(widths), 0
	for i := len(widths) - 1; i >= 0 && used+widths[i] <= available; i-- {
		maxFirst = i
		used += widths[i] + 1
	}
	first = min(m.requests.column, maxFirst)
	last, used = first, 0
	for last < len(widths) && used+widths[last] <= available {
		used += widths[last] + 1
		last++
	}
	return first, last, widths, compact
}

func (m *model) connectionsPageView(height int) []string {
	s := m.style
	state := m.statusState()
	left := s.bold(s.fg(stateHex(s, state), m.lang.Text(state)))
	actions := []control{{m.settings.key("search"), "Search"}, {"d", "Clear requests"}, {"c", "Category"}}
	rightWidth := len(actions) - 1
	for _, action := range actions {
		rightWidth += width(m.controlText(action))
	}
	right := m.controls(actions, m.width-rightWidth, 3, rightWidth)
	lines := []string{fit(left, m.width-rightWidth) + right}
	var chips string
	type filterChip struct {
		key, label string
		selected   bool
	}
	var filters []filterChip
	for _, protocol := range m.requestFilters(true) {
		filters = append(filters, filterChip{"protocol:" + protocol, protocol, m.requests.protocols[protocol]})
	}
	for _, rule := range m.requestFilters(false) {
		filters = append(filters, filterChip{"rule:" + rule, rule, m.requests.rules[rule]})
	}
	for _, filter := range filters {
		label := "[" + safe(filter.label) + "]"
		if width(chips)+width(label) > m.width {
			lines = append(lines, strings.TrimRight(chips, " "))
			chips = ""
		}
		x, y := width(chips), 3+len(lines)
		if filter.selected {
			label = s.bold(s.accent(label))
		} else {
			label = s.dim(label)
		}
		m.hits = append(m.hits, hit{kind: hitRequestFilter, key: filter.key, x0: x, x1: min(m.width, x+width(label)), y0: y, y1: y + 1})
		chips += label + " "
		if len(lines) >= 3 {
			break
		}
	}
	if chips != "" {
		lines = append(lines, chips)
	}
	if m.filter != "" {
		lines = append(lines, s.dim(m.lang.Text("Filter  "))+safe(m.filter))
	}
	first, last, widths, compact := m.requestTableLayout()
	cellLine := func(cells []string) string {
		columns := make([]string, 0, last-first)
		for i := first; i < last; i++ {
			cell := clip(safe(cells[i]), widths[i])
			if i >= 5 {
				columns = append(columns, padLeft(cell, widths[i]))
			} else {
				columns = append(columns, fit(cell, widths[i]))
			}
		}
		return strings.Join(columns, " ")
	}
	header := make([]string, len(requestColumns))
	for i, col := range requestColumns {
		label := col.label
		if compact {
			label = col.compact
		}
		header[i] = m.lang.Text(label)
	}
	leftMark, rightMark := " ", " "
	if first > 0 {
		y := 3 + len(lines)
		m.hits = append(m.hits, hit{kind: hitAction, key: "left", x1: 1, y0: y, y1: y + 1})
		leftMark = "‹"
	}
	if last < len(requestColumns) {
		y := 3 + len(lines)
		m.hits = append(m.hits, hit{kind: hitAction, key: "right", x0: m.width - 1, x1: m.width, y0: y, y1: y + 1})
		rightMark = "›"
	}
	lines = append(lines, s.bold(s.dim(leftMark+fit(cellLine(header), m.width-2)+rightMark)),
		s.faint(strings.Repeat("─", m.width)))
	items := m.visibleConnections()
	if len(items) == 0 {
		return clampLines(append(lines, m.listEmpty("No active connections", m.snapshot.connectionErr)), height)
	}
	type tableRow struct {
		group string
		index int
	}
	rows := make([]tableRow, 0, len(items))
	var group string
	focused := min(m.cursor, len(items)-1)
	focusRow := 0
	for i, c := range items {
		if name := requestCategory(c, m.requests.category); name != group {
			group = name
			rows = append(rows, tableRow{group: name})
		}
		if i == focused {
			focusRow = len(rows)
		}
		rows = append(rows, tableRow{index: i})
	}
	available := height - len(lines)
	start := max(0, focusRow-available+1)
	for _, row := range rows[start:min(len(rows), start+available)] {
		if row.group != "" {
			lines = append(lines, s.bold(s.accent("┄ "+safe(row.group))))
			continue
		}
		c := items[row.index]
		prefix := " "
		if m.connectionSelection[c.ID] {
			prefix = s.ok("✓")
		} else if row.index == focused {
			prefix = s.accent("›")
		}
		line := cellLine(m.requestCells(c))
		if row.index == focused {
			line = s.bold(line)
		}
		y := 3 + len(lines)
		m.hits = append(m.hits, hit{kind: hitRow, index: row.index, x1: m.width, y0: y, y1: y + 1})
		lines = append(lines, prefix+line)
	}
	return clampLines(lines, height)
}

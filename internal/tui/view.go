package tui

import (
	"fmt"
	"math"
	"net"
	"os"
	"slices"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/i18n"
	"github.com/charmbracelet/x/ansi"
)

const minCols = 80
const compactMinCols = 64
const minRows = 24

type menuSection struct {
	label string
	icon  string
	pages []page
}

var menuSections = []menuSection{
	{"Dashboard", "⊞", []page{dashboard}},
	{"Proxy menu", "⇆", []page{proxies}},
	{"Profile", "▤", []page{profiles}},
	{"Requests", "◷", []page{connectionsPage}},
	{"Tools", "⚙", []page{generalPage, languagePage, networkPage, corePage, dnsPage, logs}},
}

func (m *model) sidebarWidth() int {
	if m.compactTerminal && m.width < minCols {
		return 5
	}
	w := width(" TERMIHOMO") + 1
	for _, section := range menuSections {
		w = max(w, width(m.lang.Text(section.label))+9)
	}
	return w
}

func (m *model) minimumColumns() int {
	if m.compactTerminal {
		return compactMinCols
	}
	return minCols
}

func (m *model) View() tea.View {
	m.hits = m.hits[:0]
	if m.width < m.minimumColumns() || m.height < minRows {
		view := tea.NewView(m.resizeNotice())
		view.AltScreen = true
		return view
	}
	s := m.style
	sideWidth := m.sidebarWidth()
	content := *m
	content.width -= sideWidth
	content.hits = nil
	header := []string{content.topbar(), content.pageHeading(), s.fg(s.p.bd, strings.Repeat("─", content.width))}
	bodyHeight := content.height - len(header) - 1
	body := content.body(bodyHeight)
	actions := content.actionBar()
	requestMenu := m.page == connectionsPage && m.requests.menu != ""
	modal := m.confirm != nil || m.form != "" || m.help || m.detail || m.services.dialog || requestMenu
	if modal {
		content.hits = nil
		if requestMenu {
			body = content.requestMenu(body, bodyHeight)
		} else {
			body = content.overlayModal(body, bodyHeight)
		}
	}
	lines := append(header, clampLines(body, bodyHeight)...)
	lines = append(lines, content.statusbar(actions))
	sidebar := m.sidebar(sideWidth, !modal)
	for index := range lines {
		line := ansi.Truncate(lines[index], content.width, "")
		lines[index] = sidebar[index] + fit(line, content.width)
	}
	for _, region := range content.hits {
		region.x0 += sideWidth
		region.x1 += sideWidth
		m.hits = append(m.hits, region)
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func menuIndex(current page) int {
	for i, section := range menuSections {
		if slices.Contains(section.pages, current) {
			return i
		}
	}
	return 0
}

func (m *model) pageHeading() string {
	section := menuSections[menuIndex(m.page)]
	s := m.style
	line := s.bold(s.fg(s.p.fg, " "+m.lang.Text(section.label)))
	if len(section.pages) < 2 {
		return line
	}
	line += "  │"
	for _, dest := range section.pages {
		label := " " + m.lang.Text(pages[dest]) + " "
		x := width(line)
		m.hits = append(m.hits, hit{kind: hitTab, index: int(dest), x0: x, x1: x + width(label), y0: 1, y1: 2})
		if dest == m.page {
			label = s.bold(s.accent("[" + strings.TrimSpace(label) + "]"))
		}
		line += label
	}
	return line
}

func (m *model) sidebar(w int, clickable bool) []string {
	s := m.style
	lines := make([]string, 0, m.height)
	add := func(text string, active bool) {
		line := fit(clip(text, w-1), w-1)
		if active {
			line = s.bold(s.accent(line))
		} else {
			line = s.fg(s.p.dim, line)
		}
		lines = append(lines, line+s.fg(s.p.bd, "│"))
	}
	link := func(text string, dest page, active bool, top, bottom int) {
		if clickable {
			y := len(lines)
			m.hits = append(m.hits, hit{kind: hitTab, x0: 0, x1: w, y0: y, y1: y + top + 1 + bottom, index: int(dest)})
		}
		for range top {
			add("", false)
		}
		add(text, active)
		for range bottom {
			add("", false)
		}
	}
	brand := " TERMIHOMO"
	if w == 5 {
		brand = " ◈"
	}
	add(s.bold(s.accent(brand)), false)
	add("", false)
	for i, section := range menuSections {
		active := slices.Contains(section.pages, m.page)
		marker := " "
		if active {
			marker = "▏"
		}
		link(fmt.Sprintf("%s%d %s%s", marker, i+1, fit(section.icon, 2), m.lang.Text(section.label)), section.pages[0], active, 1, 1)
	}
	for len(lines) < m.height {
		add("", false)
	}
	return lines
}

func (m *model) resizeNotice() string {
	s := m.style
	w := max(40, min(m.width, 60))
	if m.width < 4 {
		w = max(4, m.width)
	}
	lines := []string{
		s.bold(s.warn(m.lang.Text("Terminal too small"))),
		fmt.Sprintf(m.lang.Text("Minimum display size is %d×%d"), m.minimumColumns(), minRows),
		fmt.Sprintf(m.lang.Text("Current size %d×%d"), m.width, m.height),
		s.dim(m.lang.Text("Resize the terminal to continue")),
	}
	box := s.panel("", lines, w)
	top := max(0, (m.height-len(box))/2)
	out := make([]string, 0, top+len(box))
	for i := 0; i < top; i++ {
		out = append(out, "")
	}
	out = append(out, box...)
	for i := range out {
		out[i] = ansi.Truncate(out[i], max(0, m.width), "")
	}
	if len(out) > m.height {
		out = out[:max(0, m.height)]
	}
	return strings.Join(out, "\n")
}

func clampLines(lines []string, height int) []string {
	if height < 0 {
		height = 0
	}
	if len(lines) > height {
		return lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

func stateHex(s styler, state string) string {
	switch state {
	case "Running":
		return s.p.ok
	case "Starting", "Stopping", "Degraded":
		return s.p.warn
	case "Stopped", "Paused":
		return s.p.dim
	case "NotInstalled":
		return s.p.faint
	case "Error", "Failed":
		return s.p.err
	default:
		return s.p.info
	}
}

// fitFg colors text and pads it with unstyled spaces to exactly w columns.
func (s styler) fitFg(hex, text string, w int) string {
	if w <= 0 {
		return ""
	}
	text = ansi.Truncate(text, w, "…")
	return s.fg(hex, text) + strings.Repeat(" ", max(0, w-width(text)))
}

func padLeft(text string, w int) string {
	if width(text) >= w {
		return text
	}
	return strings.Repeat(" ", w-width(text)) + text
}

// fitPlain pads or truncates unstyled text to exactly w display columns.
func fitPlain(text string, w int) string {
	if w <= 0 {
		return ""
	}
	text = ansi.Truncate(text, w, "…")
	if pad := w - width(text); pad > 0 {
		return text + strings.Repeat(" ", pad)
	}
	return text
}

// panelAt renders a panel padded or clipped to exactly h rows, so list pages
// keep a stable frame regardless of how many entries they contain.
func (s styler) panelAt(title string, lines []string, w, h int) []string {
	if h < 3 {
		h = 3
	}
	inner := h - 2
	if len(lines) > inner {
		lines = lines[:inner]
	}
	for len(lines) < inner {
		lines = append(lines, "")
	}
	return s.panel(title, lines, w)
}

func uptime(start time.Time) string {
	if start.IsZero() {
		return "—"
	}
	d := time.Since(start).Truncate(time.Second)
	if d < 0 {
		d = 0
	}
	return d.String()
}

func (m *model) statusState() string {
	if state := m.snapshot.status.State; state != "" {
		return state
	}
	return "Loading"
}

func (m *model) topbar() string {
	s := m.style
	st := m.snapshot.status
	brand := s.bold(s.accent(" ◈ TERMIHOMO "))
	left := brand + " " + s.faint(orDash(st.Installed))
	state := m.statusState()
	pills := []string{s.chip(m.lang.Text(state), stateHex(s, state))}
	if st.State == "Running" {
		mode := m.snapshot.overview.Mode
		if mode == "" {
			mode = "rule"
		}
		pills = append(pills, s.chip(m.lang.Text(mode), s.p.info))
		if m.snapshot.overview.TUN {
			pills = append(pills, s.chip("TUN", s.p.ok))
		}
		pills = append(pills, s.chip("⏱ "+uptime(st.Started), s.p.dim))
	}
	avail := m.width
	mid := strings.Join(pills, " ")
	if width(left)+width(mid)+1 > avail {
		mid = ""
	}
	pad := max(0, avail-width(left)-width(mid))
	line := left + strings.Repeat(" ", pad) + mid
	return line
}

func (m *model) statusbar(actions string) string {
	right := m.rateSummary()
	left := safe(m.feedback)
	if left == "" {
		left = m.style.dim("Tab " + m.lang.Text("Switch page"))
		if m.page == proxies {
			left += m.style.dim("  ·  hjkl / ↑↓←→  ·  [ ]")
		} else if m.page == connectionsPage || m.page == logs {
			left += m.style.dim("  ·  j / k " + m.lang.Text("Move selection") + "  ·  gg / End")
		}
	}
	available := m.width - width(actions)
	if available < 4 {
		return actions
	}
	info := clip(left, available-3)
	if available-width(info) >= width(right)+5 {
		info = fit(info, available-width(right)-5) + "  " + right
	}
	return actions + m.style.dim(" │ ") + info
}

func (m *model) rateSummary() string {
	s := m.style
	live := s.ok(m.lang.Text("LIVE"))
	if !m.trafficLive {
		live = s.warn(m.lang.Text("STALE"))
	}
	if len(m.rates) == 0 {
		return s.faint("↑ —  ↓ —") + "  " + live
	}
	cur := m.rates[len(m.rates)-1]
	up := s.fg(s.p.up, "↑ "+bytesLabel(cur.Up)+"/s")
	down := s.fg(s.p.down, "↓ "+bytesLabel(cur.Down)+"/s")
	return up + "  " + down + "  " + live
}

type control struct {
	key, label string
}

func (m *model) controlText(c control) string {
	s := m.style
	key := c.key
	switch key {
	case "enter":
		key = "↵"
	case "esc":
		key = "Esc"
	case "profile-update":
		key = "↻"
	case "ctrl+a":
		key = "Ctrl+A"
	case "ctrl+r":
		key = "Ctrl+R"
	case "ctrl+v":
		key = "Ctrl+V"
	}
	label := m.lang.Text(c.label)
	accent := s.p.acc
	switch c.label {
	case "Start", "Confirm":
		accent = s.p.ok
	case "Stop", "Close connection":
		accent = s.p.err
	}
	return s.dim("[") + s.fg(accent, key) + " " + label + s.dim("]")
}

func (m *model) controls(controls []control, x, y, available int) string {
	var line string
	total := max(0, len(controls)-1)
	for _, c := range controls {
		total += width(m.controlText(c))
	}
	compact := total > available
	for _, c := range controls {
		label := m.controlText(c)
		if compact {
			label = "[" + m.style.accent(c.key) + " " + m.lang.Text(c.label) + "]"
		}
		w := width(label)
		if w > available {
			break
		}
		m.hits = append(m.hits, hit{kind: hitAction, key: c.key, x0: x, x1: x + w, y0: y, y1: y + 1})
		if line != "" {
			line += " "
		}
		line += label
		x += w + 1
		available -= w + 1
	}
	return line
}

func (m *model) actionBar() string {
	k := m.settings.key
	var actions []control
	switch m.page {
	case dashboard:
		if m.kernelActive() {
			actions = []control{{"ctrl+r", "Restart"}}
		}

	case proxies:
		if m.snapshot.overview.Mode != "direct" {
			actions = []control{{"e", "Detail"}}
		}
	case profiles:
		actions = []control{{"enter", "Switch profile"}}
	case logs:
		actions = []control{{k("log-level"), "Level"}, {"p", "Pause"}, {"c", "Clear"}}
		if m.logPaused {
			actions[1] = control{"p", "Resume"}
		}
	case corePage:
		actions = []control{{"i", "Install"}, {"a", "Start"}, {"x", "Stop"}, {"ctrl+r", "Restart"}}
	case generalPage:
		actions = []control{{"w", "LAN proxy"}, {"ctrl+r", "Apply"}}
	case networkPage:
		actions = []control{{"ctrl+r", "Apply"}}
		if m.showProxyExclusions {
			actions = []control{{"a", "Add"}, {"esc", "Back"}}
			if len(m.snapshot.systemProxy.Exclusions) > 0 {
				actions = []control{{"a", "Add"}, {"enter", "Edit"}, {"d", "Delete"}, {"esc", "Back"}}
			}
		}
	case connectionsPage:
		actions = []control{{"ctrl+a", "Select all"}, {"x", "Close"}}
	}
	if m.filter != "" {
		actions = append(actions, control{"esc", "Back"})
	}
	global := []control{{k("refresh"), "Refresh"}, {k("help"), "Help"}, {k("quit"), "Quit"}}
	available := m.width
	all := append(actions, global...)
	for _, item := range all {
		available -= width(m.controlText(item)) + 1
	}
	if available < -1 {
		all = slices.Clone(all)
		for i, item := range all {
			switch item.label {
			case "Replace subscription":
				all[i].label = "Replace"
			case "Replace with YAML":
				all[i].label = "YAML"
			case "Add subscription":
				all[i].label = "Add"
			case "Close connection":
				all[i].label = "Close"
			}
		}
	}
	return m.controls(all, 0, m.height-1, m.width)
}

func (m *model) body(height int) []string {
	switch m.page {
	case dashboard:
		return m.dashboardPage(height)
	case proxies:
		return clampLines(m.proxyGrid(height), height)
	case profiles:
		return m.profilesPage(height)
	case logs:
		return m.logsPage(height)
	case corePage:
		return m.corePageView(height)
	case dnsPage:
		return m.dnsQueryPage(height)
	case generalPage:
		return m.toolPage(height)
	case networkPage:
		return m.networkPageView(height)
	case languagePage:
		status := "English"
		if m.lang == i18n.Chinese {
			status = "简体中文"
		}
		buttons := m.controls([]control{{"e", "English"}, {"c", "简体中文"}}, 2, 5, m.width-4)
		return m.style.panel(m.lang.Text("Language"), []string{
			m.lang.Text("Current language") + " · " + status, buttons,
		}, m.width)
	case connectionsPage:
		return m.connectionsPageView(height)
	}
	return nil
}

func (m *model) overlayModal(body []string, height int) []string {
	var lines []string
	var buttons []control
	var title string
	inputRow, inputStart, inputX := -1, 0, 2
	fieldRows := map[int]string{}
	splitButtons := false
	w := min(68, m.width-8)
	switch {
	case m.confirm != nil:
		title = "Confirm"
		lines = wrapText(safe(m.confirmLabel)+m.lang.Text("?"), w-4)
		buttons = []control{{"y", "Confirm"}, {"n", "Cancel"}}
	case m.form != "":
		title = m.form
		if m.formError != "" {
			for _, line := range wrapText(safe(m.formError), w-4) {
				lines = append(lines, m.style.err(line))
			}
			lines = append(lines, "")
		}
		input, start := m.formInput(w - 6)
		inputStart = start
		if m.form == "Subscription URL" || m.form == "Edit profile" || m.form == "Local YAML path" {
			splitButtons = true
			linkLabel, linkHint := "Link", "Required"
			switch m.form {
			case "Subscription URL":
				title = "Import from link"
			case "Local YAML path":
				title, linkLabel = "Import from file", "File path"
			case "Edit profile":
				linkHint = "Keep current link"
			}
			inputX = 15
			fieldWidth := w - 4 - inputX
			input, inputStart = m.formInput(fieldWidth)
			row := func(key, label, value, hint string, active bool) {
				fieldRows[len(lines)] = key
				labelText := m.style.dim(fit(clip(m.lang.Text(label), inputX-1), inputX))
				if active {
					inputRow = len(lines)
					lines = append(lines, m.style.accent(fit(clip(m.lang.Text(label), inputX-1), inputX))+input)
				} else {
					if value == "" {
						value = m.lang.Text(hint)
					}
					lines = append(lines, labelText+padLeft(clip(safe(value), fieldWidth), fieldWidth))
				}
			}
			divider := m.style.faint(strings.Repeat("─", w-4))
			row("profile-name", "Name", m.profileName, "Optional", m.nameFocused)
			lines = append(lines, divider)
			row("profile-link", linkLabel, m.profileLink, linkHint, !m.nameFocused && !m.intervalFocused)
			if m.form == "Edit profile" && m.editProfileSource == "subscription" {
				lines = append(lines, "", divider)
				state := "on"
				if m.profileSchedule.Disabled {
					state = "off"
				}
				row("profile-auto", "Auto update", m.lang.Text(state)+" (Ctrl+A)", "", false)
				lines = append(lines, divider)
				row("profile-interval", "Interval (min)", m.profileInterval, "", m.intervalFocused)
			}
			submit := "Submit"
			if m.form == "Edit profile" {
				submit = "Save"
			}
			buttons = []control{{"esc", "Cancel"}, {"enter", submit}}
		} else if m.page == networkPage {
			splitButtons = true
			hint := "Enter one domain, IP address, or network range"
			switch m.form {
			case "Stack mode":
				hint = "system / gvisor / mixed · applies the next time TUN is enabled"
			case "Outbound interface":
				hint = "Enter an interface name, or leave empty for automatic"
			}
			lines = append(lines, wrapText(m.lang.Text(hint), w-4)...)
			inputRow, inputX = len(lines), 15
			input, inputStart = m.formInput(w - 4 - inputX)
			label := fit(clip(m.lang.Text("Value"), inputX-1), inputX)
			lines = append(lines, m.style.accent(label)+input)
			buttons = []control{{"esc", "Cancel"}, {"enter", "Submit"}}
		} else {
			if m.form == "DNS query" {
				lines = append(lines, m.style.dim(m.lang.Text("Enter a domain and optional record type")))
			} else if m.page == generalPage && m.form != "Filter" {
				if m.toolField.Key == "mixed-port" {
					lines = append(lines, m.style.dim(fmt.Sprintf(m.lang.Text("Leave empty to use port %d"), core.DefaultProxyPort)))
				} else {
					lines = append(lines, m.style.dim(m.lang.Text("Empty uses profile defaults")))
				}
				if m.toolField.Type != "text" && m.toolField.Key != "mixed-port" {
					lines = append(lines, m.style.dim(m.lang.Text("Value format")+" "+m.lang.Text(m.toolField.Type)))
				}
			}
			inputRow = len(lines)
			lines = append(lines, m.style.accent("› ")+input)
			buttons = []control{{"enter", "Save"}, {"esc", "Cancel"}, {"ctrl+v", "Paste"}}
		}
	case m.services.dialog:
		title = "Service status"
		lines = m.serviceDialog(height-4, w-4, fieldRows)
		buttons = []control{{"f", "Check all"}, {"enter", "Check selected"}, {"esc", "Close"}}
	case m.help:
		title = "Keyboard shortcuts"
		w = min(72, m.width-8)
		lines = m.helpLines()
		buttons = []control{{"esc", "Close"}}
	case m.detail:
		title = "Detail"
		switch m.page {
		case dnsPage:
			title = "DNS details"
			lines = m.dnsDetails(w - 4)
		case proxies:
			items := m.visibleProxies()
			if len(items) > 0 {
				lines = m.nodeDetail(items[min(m.cursor, len(items)-1)], w-4)
			}
		case connectionsPage:
			items := m.visibleConnections()
			if len(items) > 0 {
				lines = m.connectionDetail(items[min(m.cursor, len(items)-1)], w-4)
				buttons = append(buttons, control{"x", "Close connection"})
			}
		}
		buttons = append(buttons, control{"esc", "Back"})
	}
	lines = lines[:min(len(lines), height-4)]
	left := (m.width - w) / 2
	top := (height - len(lines) - 4) / 2
	buttonRow := top + 3 + 2 + len(lines)
	buttonLine := ""
	if splitButtons {
		leftButton := m.controls(buttons[:1], left+2, buttonRow, w-4)
		rightWidth := width(m.controlText(buttons[1]))
		rightButton := m.controls(buttons[1:], left+w-2-rightWidth, buttonRow, rightWidth)
		buttonLine = fit(leftButton, w-4-rightWidth) + rightButton
	} else {
		buttonLine = m.controls(buttons, left+2, buttonRow, w-4)
	}
	lines = append(lines, "", buttonLine)
	if inputRow >= 0 && inputRow < len(lines)-2 {
		y := top + 4 + inputRow
		m.hits = append(m.hits, hit{kind: hitInput, x0: left + 2 + inputX, x1: left + w - 2, y0: y, y1: y + 1, index: inputStart})
	}
	for row, key := range fieldRows {
		if row < len(lines)-2 {
			y := top + 4 + row
			m.hits = append(m.hits, hit{kind: hitAction, key: key, x0: left + 2, x1: left + w - 2, y0: y, y1: y + 1})
		}
	}
	box := m.style.panel(m.lang.Text(title), lines, w)
	out := clampLines(body, height)
	for i, line := range box {
		out[top+i] = strings.Repeat(" ", left) + line
	}
	return out
}

func (m *model) formInput(available int) (string, int) {
	cursor := min(m.inputCursor, len(m.input))
	start := cursor
	before := ""
	for start > 0 {
		next := safe(string(m.input[start-1])) + before
		if width(next) >= available {
			break
		}
		start--
		before = next
	}
	if cursor == len(m.input) {
		return before + m.style.caret(" "), start
	}
	current := safe(string(m.input[cursor]))
	if width(before)+width(current) > available {
		return before + m.style.caret(" "), start
	}
	return before + m.style.caret(current) +
		clip(safe(string(m.input[cursor+1:])), available-width(before)-width(current)), start
}

func (m *model) placeCaret(column, start int) {
	m.inputCursor = start
	for m.inputCursor < len(m.input) && column > 0 {
		column -= width(safe(string(m.input[m.inputCursor])))
		if column >= 0 {
			m.inputCursor++
		}
	}
}

// wrapText word-wraps text to w display columns, capped at six lines so a long
// error cannot overflow the form overlay. Words longer than w are hard-broken.
func wrapText(text string, w int) []string {
	if w <= 0 {
		return []string{text}
	}
	var lines []string
	var cur strings.Builder
	curW := 0
	flush := func() {
		if cur.Len() > 0 {
			lines = append(lines, cur.String())
			cur.Reset()
			curW = 0
		}
	}
	for _, word := range strings.Fields(text) {
		ww := width(word)
		if ww > w {
			flush()
			for _, r := range word {
				if curW+width(string(r)) > w {
					flush()
				}
				cur.WriteRune(r)
				curW += width(string(r))
			}
			continue
		}
		if curW > 0 && curW+1+ww > w {
			flush()
		}
		if curW > 0 {
			cur.WriteRune(' ')
			curW++
		}
		cur.WriteString(word)
		curW += ww
	}
	flush()
	if len(lines) > 6 {
		lines = lines[:6]
	}
	return lines
}

func (m *model) helpLines() []string {
	s := m.style
	k := m.settings.key
	row := func(keys, desc string) string { return s.accent(fit(keys, 14)) + s.dim(desc) }
	lines := []string{
		row("Tab / 1–5", m.lang.Text("Switch page")),
		row("[ / ]", m.lang.Text("Switch group or content tab")),
		row("↑↓←→ / hjkl", m.lang.Text("Move selection")),
		row("gg / End", m.lang.Text("First / last row")),
		row("Ctrl+U/D/B/F", m.lang.Text("Scroll half / full pages")),
		row(k("refresh"), m.lang.Text("Refresh")),
		row(k("search"), m.lang.Text("Search proxies, groups, rules or connections")),
		row(k("sort")+" / t / ↵", m.lang.Text("Sort / test focused node / apply node")),
		row("↵ / Esc", m.lang.Text("Open details / go back")),
		row("Ctrl+V", m.lang.Text("Paste from clipboard into a form")),
		row(m.lang.Text("Dashboard"), m.lang.Text("a start / stop")),
		row("f / e", m.lang.Text("Dashboard service check / list")),
		row(m.lang.Text("Profile"), m.lang.Text("a add · i import · u update")),
		row(m.lang.Text("Core"), m.lang.Text("i install · a start · x stop · Ctrl+R restart")),
		row(m.lang.Text("Logs"), k("log-level")+m.lang.Text(" level · p pause · c clear")),
		row(k("quit"), m.lang.Text("Quit and keep Mihomo running")),
	}
	return lines
}

func (m *model) kv(key, val string) string {
	return m.style.dim(fit(key, 12)) + val
}

// kvw clips the value so long names cannot push past the panel border.
func (m *model) kvw(key, val string, w int) string {
	return m.style.dim(fit(key, 12)) + clip(val, max(4, w-12))
}

func orDash(v string) string {
	if v == "" {
		return "—"
	}
	return v
}

func pidStr(pid int) string {
	if pid == 0 {
		return "—"
	}
	return fmt.Sprintf("%d", pid)
}

func (m *model) tunLabel() string {
	switch {
	case m.snapshot.overview.TUN:
		return "TUN on"
	case m.snapshot.overview.TUNConfigured:
		return "TUN inactive"
	case m.snapshot.overview.TUNUnavailable:
		return "TUN unavailable"
	default:
		return "TUN off"
	}
}

func (m *model) dashboardControls(top, leftWidth, rateWidth int) []string {
	const gap = 2
	x := 2
	var segments []string
	for _, item := range []control{{"b", "rule"}, {"m", "global"}, {"d", "direct"}} {
		marker := "○"
		if m.snapshot.overview.Mode == item.label {
			marker = "●"
		}
		text := marker + " " + m.lang.Text(item.label) + "(" + item.key + ")"
		if marker == "●" {
			text = m.style.bold(m.style.accent(text))
		}
		w := width(text)
		m.hits = append(m.hits, hit{kind: hitAction, key: item.key, x0: x, x1: x + w, y0: top + 1, y1: top + 2})
		x += w + 1
		segments = append(segments, text)
	}
	system, tun := "Unavailable", "off"
	if m.snapshot.systemProxy.Available {
		system = "off"
		if m.snapshot.systemProxy.Enabled {
			system = "on"
			if !m.snapshot.systemProxy.Owned {
				system = "External"
			}
		}
	}
	switch m.tunLabel() {
	case "TUN on":
		tun = "on"
	case "TUN unavailable", "TUN inactive":
		tun = "Unavailable"
	}
	right := leftWidth + gap
	toggle := func(title, key, label string, x int) []string {
		label = clip(m.lang.Text(label), rateWidth-8)
		return m.style.panel(m.lang.Text(title), []string{
			m.controls([]control{{key, label}}, x+2, top+1, rateWidth-4),
		}, rateWidth)
	}
	return joinH(gap,
		m.style.panel(m.lang.Text("Outbound mode"), []string{strings.Join(segments, " ")}, leftWidth),
		toggle("System proxy", "p", system, right),
		toggle("TUN", "u", tun, right+rateWidth+gap),
	)
}

func (s snapshot) outboundTarget() string {
	switch s.overview.Mode {
	case "direct":
		return "DIRECT"
	case "global":
		return "GLOBAL"
	case "rule":
		for _, rule := range s.rules {
			if strings.EqualFold(rule.Type, "match") {
				return rule.Proxy
			}
		}
	}
	return ""
}

// proxyChain follows the active selections through nested policy groups.
func (m *model) proxyChain(target string) string {
	selected := make(map[string]string)
	for _, proxy := range m.snapshot.proxies {
		if proxy.Now != "" {
			selected[proxy.Name] = proxy.Now
		}
	}
	chain := []string{safe(target)}
	for selected[target] != "" {
		next := selected[target]
		delete(selected, target)
		chain = append(chain, safe(next))
		target = next
	}
	return strings.Join(chain, " → ")
}

func (m *model) kernelActive() bool {
	return m.snapshot.status.State == "Running" || m.snapshot.status.State == "Degraded"
}

func (m *model) dashboardPower(cardWidth int) string {
	s := m.style
	icon, label, color := "▶", "Start", s.p.ok
	if m.kernelActive() {
		icon, label, color = "■", "Stop", s.p.err
	}
	if m.lifecycle != "" {
		return s.warn("… " + m.lang.Text(m.lifecycle))
	}
	text := s.bold(s.fg(color, "[a "+icon+" "+m.lang.Text(label)+"]"))
	x := cardWidth - 2 - width(text)
	m.hits = append(m.hits, hit{kind: hitAction, key: "a", x0: x, x1: x + width(text), y0: 5, y1: 6})
	return text
}

func (m *model) dashboardPage(height int) []string {
	s := m.style
	st := m.snapshot.status
	state := m.statusState()
	const gap = 2
	rateWidth := (m.width - 2*gap) / 4
	exitWidth := m.width - 2*gap - 2*rateWidth
	power := m.dashboardPower(exitWidth)
	address := s.dim(m.lang.Text("Proxy")+"  ") + safe(orDash(st.Proxy))
	powerSpace := exitWidth - 4 - width(power)
	status := s.chip(m.lang.Text(state), stateHex(s, state)) + s.dim("  "+uptime(st.Started))
	nodeSpace := exitWidth - 4 - width(status) - 2
	if st.State == "Running" && nodeSpace > 0 {
		if node := safe(serviceNode(m.snapshot)); node != "" {
			node = clip(node, nodeSpace)
			status = fit(status, exitWidth-4-width(node)) + s.accent(node)
		}
	}
	exit := []string{
		status,
		fit(clip(address, powerSpace-1), powerSpace) + power,
		s.dim(m.lang.Text("Version")+"  ") + safe(orDash(st.Installed)),
	}
	if m.snapshot.err != nil {
		exit[2] = s.err(m.lang.Error(m.snapshot.err))
	}
	var traffic core.Traffic
	if len(m.rates) > 0 {
		traffic = m.rates[len(m.rates)-1]
	}
	up, down := make([]int64, 0, len(m.rates)), make([]int64, 0, len(m.rates))
	for _, sample := range m.rates {
		up = append(up, sample.Up)
		down = append(down, sample.Down)
	}
	metric := func(label, color string, rate, total int64, samples []int64) []string {
		value := "—"
		if m.trafficLive {
			value = bytesLabel(rate) + "/s"
		}
		stats := s.bold(s.fg(color, value))
		totalLabel := s.dim(bytesLabel(total))
		space := rateWidth - 4 - width(stats) - width(totalLabel)
		if space > 0 {
			stats += strings.Repeat(" ", space) + totalLabel
		}
		return s.panelColor(m.lang.Text(label), append([]string{stats},
			s.areaChart(samples, rateWidth-4, 2, color)...), rateWidth, color)
	}
	lines := joinH(gap,
		s.panelColor(m.lang.Text("Overview"), exit, exitWidth, s.p.ok),
		metric("Upload", s.p.up, traffic.Up, traffic.UpTotal, up),
		metric("Download", s.p.down, traffic.Down, traffic.DownTotal, down),
	)
	lines = append(lines, m.dashboardControls(3+len(lines), exitWidth, rateWidth)...)
	lines = append(lines, m.dashboardInfo(3+len(lines), exitWidth, rateWidth)...)
	remaining := max(3, height-len(lines))
	groupWidth := m.width * 2 / 5
	connectionWidth := m.width - groupWidth - gap
	connections := m.snapshot.connections
	connRows := make([]string, 0, remaining-2)
	if connectionWidth >= 48 && len(connections) > 0 {
		hostWidth, chainWidth := connectionColumns(connectionWidth - 4)
		connRows = append(connRows, s.dim(fit(m.lang.Text("Host"), hostWidth)+" "+fit(m.lang.Text("Chains"), chainWidth)+
			padLeft("↓", 10)+padLeft("↑", 10)))
	}
	for _, connection := range connections[:min(len(connections), remaining-2-len(connRows))] {
		connRows = append(connRows, m.connectionRow(connection, connectionWidth-4))
	}
	if len(connRows) == 0 {
		connRows = append(connRows, m.listEmpty("No active connections", m.snapshot.connectionErr))
	}
	title := fmt.Sprintf("%s · %d  ›", m.lang.Text("Connections"), len(connections))
	y := 3 + len(lines)
	m.hits = append(m.hits, hit{kind: hitTab, x1: connectionWidth, y0: y, y1: y + 1, index: int(connectionsPage)})
	var groupRows []string
	if m.snapshot.overview.Mode != "direct" {
		for _, group := range m.proxyTabs() {
			if group.Name != "" {
				groupRows = append(groupRows, clip(m.proxyChain(group.Name), groupWidth-4))
				if len(groupRows) == remaining-2 {
					break
				}
			}
		}
	}
	if len(groupRows) == 0 {
		groupRows = append(groupRows, m.lang.Text("No selector groups"))
	}
	return append(lines, joinH(gap, s.panelAt(title, connRows, connectionWidth, remaining),
		s.panelAt(m.lang.Text("Groups"), groupRows, groupWidth, remaining))...)
}

func (m *model) dashboardInfo(top, leftWidth, rateWidth int) []string {
	s := m.style
	const gap = 2
	lanWidth := (leftWidth - gap) / 2
	profileWidth := leftWidth - lanWidth - gap
	address := "—"
	if len(m.snapshot.lanIPs) > 0 {
		address = m.snapshot.lanIPs[0]
	}
	_, proxyPort, _ := net.SplitHostPort(m.snapshot.status.Proxy)
	lan := "off"
	if m.snapshot.overview.LAN {
		lan = "on"
	}
	lanLabel := m.lang.Text("LAN proxy")
	lanToggle := control{"w", lan}
	lanLines := []string{s.bold(s.accent(address))}
	if width(lanLabel)+1+width(m.controlText(lanToggle)) > lanWidth-4 {
		lanLines = append(lanLines, s.dim(lanLabel))
		lanLabel = ""
	}
	x := 2
	if lanLabel != "" {
		lanLabel += " "
		x += width(lanLabel)
	}
	lanLines = append(lanLines, s.dim(lanLabel)+m.controls([]control{lanToggle}, x, top+1+len(lanLines), lanWidth-x-2),
		s.dim(m.lang.Text("Port")+" ")+orDash(proxyPort))
	profileX := lanWidth + gap
	title := m.profileTitle()
	quota := m.profileQuotaLines(profileWidth - 4)
	profileControls := "—"
	if m.snapshot.profile.Source == "subscription" {
		if m.profileUpdating {
			frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			profileControls = s.accent(frames[m.profileFrame%len(frames)] + " " + m.lang.Text("Updating…"))
		} else {
			profileControls = m.controls([]control{{"profile-update", "Update"}}, profileX+2, top+2, profileWidth-4)
		}
	}
	profileLines := []string{s.bold(title), profileControls, quota[0]}
	if m.height >= 27 {
		profileLines = append(profileLines, quota[1:]...)
	}
	memory := m.dashboardMemory(rateWidth - 4)
	service := m.serviceCard(leftWidth+gap, top, rateWidth-4)
	profileHeight := max(len(profileLines), len(memory), len(lanLines), len(service)) + 2
	return joinH(gap,
		s.panelAt(m.lang.Text("LAN IP"), lanLines, lanWidth, profileHeight),
		s.panelAt(m.lang.Text("Profile"), profileLines, profileWidth, profileHeight),
		s.panelAt(m.lang.Text("Service status"), service, rateWidth, profileHeight),
		s.panelAt(m.lang.Text("Memory"), memory, rateWidth, profileHeight))
}

func (m *model) dashboardMemory(w int) []string {
	s := m.style
	app, kernel := m.snapshot.appResidentBytes, m.snapshot.overview.ResidentBytes
	total := app + kernel
	label := func(bytes int64) string {
		if bytes <= 0 {
			return "—"
		}
		return strings.ReplaceAll(bytesLabel(bytes), " ", "")
	}
	bar := s.faint(strings.Repeat("─", w))
	if total > 0 {
		appWidth := int(float64(app)/float64(total)*float64(w) + 0.5)
		if app > 0 && kernel > 0 && w >= 2 {
			appWidth = min(w-1, max(1, appWidth))
		}
		bar = s.fg(s.p.up, strings.Repeat("━", appWidth)) +
			s.fg(s.p.down, strings.Repeat("─", w-appWidth))
	}
	appLabel := s.fg(s.p.up, "●") + " " + m.lang.Text("App") + " " + label(app)
	kernelLabel := s.fg(s.p.down, "●") + " " + m.lang.Text("Core") + " " + label(kernel)
	if width(appLabel)+width(kernelLabel)+2 <= w {
		return []string{s.bold(label(total)), bar, appLabel + "  " + kernelLabel}
	}
	if width(appLabel) > w {
		appLabel = m.lang.Text("App") + label(app)
	}
	if width(kernelLabel) > w {
		kernelLabel = m.lang.Text("Core") + label(kernel)
	}
	return []string{s.bold(label(total)), bar, appLabel, kernelLabel}
}

func (m *model) groupsFor(name string) string {
	var groups []string
	for _, item := range m.snapshot.proxies {
		if len(item.All) == 0 {
			continue
		}
		for _, option := range item.All {
			if option == name {
				groups = append(groups, safe(item.Name))
				break
			}
		}
	}
	if len(groups) == 0 {
		return "—"
	}
	return strings.Join(groups, " · ")
}

func (m *model) boolMark(v bool) string {
	if v {
		return m.style.ok(m.lang.Text("yes"))
	}
	return m.style.faint(m.lang.Text("no"))
}

func (m *model) delayLabel(d int) string {
	if d <= 0 {
		return m.style.faint("—")
	}
	return m.style.fg(msHex(m.style, d), fmt.Sprintf("%d ms", d))
}

func (m *model) nodeDelayStatus(node core.Proxy) string {
	if !m.proxyControllerAvailable() {
		return m.style.dim("—")
	}
	switch result := m.delays[node.Name]; {
	case m.speedNodes[node.Name]:
		return m.style.warn(m.lang.Text("Testing…"))
	case result.TimedOut:
		return m.style.err(m.lang.Text("Timeout"))
	case result.Failed:
		return m.style.err(m.lang.Text("Test failed"))
	default:
		return m.delayLabel(m.proxyDelay(node))
	}
}

func (m *model) nodeDetail(p core.Proxy, w int) []string {
	s := m.style
	if w < 8 {
		w = 8
	}
	alive := s.dim("—")
	if m.proxyControllerAvailable() {
		alive = m.boolMark(p.Alive)
	}
	lines := []string{
		m.kvw(m.lang.Text("Name"), s.accent(safe(p.Name)), w),
		m.kvw(m.lang.Text("Type"), s.fg(s.p.fg, safe(p.Type)), w),
		m.kv(m.lang.Text("Delay"), m.nodeDelayStatus(p)),
		m.kvw(m.lang.Text("Group"), s.fg(s.p.dim, m.groupsFor(p.Name)), w),
		m.kv(m.lang.Text("Alive"), alive),
	}

	if m.proxyControllerAvailable() && len(p.History) > 0 {
		hist := make([]int64, len(p.History))
		for i, h := range p.History {
			hist[i] = int64(h.Delay)
		}
		lines = append(lines, "", s.dim(m.lang.Text("History")), s.sparkline(hist, max(8, w-2), s.p.acc))
	}
	return lines
}

func (m *model) listEmpty(label string, err error) string {
	if err != nil {
		return m.style.err(m.lang.Error(err))
	}
	return m.emptyMessage(m.lang.Text(label))
}

func (m *model) emptyMessage(none string) string {
	s := m.style
	switch m.snapshot.status.State {
	case "Running":
	case "NotInstalled":
		return s.faint(m.lang.Text("Install Mihomo to load entries"))
	case "Starting", "Stopping", "Degraded":
		return s.faint(m.lang.Text("Controller is not ready; retrying"))
	case "":
		if !m.loaded {
			return s.faint(m.lang.Text("Loading"))
		}
		return s.faint(none)
	default:
		return s.faint(m.lang.Text("Start Mihomo to load entries"))
	}
	if !m.loaded {
		return s.faint(m.lang.Text("Loading"))
	}
	if m.filter != "" {
		return s.faint(m.lang.Text("No matching entries"))
	}
	return s.faint(none)
}

// listPanel renders a list into a fixed-height panel: header lines first, then
// the cursor viewport, then blank padding. It never emits more rows than fit.
func (m *model) listPanel(title string, header, entries []string, cursor, height, x, w int, empty string, kind hitKind) []string {
	inner := max(0, height-2)
	if len(header) > inner {
		header = header[:inner]
	}
	available := inner - len(header)
	body := append([]string(nil), header...)
	if len(entries) == 0 {
		if available > 0 {
			body = append(body, empty)
		}
	} else {
		cursor = min(cursor, len(entries)-1)
		first := max(0, cursor-available+1)
		for i := first; i < min(len(entries), first+available); i++ {
			y := 4 + len(body)
			m.hits = append(m.hits, hit{kind: kind, x0: x + 1, x1: x + w - 1, y0: y, y1: y + 1, index: i})
			line := "  " + entries[i]
			if i == cursor {
				line = m.style.accent("› ") + entries[i]
			}
			body = append(body, line)
		}
	}
	for len(body) < inner {
		body = append(body, "")
	}
	if len(body) > inner {
		body = body[:inner]
	}
	return m.style.panel(title, body, w)
}

func (m *model) profileTitle() string {
	profile := m.snapshot.profile
	if profile.Name != "" {
		return safe(profile.Name)
	}
	if profile.Source == "" {
		return m.lang.Text("Built-in DIRECT")
	}
	return m.lang.Text(profile.Source)
}

func (m *model) profileQuotaLines(w int) []string {
	s := m.style
	quota := m.snapshot.profile.Quota
	if quota == nil {
		return []string{s.dim(m.lang.Text("Quota unavailable"))}
	}
	used := quota.Upload + min(quota.Download, int64(1<<63-1)-quota.Upload)
	total := "—"
	if quota.Total > 0 {
		total = bytesLabel(quota.Total)
	}
	lines := []string{s.fg(s.p.info, bytesLabel(used)+" / "+total)}
	if quota.Total > 0 {
		ratio := float64(used) / float64(quota.Total)
		bars := min(20, max(0, w-12))
		filled := int(min(1, ratio) * float64(bars))
		lines = append(lines, fmt.Sprintf("%.0f%% ", ratio*100)+s.accent(strings.Repeat("━", filled))+s.faint(strings.Repeat("─", bars-filled)))
	}
	expiry := m.lang.Text("Expiry unavailable")
	if quota.Expire > 0 {
		at := time.Unix(quota.Expire, 0)
		days := max(0, int(math.Ceil(time.Until(at).Hours()/24)))
		expiry = fmt.Sprintf(m.lang.Text("%s · %d days left"), at.Local().Format(time.DateOnly), days)
		if !at.After(time.Now()) {
			expiry = at.Local().Format(time.DateOnly) + " · " + m.lang.Text("Expired")
		}
	}
	return append(lines, s.dim(expiry))
}

func (m *model) profilesPage(height int) []string {
	s := m.style
	toolbar := m.controls([]control{{"a", "Add link"}, {"i", "Add file"}, {"u", "Update all profiles"}}, 0, 3, m.width)
	if m.profileUpdating && !m.cardUpdating {
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		toolbar = s.accent(frames[m.profileFrame%len(frames)] + " " + m.lang.Text("Updating…"))
	}
	lines := []string{toolbar, ""}
	if m.snapshot.profileErr != nil {
		lines[1] = s.err(m.lang.Error(m.snapshot.profileErr))
	}
	items := m.profileCards()
	if len(items) == 0 {
		return append(lines, s.dim(m.lang.Text("No active profile")))
	}
	cols := min(2, max(1, (m.width+2)/34))
	cardWidth := (m.width - 2*(cols-1)) / cols
	const cardHeight = 6
	rows := max(1, (height-len(lines))/cardHeight)
	first := max(0, min(m.cursor, len(items)-1)/cols-rows+1) * cols
	for offset := first; offset < min(len(items), first+rows*cols); offset += cols {
		var cards [][]string
		for i := offset; i < min(len(items), offset+cols); i++ {
			item := items[i]
			card := *m
			card.snapshot.profile = item
			title, color := card.profileTitle(), s.p.bd2
			if item.Active {
				title, color = "✓ "+title, s.p.ok
			}
			if i == m.cursor {
				title, color = "› "+title, s.p.acc
			}
			x, y := (i-offset)*(cardWidth+2), 3+len(lines)
			data := card.profileQuotaLines(cardWidth - 4)
			data = append(data[:min(3, len(data))], make([]string, max(0, 3-len(data)))...)
			data = append(data, s.dim(m.lang.Text("Updated")+" "+item.Updated.Local().Format("01-02 15:04")))
			actions := []control{{"e", "Edit"}, {"d", "Delete"}}
			if item.Source == "subscription" {
				actions = append(actions, control{"s", "Update"})
			}
			if cardWidth < 36 && m.lang.Text("Edit") == "Edit" {
				actions[1].label = "Del"
				if len(actions) == 3 {
					actions[2].label = "Sync"
				}
			}
			start := len(m.hits)
			buttons := m.controls(actions, x+2, y+4, cardWidth-4)
			for index := start; index < len(m.hits); index++ {
				m.hits[index].index = i
			}
			if m.cardUpdating && m.updatingProfileID == item.ID {
				data[3] = s.accent(m.lang.Text("Updating…"))
			} else {
				data[3] = buttons + " " + data[3]
			}
			cards = append(cards, s.panelColor(title, data, cardWidth, color))
			m.hits = append(m.hits, hit{kind: hitRow, index: i, x0: x, x1: x + cardWidth, y0: y, y1: y + cardHeight})
		}
		lines = append(lines, joinH(2, cards...)...)
	}
	return lines
}

func levelIndex(t string) int {
	for i, lv := range levels {
		if strings.EqualFold(t, lv) {
			return i
		}
	}
	return 1
}

func levelHex(s styler, lv string) string {
	switch lv {
	case "debug":
		return s.p.faint
	case "info":
		return s.p.info
	case "warning":
		return s.p.warn
	case "error":
		return s.p.err
	default:
		return s.p.dim
	}
}

func (m *model) logLine(e core.LogEntry) string {
	s := m.style
	stamp := "—"
	if !e.Time.IsZero() {
		stamp = e.Time.Format("15:04:05")
	}
	lv := levels[levelIndex(e.Type)]
	label := "[" + fitPlain(strings.ToUpper(m.lang.Text(lv)), 7) + "] "
	hex := s.p.fg
	switch lv {
	case "debug":
		hex = s.p.dim
	case "warning":
		hex = s.p.warn
	case "error":
		hex = s.p.err
	}
	payloadW := max(8, m.width-4-width(stamp+" ")-width(label))
	return s.faint(stamp+" ") + s.fg(levelHex(s, lv), label) + s.fg(hex, fitPlain(safe(e.Payload), payloadW))
}

func (m *model) logsPage(height int) []string {
	s := m.style
	var rows []string
	for _, e := range m.logEntries {
		if levelIndex(e.Type) >= m.logLevel {
			rows = append(rows, m.logLine(e))
		}
	}
	inner := max(1, height-2)
	filtered := len(rows)
	end := len(rows) - min(m.logOffset, max(0, len(rows)-inner))
	rows = rows[max(0, end-inner):end]
	// Keep a tail view: the newest line stays on the bottom row.
	if pad := inner - len(rows); pad > 0 {
		rows = append(make([]string, pad), rows...)
	}
	if filtered == 0 {
		rows[len(rows)-1] = s.faint(m.lang.Text("Waiting for log events"))
	}
	title := m.lang.Text("Mihomo logs")
	if m.logOffset > 0 {
		title += " · " + m.lang.Text("History")
	}
	if m.logPaused {
		title += " · " + m.lang.Text("paused")
	} else if m.logsLive {
		title += " · " + m.lang.Text("live")
	} else {
		title += " · " + m.lang.Text("waiting")
	}
	return s.panel(title, rows, m.width)
}

func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || path == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	return path
}

func (m *model) corePageView(height int) []string {
	s := m.style
	st := m.snapshot.status
	state := m.statusState()
	checksum := s.faint(m.lang.Text("not installed"))
	if st.Installed != "" {
		checksum = s.ok(m.lang.Text("verified · pinned"))
	}
	info := []string{
		m.kv(m.lang.Text("State"), s.fg(stateHex(s, st.State), m.lang.Text(state))),
		m.kv(m.lang.Text("Version"), orDash(st.Installed)),
		m.kv("PID", pidStr(st.PID)),
		m.kv(m.lang.Text("Controller"), orDash(st.Controller)),
		m.kv(m.lang.Text("Binary"), s.fg(s.p.dim, shortenHome(orDash(m.manager.BinaryPath())))),
		m.kv("SHA-256", checksum),
	}
	if m.snapshot.err != nil {
		info = append(info, "", m.kv(m.lang.Text("Error"), s.err(m.lang.Error(m.snapshot.err))))
	}
	actionY, actionX, actionW := 4+len(info)+2, 0, m.width
	if m.width >= 88 {
		actionX = m.width*55/100 + 2
		actionW = m.width - actionX
		actionY = 4
	}
	action := func(key, label, desc string) string {
		m.hits = append(m.hits, hit{kind: hitAction, key: key, x0: actionX + 1, x1: actionX + actionW - 1, y0: actionY, y1: actionY + 1})
		actionY++
		displayKey := key
		if key == "ctrl+r" {
			displayKey = "Ctrl+R"
		}
		return s.accent(fit(displayKey, 7)) + s.fg(s.p.fg, fit(label, 20)) + s.dim(desc)
	}
	actions := []string{
		action("i", m.lang.Text("Install / reinstall"), m.lang.Text("Pinned · SHA-256 verified")),
		action("a", m.lang.Text("Start"), m.lang.Text("Own process · survives TUI exit")),
		action("x", m.lang.Text("Stop"), m.lang.Text("Graceful shutdown")),
		action("ctrl+r", m.lang.Text("Restart"), m.lang.Text("Stop, then start again")),
	}
	if m.width >= 88 {
		lw := m.width * 55 / 100
		rw := m.width - lw - 2
		return joinH(2,
			s.panel(m.lang.Text("Kernel"), info, lw),
			s.panel(m.lang.Text("Actions"), actions, rw),
		)
	}
	var out []string
	out = append(out, s.panel(m.lang.Text("Kernel"), info, m.width)...)
	out = append(out, s.panel(m.lang.Text("Actions"), actions, m.width)...)
	return out
}

func connectionHost(c core.Connection) string {
	host := c.Metadata.Host
	if host == "" {
		host = c.Metadata.DestinationIP
	}
	if c.Metadata.DestinationPort != "" {
		host += ":" + c.Metadata.DestinationPort
	}
	return orDash(host)
}

func connectionColumns(w int) (host, chain int) {
	chain = min(20, max(8, w-35))
	return max(14, w-21-chain), chain
}

func (m *model) connectionRow(c core.Connection, w int) string {
	s := m.style
	chains := "—"
	if len(c.Chains) > 0 {
		chains = strings.Join(c.Chains, " › ")
	}
	hostW, chainW := connectionColumns(w)
	return s.fitFg(s.p.fg, safe(connectionHost(c)), hostW) +
		s.dim(" "+fitPlain(safe(chains), chainW)) +
		s.fg(s.p.down, " ↓"+padLeft(bytesLabel(c.Download), 8)) +
		s.fg(s.p.up, " ↑"+padLeft(bytesLabel(c.Upload), 8))
}

func (m *model) connectionDetail(c core.Connection, w int) []string {
	s := m.style
	chains := "—"
	if len(c.Chains) > 0 {
		chains = strings.Join(c.Chains, " › ")
	}
	rule := c.Rule
	if c.RulePayload != "" {
		rule += " · " + c.RulePayload
	}
	return []string{
		m.kvw(m.lang.Text("Process"), safe(requestProcess(c)), w),
		m.kvw(m.lang.Text("Host"), s.accent(safe(connectionHost(c))), w),
		m.kv(m.lang.Text("Protocol"), strings.ToUpper(safe(orDash(c.Metadata.Network)))),
		m.kvw(m.lang.Text("Chain"), safe(orDash(chains)), w),
		m.kv(m.lang.Text("Download"), s.fg(s.p.down, bytesLabel(c.Download))),
		m.kv(m.lang.Text("Upload"), s.fg(s.p.up, bytesLabel(c.Upload))),
		m.kvw(m.lang.Text("Rule"), safe(orDash(rule)), w),
	}
}

func bytesLabel(bytes int64) string {
	value := float64(bytes)
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	i := 0
	for value >= 1024 && i < len(units)-1 {
		value /= 1024
		i++
	}
	if value >= 100 && i > 0 {
		return fmt.Sprintf("%.0f %s", value, units[i])
	}
	return fmt.Sprintf("%.1f %s", value, units[i])
}

func safe(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
}

package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/i18n"
)

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	return m.handleKey(msg.String(), msg.Key().Text)
}

func (m *model) modalOpen() bool {
	return m.confirm != nil || m.form != "" || m.services.dialog || m.help || m.detail ||
		(m.page == connectionsPage && m.requests.menu != "")
}

func (m *model) handleKey(key, value string) tea.Cmd {
	quit := m.settings.key("quit")
	exclusions := m.page == networkPage && m.showProxyExclusions
	if key == quit && (m.modalOpen() || exclusions) {
		key = "esc"
	}
	if key == "ctrl+c" || key == quit {
		if m.busy {
			m.feedback = m.lang.Text("Wait for the current operation to finish")
			return nil
		}
		if m.cancelTests != nil {
			m.cancelTests()
		}
		return tea.Quit
	}
	if m.page == connectionsPage && m.requests.menu != "" {
		m.requestMenuKey(key)
		return nil
	}
	if m.confirm != nil {
		switch key {
		case "y", "enter":
			run, label := m.confirm, m.confirmLabel
			if label == m.lang.Text("Stop Mihomo") || label == m.lang.Text("Pause proxy") {
				m.lifecycle = "Stopping"
			}
			m.confirm = nil
			return m.act(label, run)
		case "n", "esc":
			m.confirm = nil
			m.retryForm, m.retryInput, m.retryName = "", nil, ""
		}
		return nil
	}
	if m.form != "" {
		if key == "alt+"+quit && len([]rune(quit)) == 1 {
			m.addInput(quit)
			return nil
		}
		return m.formKey(key, value)
	}
	if m.services.dialog {
		switch key {
		case "esc", "e":
			m.services.dialog = false
		case "up", "k":
			m.services.cursor = max(0, m.services.cursor-1)
		case "down", "j":
			m.services.cursor = min(len(serviceTargets)-1, m.services.cursor+1)
		case "home":
			m.services.cursor = 0
		case "end":
			m.services.cursor = len(serviceTargets) - 1
		case "f":
			return m.checkServices(-1)
		case "enter":
			return m.checkServices(m.services.cursor)
		}
		if index, err := strconv.Atoi(strings.TrimPrefix(key, "service-")); strings.HasPrefix(key, "service-") && err == nil && index >= 0 && index < len(serviceTargets) {
			m.services.cursor = index
			return m.checkServices(index)
		}
		return nil
	}
	if m.help {
		m.help = false
		return nil
	}
	if m.detail {
		if key == "esc" || key == "h" || key == "enter" || (key == "x" && m.page == connectionsPage) {
			m.detail = false
			if key == "x" && m.page == connectionsPage {
				return m.closeConnection()
			}
		}
		return nil
	}
	if key == "esc" && exclusions {
		return m.proxyExclusionsKey(key)
	}
	if m.page == proxies {
		switch key {
		case "left", "right", "h", "l":
			delta := 1
			if key == "left" || key == "h" {
				delta = -1
			}
			m.scroll(delta)
			return nil
		case "[", "]":
			tabs := m.proxyTabs()
			i := m.proxyTabIndex(tabs)
			if key == "[" {
				i = (i + len(tabs) - 1) % len(tabs)
			} else {
				i = (i + 1) % len(tabs)
			}
			m.setProxyTab(tabs[i].Name)
			return nil
		}
	}
	wasG := m.pendingG
	m.pendingG = false
	const maxInt = int(^uint(0) >> 2)
	switch key {
	case m.settings.key("help"):
		m.help = true
		return nil
	case "tab", "shift+tab":
		i := menuIndex(m.page)
		delta := 1
		if key == "shift+tab" {
			delta = -1
		}
		m.changePage(menuSections[(i+delta+len(menuSections))%len(menuSections)].pages[0])
		return nil
	case "[", "]":
		tabs := menuSections[menuIndex(m.page)].pages
		if len(tabs) > 1 {
			delta := 1
			if key == "[" {
				delta = -1
			}
			m.changePage(tabs[(slices.Index(tabs, m.page)+delta+len(tabs))%len(tabs)])
		}
		return nil
	case "1", "2", "3", "4", "5":
		m.changePage(menuSections[key[0]-'1'].pages[0])
		return nil
	case m.settings.key("refresh"):
		if !m.refreshing {
			m.refreshing = true
			return m.refresh()
		}
	case "up", "k", "down", "j":
		delta := 1
		if key == "up" || key == "k" {
			delta = -1
		}
		if m.page == proxies {
			delta *= proxyGridColumns(m.width - m.sidebarWidth())
		}
		m.scroll(delta)
	case "pgup", "ctrl+b":
		m.scroll(-max(1, m.height-9))
	case "pgdown", "ctrl+f":
		m.scroll(max(1, m.height-9))
	case "ctrl+u":
		m.scroll(-max(1, (m.height-9)/2))
	case "ctrl+d":
		m.scroll(max(1, (m.height-9)/2))
	case "home":
		m.scroll(-maxInt)
	case "g":
		if wasG {
			m.scroll(-maxInt)
		}
		m.pendingG = !wasG
	case "end":
		m.scroll(maxInt)
	case "esc":
		if m.page == connectionsPage && len(m.connectionSelection) > 0 {
			m.connectionSelection = nil
			return nil
		}
		if m.filter != "" {
			m.filter, m.cursor = "", 0
		}
	case m.settings.key("search"):
		switch m.page {
		case connectionsPage:
			m.filter, m.cursor = "", 0
			m.connectionSelection = nil
			m.openForm("Filter", "")
		case proxies, dnsPage:
			m.openForm("Filter", m.filter)
		}
	case m.settings.key("sort"):
		if m.page == proxies {
			m.sortByDelay = !m.sortByDelay
			m.cursor = 0
		}
	}
	if m.busy {
		return nil
	}
	if m.page == dashboard {
		switch key {
		case "f":
			return m.checkServices(-1)
		case "e":
			m.services.dialog, m.services.cursor = true, 0
			return nil
		}
		if key == "p" {
			return m.toggleSystemProxy()
		}
		switch key {
		case "b", "m", "d", "u", "w":
			if m.snapshot.status.State != "Running" {
				m.feedback = m.lang.Text("Start Mihomo to apply changes")
				return nil
			}
		}
		switch key {
		case "b", "m", "d":
			mode := map[string]string{"b": "rule", "m": "global", "d": "direct"}[key]
			return m.act(m.lang.Text("Routing mode")+" · "+m.lang.Text(mode), func() error { return m.manager.SetMode(m.ctx, mode) })
		case "w":
			if m.snapshot.overview.LAN {
				return m.act(m.lang.Text("LAN proxy off"), func() error { return m.manager.SetLAN(m.ctx, false) })
			}
			m.confirmLabel = m.lang.Text("Allow devices on local subnets to use this proxy")
			m.confirm = func() error { return m.manager.SetLAN(m.ctx, true) }
			return nil
		case "u":
			return m.toggleTUN()
		}
	}
	switch m.page {
	case dashboard, corePage:
		switch key {
		case "i":
			return m.act(m.lang.Text("Install Mihomo"), func() error { return m.manager.Install(m.ctx) })
		case "a":
			if m.page == dashboard && m.kernelActive() {
				m.confirmLabel = m.lang.Text("Pause proxy")
				m.confirm = func() error { return m.manager.PauseProxy(m.ctx) }
				return nil
			}
			m.lifecycle = "Starting"
			start := m.manager.Start
			if m.page == dashboard {
				start = m.manager.StartDashboardProxy
			}
			return m.act(m.lang.Text("Start Mihomo"), func() error { return start(m.ctx) })
		case "x":
			if m.page == corePage {
				m.confirmLabel, m.confirm = m.lang.Text("Stop Mihomo"), m.manager.Stop
			}
		case "ctrl+r":
			m.confirmLabel, m.confirm = m.lang.Text("Restart Mihomo"), func() error { return m.manager.Restart(m.ctx) }
		}
	case languagePage:
		if key == "e" || key == "c" {
			lang := i18n.English
			if key == "c" {
				lang = i18n.Chinese
			}
			err := m.manager.Paths.Ensure()
			if err == nil {
				err = i18n.Save(m.manager.Paths.Config, lang)
			}
			if err != nil {
				m.feedback = m.lang.Error(err)
				return nil
			}
			m.lang = lang
			m.feedback = m.lang.Text("Language saved")
		}
	case profiles:
		switch key {
		case "e", "d", "s":
			return m.profileCardAction(key)
		case "a":
			m.openForm("Subscription URL", "")
		case "i":
			m.openForm("Local YAML path", "")
		case "u":
			return m.updateAllProfiles(false)
		case "enter":
			return m.activateProfileCard()
		}
	case dnsPage:
		return m.dnsPageKey(key)
	case generalPage:
		return m.toolPageKey(key)
	case networkPage:
		return m.networkPageKey(key)
	case connectionsPage:
		switch key {
		case "c":
			m.openRequestMenu("Category")
		case "ctrl+p":
			m.openRequestMenu("Protocol")
		case "ctrl+t":
			m.openRequestMenu("Rule")
		case "left", "h":
			m.requests.column = max(0, m.requests.column-1)
		case "right", "l":
			layout := *m
			layout.width -= m.sidebarWidth()
			first, last, _, _ := layout.requestTableLayout()
			if last < len(requestColumns) {
				m.requests.column = first + 1
			}
		case "d":
			m.confirmCloseAllRequests()
		}
		if key == "ctrl+a" {
			m.connectionSelection = make(map[string]bool)
			for _, item := range m.visibleConnections() {
				m.connectionSelection[item.ID] = true
			}
			return nil
		}
		if key == "enter" && len(m.visibleConnections()) > 0 {
			m.detail = true
		}
		if key == "x" {
			return m.closeConnection()
		}
	case proxies:
		if key == "f" {
			return m.testAllNodes()
		}
		if key == "enter" {
			return m.selectProxyCard()
		}
		if key == "t" {
			return m.testProxy()
		}
		if key == "e" && len(m.visibleProxies()) > 0 {
			m.detail = true
			return nil
		}
	case logs:
		if key == m.settings.key("log-level") {
			m.logLevel = (m.logLevel + 1) % len(levels)
			m.logOffset = 0
		}
		if key == "p" {
			m.logPaused = !m.logPaused
		}
		if key == "c" {
			m.logEntries, m.logOffset = nil, 0
		}
	}
	return nil
}

func (m *model) closeConnection() tea.Cmd {
	items := m.visibleConnections()
	if m.busy || len(items) == 0 {
		return nil
	}
	var ids []string
	for _, item := range items {
		if m.connectionSelection[item.ID] {
			ids = append(ids, item.ID)
		}
	}
	if len(ids) > 0 {
		m.confirmLabel = fmt.Sprintf(m.lang.Text("Close %d selected connections"), len(ids))
		m.confirm = func() error { return m.manager.CloseConnections(m.ctx, ids) }
		return nil
	}
	item := items[min(m.cursor, len(items)-1)]
	m.confirmLabel = fmt.Sprintf(m.lang.Text("Close connection %s"), safe(connectionHost(item)))
	m.confirm = func() error { return m.manager.CloseConnection(m.ctx, item.ID) }
	return nil
}

func (m *model) selectProxyCard() tea.Cmd {
	if !m.proxyControllerAvailable() {
		m.feedback = m.lang.Text("Start Mihomo to apply changes")
		return nil
	}
	items := m.visibleProxies()
	tabs := m.proxyTabs()
	group := tabs[m.proxyTabIndex(tabs)]
	if group.Name == "" || len(items) == 0 {
		return nil
	}
	if group.Type != "Selector" {
		m.feedback = m.lang.Text("This group chooses its option automatically")
		return nil
	}
	node := items[min(m.cursor, len(items)-1)].Name
	if node == group.Now {
		return nil
	}
	return m.act(fmt.Sprintf(m.lang.Text("Select %s"), safe(node)), func() error {
		return m.manager.Select(m.ctx, group.Name, node)
	})
}

func (m *model) testProxy() tea.Cmd {
	if m.speedNodes != nil {
		return nil
	}
	if !m.proxyControllerAvailable() {
		m.feedback = m.lang.Text("Start Mihomo to apply changes")
		return nil
	}
	items := m.visibleProxies()
	if len(items) == 0 {
		return nil
	}
	name := items[min(m.cursor, len(items)-1)].Name
	generation := m.beginSpeedTest([]string{name})
	m.feedback = ""
	return func() tea.Msg {
		delay, err := m.manager.Delay(m.testContext(), name, "")
		if err != nil {
			return speedDone{generation: generation, label: safe(name), err: err, delays: map[string]core.DelayResult{name: core.NewDelayResult(delay, err)}}
		}
		return speedDone{generation: generation, label: safe(name), detail: fmt.Sprintf("%d ms", delay), delays: map[string]core.DelayResult{name: core.NewDelayResult(delay, nil)}}
	}
}

func (m *model) beginSpeedTest(names []string) uint64 {
	m.speedGeneration++
	m.speedNodes = make(map[string]bool, len(names))
	for _, name := range names {
		m.speedNodes[name] = true
	}
	return m.speedGeneration
}

func (m *model) testAllNodes() tea.Cmd {
	if m.speedNodes != nil {
		return nil
	}
	if !m.proxyControllerAvailable() {
		m.feedback = m.lang.Text("Start Mihomo to apply changes")
		return nil
	}
	if m.proxyDirectView() {
		return nil
	}
	var names []string
	for _, item := range m.snapshot.proxies {
		if !item.IsGroup() && !isVirtualProxy(item) && item.Type != "Direct" && item.Type != "Reject" {
			names = append(names, item.Name)
		}
	}
	if len(names) == 0 {
		m.feedback = m.lang.Text("No proxy nodes to test")
		return nil
	}
	label := m.lang.Text("Test all nodes")
	generation := m.beginSpeedTest(names)
	m.feedback = ""
	send, manager, ctx := m.send, m.manager, m.testContext()
	return func() tea.Msg {
		results, err := manager.TestNodes(ctx, names, "", func(name string, result *core.DelayResult) {
			if send != nil {
				send(speedProgress{generation, name, result})
			}
		})
		if err != nil {
			return speedDone{generation: generation, label: label, err: err}
		}
		failed := 0
		for _, result := range results {
			if result.Failed {
				failed++
			}
		}
		return speedDone{generation: generation, label: label, delays: results, detail: fmt.Sprintf(m.lang.Text("%d reachable · %d failed"), len(results)-failed, failed)}
	}
}

func (m *model) changePage(next page) {
	m.page, m.cursor, m.filter = next, 0, ""
	m.showProxyExclusions = false
	m.connectionSelection = nil
	m.requests.menu = ""
	m.pendingG, m.detail = false, false
	if next == proxies {
		m.setProxyTab(m.proxyTabs()[0].Name)
	}
}

func (m *model) addInput(value string) {
	var runes []rune
	for _, r := range value {
		if !unicode.IsControl(r) {
			runes = append(runes, r)
		}
	}
	if len(runes) == 0 {
		return
	}
	if room := 8192 - len(m.input); len(runes) > room {
		runes = runes[:max(0, room)]
	}
	if len(runes) == 0 {
		return
	}
	m.recordInputEdit()
	m.inputCursor = min(max(0, m.inputCursor), len(m.input))
	next := make([]rune, 0, len(m.input)+len(runes))
	next = append(next, m.input[:m.inputCursor]...)
	next = append(next, runes...)
	next = append(next, m.input[m.inputCursor:]...)
	m.input = next
	m.inputCursor += len(runes)
}

func (m *model) openForm(name, value string) {
	m.form = name
	m.formID++
	m.formPage, m.formField = m.page, core.ToolField{}
	if (m.page == generalPage || m.page == networkPage) && name == m.toolField.Label {
		m.formField = m.toolField
	}
	m.input = []rune(value)
	m.inputCursor = len(m.input)
	m.formError = ""
	m.profileName, m.profileLink, m.nameFocused = "", "", false
	m.profileSchedule, m.profileInterval, m.intervalFocused = core.ProfileSchedule{}, "", false
	m.formHistories = [3]formHistory{}
}

func (m *model) closeForm() {
	m.form, m.input, m.inputCursor, m.formError = "", nil, 0, ""
	m.profileName, m.profileLink, m.nameFocused = "", "", false
	m.profileSchedule, m.profileInterval, m.intervalFocused = core.ProfileSchedule{}, "", false
	m.formHistories = [3]formHistory{}
}

func (m *model) mouse(msg tea.MouseMsg) tea.Cmd {
	switch ev := msg.(type) {
	case tea.MouseWheelMsg:
		if m.services.dialog {
			switch ev.Button {
			case tea.MouseWheelUp:
				return m.handleKey("up", "")
			case tea.MouseWheelDown:
				return m.handleKey("down", "")
			}
			return nil
		}
		if m.help || m.confirm != nil || m.form != "" || m.detail || m.requests.menu != "" || m.services.dialog || m.width < m.minimumColumns() || m.height < minRows {
			return nil
		}
		if ev.X < m.sidebarWidth() || ev.Y < 3 || ev.Y >= m.height-1 {
			return nil
		}
		delta := 1
		switch ev.Button {
		case tea.MouseWheelUp:
			delta = -1
		case tea.MouseWheelDown:
		default:
			return nil
		}
		if m.page == proxies {
			delta *= proxyGridColumns(m.width - m.sidebarWidth())
		}
		m.scroll(delta)
	case tea.MouseClickMsg:
		m.pendingG = false
		if ev.Button != tea.MouseLeft {
			return nil
		}
		for _, h := range m.hits {
			if ev.X >= h.x0 && ev.X < h.x1 && ev.Y >= h.y0 && ev.Y < h.y1 {
				if h.kind == hitInput && m.form != "" {
					m.placeCaret(ev.X-h.x0, h.index)
					return nil
				}
				return m.activate(h)
			}
		}
	}
	return nil
}

func (m *model) scroll(delta int) {
	if m.page == logs {
		m.logOffset = min(m.maxLogOffset(), max(0, m.logOffset-delta))
		return
	}
	var n int
	switch m.page {
	case profiles:
		n = len(m.snapshot.profiles)
	case proxies:
		n = len(m.visibleProxies())
	case dnsPage:
		n = len(m.visibleDNSRows())
	case generalPage:
		n = len(m.generalToolFields())
	case networkPage:
		n = networkRowCount
		if m.showProxyExclusions {
			n = len(m.snapshot.systemProxy.Exclusions)
		}
	case connectionsPage:
		n = len(m.visibleConnections())
	default:
		return
	}
	m.cursor = min(max(0, n-1), max(0, m.cursor+delta))
}

func (m *model) updateProfile() tea.Cmd {
	if m.busy {
		return nil
	}
	if m.snapshot.profile.Source != "subscription" {
		m.feedback = m.lang.Text("no subscription is configured")
		return nil
	}
	m.profileUpdating, m.profileFrame = true, 0
	m.profileSpinID++
	return tea.Batch(
		m.act(m.lang.Text("Update profile"), func() error { return m.manager.UpdateProfile(m.ctx) }),
		profileSpinTick(m.profileSpinID),
	)
}

func (m *model) profileCards() []core.ProfileInfo {
	if len(m.snapshot.profiles) > 0 {
		return m.snapshot.profiles
	}
	if m.snapshot.profile.Source != "" {
		item := m.snapshot.profile
		item.Active = true
		return []core.ProfileInfo{item}
	}
	return nil
}

func (m *model) profileCardAction(key string) tea.Cmd {
	items := m.profileCards()
	if m.busy || len(items) == 0 {
		return nil
	}
	item := items[min(m.cursor, len(items)-1)]
	switch key {
	case "e":
		link, err := m.manager.ProfileEditLink(item.ID)
		if err != nil {
			m.feedback = m.lang.Text("Error · ") + m.lang.Error(err)
			return nil
		}
		m.openForm("Edit profile", link)
		m.editProfileID, m.editProfileSource = item.ID, item.Source
		m.profileName, m.profileSchedule = item.Name, item.Schedule
		m.profileInterval = strconv.Itoa(item.Schedule.Interval())
		m.focusSubscriptionName(true)
	case "d":
		m.confirmLabel = fmt.Sprintf(m.lang.Text("Delete profile %s"), safe(item.Name))
		m.confirm = func() error { return m.manager.DeleteProfile(m.ctx, item.ID) }
	case "s":
		if item.Source != "subscription" {
			return nil
		}
		m.profileUpdating, m.cardUpdating, m.profileFrame = true, true, 0
		m.updatingProfileID = item.ID
		m.profileSpinID++
		return tea.Batch(profileSpinTick(m.profileSpinID),
			m.act(m.lang.Text("Update profile"), func() error { return m.manager.UpdateSavedProfile(m.ctx, item.ID) }))
	}
	return nil
}

func (m *model) activateProfileCard() tea.Cmd {
	items := m.snapshot.profiles
	if m.busy || len(items) == 0 {
		return nil
	}
	item := items[min(m.cursor, len(items)-1)]
	if item.Active {
		return nil
	}
	return m.act(m.lang.Text("Switch profile"), func() error {
		return m.manager.ActivateProfile(m.ctx, item.ID)
	})
}

func (m *model) updateAllProfiles(dueOnly bool) tea.Cmd {
	m.profileUpdating, m.profileFrame, m.busy = true, 0, true
	m.profileSpinID++
	label := m.lang.Text("Update all profiles")
	m.feedback = label + "…"
	return tea.Batch(profileSpinTick(m.profileSpinID), func() tea.Msg {
		updated, failed, err := m.manager.UpdateProfiles(m.ctx, dueOnly)
		return operation{label: label, err: err,
			detail: fmt.Sprintf(m.lang.Text("%d updated · %d failed"), updated, failed)}
	})
}

func (m *model) activate(h hit) tea.Cmd {
	switch h.kind {
	case hitAction:
		if m.page == profiles && m.form == "" && m.confirm == nil && (h.key == "e" || h.key == "d" || h.key == "s") {
			m.cursor = h.index
			return m.profileCardAction(h.key)
		}
		if h.key == "profile-update" {
			return m.updateProfile()
		}
		return m.handleKey(h.key, "")
	case hitTab:
		m.changePage(page(h.index))
		return nil
	case hitDNSFilter:
		m.toggleDNSFilter(h.key)
	case hitRequestFilter:
		m.toggleRequestFilter(h.key)
	case hitRequestOption:
		m.chooseRequestOption(h.index)
	case hitProxyTab:
		tabs := m.proxyTabs()
		if h.index < len(tabs) {
			m.setProxyTab(tabs[h.index].Name)
		}
	case hitProxyCard:
		m.cursor = h.index
	case hitRow:
		m.cursor = h.index
		if m.page == dnsPage {
			return m.dnsPageKey("enter")
		}
		if m.page == generalPage && !m.busy {
			m.editToolField()
		}
		if m.page == networkPage && !m.busy && !m.showProxyExclusions {
			return m.networkPageKey("enter")
		}
		if m.page == profiles {
			return m.activateProfileCard()
		}
		if m.page == connectionsPage {
			m.detail = true
		}
	}
	return nil
}

func (m *model) focusProfileInterval() {
	if m.intervalFocused {
		return
	}
	m.focusSubscriptionName(false)
	m.profileLink = string(m.input)
	m.input = []rune(m.profileInterval)
	m.inputCursor = len(m.input)
	m.intervalFocused = true
}

func (m *model) leaveProfileInterval() {
	if !m.intervalFocused {
		return
	}
	m.profileInterval = string(m.input)
	m.input = []rune(m.profileLink)
	m.inputCursor = len(m.input)
	m.intervalFocused = false
}

func (m *model) focusSubscriptionName(name bool) {
	if name == m.nameFocused {
		return
	}
	if name {
		m.profileLink = string(m.input)
		m.input = []rune(m.profileName)
	} else {
		m.profileName = string(m.input)
		m.input = []rune(m.profileLink)
	}
	m.inputCursor = len(m.input)
	m.nameFocused = name
}

func (m *model) inputHistory() *formHistory {
	index := 0
	if m.nameFocused {
		index = 1
	} else if m.intervalFocused {
		index = 2
	}
	return &m.formHistories[index]
}

func (m *model) inputRevision() formRevision {
	return formRevision{string(m.input), m.inputCursor}
}

func (m *model) recordInputEdit() {
	history := m.inputHistory()
	const limit = 100
	if len(history.undo) == limit {
		copy(history.undo, history.undo[1:])
		history.undo = history.undo[:limit-1]
	}
	history.undo = append(history.undo, m.inputRevision())
	history.redo = nil
}

func (m *model) replayInputEdit(redo bool) {
	history := m.inputHistory()
	from, to := &history.undo, &history.redo
	if redo {
		from, to = to, from
	}
	if len(*from) == 0 {
		return
	}
	index := len(*from) - 1
	revision := (*from)[index]
	(*from)[index] = formRevision{}
	*from = (*from)[:index]
	*to = append(*to, m.inputRevision())
	m.input, m.inputCursor = []rune(revision.text), revision.cursor
}

func (m *model) formKey(key, value string) tea.Cmd {
	switch key {
	case "ctrl+z":
		m.replayInputEdit(false)
	case "ctrl+shift+z", "ctrl+y":
		m.replayInputEdit(true)
	case "tab", "shift+tab":
		if m.form == "Edit profile" && m.editProfileSource == "subscription" {
			switch {
			case m.intervalFocused:
				m.leaveProfileInterval()
				m.focusSubscriptionName(key == "tab")
			case m.nameFocused && key == "tab", !m.nameFocused && key == "shift+tab":
				m.focusSubscriptionName(!m.nameFocused)
			default:
				m.focusProfileInterval()
			}
		} else if m.form == "Subscription URL" || m.form == "Edit profile" || m.form == "Local YAML path" {
			m.focusSubscriptionName(!m.nameFocused)
		}
	case "profile-name", "profile-link":
		m.leaveProfileInterval()
		m.focusSubscriptionName(key == "profile-name")
	case "profile-interval":
		m.focusProfileInterval()
	case "profile-auto", "ctrl+a":
		if m.form == "Edit profile" && m.editProfileSource == "subscription" {
			m.profileSchedule.Disabled = !m.profileSchedule.Disabled
		} else {
			m.inputCursor = 0
		}
	case "esc":
		m.closeForm()
	case "left":
		m.inputCursor = max(0, m.inputCursor-1)
	case "right":
		m.inputCursor = min(len(m.input), m.inputCursor+1)
	case "home":
		m.inputCursor = 0
	case "end", "ctrl+e":
		m.inputCursor = len(m.input)
	case "backspace":
		if m.inputCursor > 0 {
			m.recordInputEdit()
			m.input = append(m.input[:m.inputCursor-1], m.input[m.inputCursor:]...)
			m.inputCursor--
		}
	case "delete":
		if m.inputCursor < len(m.input) {
			m.recordInputEdit()
			m.input = append(m.input[:m.inputCursor], m.input[m.inputCursor+1:]...)
		}
	case "ctrl+u":
		if m.inputCursor == 0 {
			return nil
		}
		m.recordInputEdit()
		m.input = append([]rune(nil), m.input[m.inputCursor:]...)
		m.inputCursor = 0
	case "ctrl+k":
		if m.inputCursor == len(m.input) {
			return nil
		}
		m.recordInputEdit()
		m.input = m.input[:m.inputCursor]
	case "ctrl+w":
		start := m.inputCursor
		for start > 0 && unicode.IsSpace(m.input[start-1]) {
			start--
		}
		for start > 0 && !unicode.IsSpace(m.input[start-1]) {
			start--
		}
		if start != m.inputCursor {
			m.recordInputEdit()
			m.input = append(m.input[:start], m.input[m.inputCursor:]...)
			m.inputCursor = start
		}
	case "ctrl+v":
		formID := m.formID
		return func() tea.Msg {
			text, err := readClipboard()
			return clipboardMsg{formID: formID, text: text, err: err}
		}
	case "enter":
		if m.form == "Add proxy exclusion" || m.form == "Edit proxy exclusion" {
			return m.saveProxyExclusion()
		}
		if m.form == "Edit profile" && m.editProfileSource == "subscription" {
			m.leaveProfileInterval()
			minutes, err := strconv.Atoi(strings.TrimSpace(m.profileInterval))
			if err != nil || minutes < 1 || minutes > 525600 {
				m.formError = m.lang.Text("Update interval must be between 1 and 525600 minutes")
				m.focusProfileInterval()
				return nil
			}
			m.profileSchedule.Minutes = minutes
		}
		schedule := m.profileSchedule
		if m.form == "Subscription URL" {
			m.focusSubscriptionName(false)
			if strings.TrimSpace(string(m.input)) == "" {
				m.formError = m.lang.Text("Subscription URL is required")
				return nil
			}
		}
		if m.form == "Edit profile" || m.form == "Local YAML path" {
			m.focusSubscriptionName(false)
			if m.form == "Local YAML path" && strings.TrimSpace(string(m.input)) == "" {
				m.formError = m.lang.Text("File path (required)")
				return nil
			}
		}
		id, source := m.editProfileID, m.editProfileSource
		form, input, name := m.form, strings.TrimSpace(string(m.input)), strings.TrimSpace(m.profileName)
		m.retryPage, m.retryField = m.formPage, m.formField
		m.closeForm()
		if form == "DNS query" {
			return m.queryDNS(input)
		}
		if field := m.retryField; field.Key != "" {
			m.retryForm, m.retryInput = form, []rune(input)
			return m.act(m.lang.Text("Save setting"), func() error {
				return m.manager.SetToolSetting(m.ctx, field.Key, input)
			})
		}
		switch form {
		case "Edit profile":
			m.retrySchedule = schedule
			m.retryProfileID, m.retryProfileSource = id, source
			m.retryForm, m.retryInput, m.retryName = form, []rune(input), name
			return m.act(m.lang.Text("Edit profile"), func() error {
				return m.manager.EditProfile(m.ctx, id, name, input, schedule)
			})
		case "Filter":
			m.filter, m.cursor = strings.ToLower(input), 0
			m.connectionSelection = nil
		case "Subscription URL", "Local YAML path":
			if input == "" {
				return nil
			}
			m.retryForm, m.retryInput, m.retryName = form, []rune(input), name
			return m.act(m.lang.Text("Import profile"), func() error {
				return m.manager.AddProfile(m.ctx, input, name, form == "Local YAML path")
			})
		}
	default:
		m.addInput(value)
	}
	return nil
}

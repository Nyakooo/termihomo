package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/Nyakooo/termihomo/internal/core"
)

type page int

const (
	dashboard page = iota
	proxies
	groups
	profiles
	traffic
	logs
	corePage
)

var pages = []string{"Overview", "Proxies", "Groups", "Profile", "Traffic", "Logs", "Core"}
var levels = []string{"debug", "info", "warning", "error"}

type snapshot struct {
	status   core.Status
	profile  core.ProfileInfo
	overview core.Overview
	proxies  []core.Proxy
	err      error
}

type refreshMsg struct{}
type trafficOffline struct{}
type logsOffline struct{}
type operation struct {
	label  string
	detail string
	err    error
}

type model struct {
	manager       core.Manager
	settings      settings
	ctx           context.Context
	snapshot      snapshot
	page          page
	width, height int
	cursor        int
	choice        int
	groupName     string
	filter        string
	form          string
	input         []rune
	confirm       func() error
	confirmLabel  string
	busy          bool
	refreshing    bool
	help          bool
	sortByDelay   bool
	feedback      string
	rates         []core.Traffic
	trafficLive   bool
	logEntries    []core.LogEntry
	logsLive      bool
	logLevel      int
}

func Run(manager core.Manager, input io.Reader, output io.Writer) error {
	settings, err := loadSettings(manager.Paths.Config)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &model{manager: manager, settings: settings, ctx: ctx, width: 80, height: 24, logLevel: 1, refreshing: true}
	program := tea.NewProgram(m, tea.WithInput(input), tea.WithOutput(output), tea.WithContext(ctx))
	go reconnect(ctx, func() error {
		return manager.StreamTraffic(ctx, func(event core.Traffic) { program.Send(event) })
	}, func() { program.Send(trafficOffline{}) })
	go reconnect(ctx, func() error {
		return manager.StreamLogs(ctx, "debug", func(event core.LogEntry) { program.Send(event) })
	}, func() { program.Send(logsOffline{}) })
	_, err = program.Run()
	return err
}

func reconnect(ctx context.Context, connect func() error, offline func()) {
	for ctx.Err() == nil {
		_ = connect()
		if ctx.Err() != nil {
			return
		}
		offline()
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.refresh(), m.tick())
}

func (m *model) tick() tea.Cmd {
	return tea.Tick(m.settings.interval(), func(time.Time) tea.Msg { return refreshMsg{} })
}

func (m *model) refresh() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 7*time.Second)
		defer cancel()
		var s snapshot
		s.status, s.err = m.manager.Status(ctx)
		if s.err != nil {
			return s
		}
		s.profile, s.err = m.manager.Profile()
		if s.err != nil || s.status.State != "Running" {
			return s
		}
		s.proxies, s.err = m.manager.Proxies(ctx)
		if s.err == nil {
			s.overview, s.err = m.manager.Overview(ctx)
		}
		return s
	}
}

func (m *model) act(label string, run func() error) tea.Cmd {
	m.busy = true
	m.feedback = label + "…"
	return func() tea.Msg { return operation{label: label, err: run()} }
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case refreshMsg:
		if !m.refreshing {
			m.refreshing = true
			return m, tea.Batch(m.refresh(), m.tick())
		}
		return m, m.tick()
	case snapshot:
		m.refreshing = false
		m.snapshot = msg
	case operation:
		m.busy = false
		m.feedback = msg.label + " complete"
		if msg.detail != "" {
			m.feedback = msg.label + " · " + msg.detail
		}
		if msg.err != nil {
			m.feedback = "Error · " + msg.err.Error()
		}
		if !m.refreshing {
			m.refreshing = true
			return m, m.refresh()
		}
	case core.Traffic:
		m.trafficLive = true
		m.rates = append(m.rates, msg)
		m.rates = m.rates[max(0, len(m.rates)-60):]
	case trafficOffline:
		m.trafficLive = false
	case core.LogEntry:
		m.logsLive = true
		m.logEntries = append(m.logEntries, msg)
		m.logEntries = m.logEntries[max(0, len(m.logEntries)-200):]
	case logsOffline:
		m.logsLive = false
	case tea.PasteMsg:
		if m.form != "" {
			m.addInput(msg.Content)
		}
	case tea.KeyPressMsg:
		return m, m.key(msg)
	}
	return m, nil
}

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	if key == "ctrl+c" || (key == m.settings.key("quit") && m.form == "" && m.confirm == nil && !m.help) {
		if m.busy {
			m.feedback = "Wait for the current operation to finish"
			return nil
		}
		return tea.Quit
	}
	if m.confirm != nil {
		switch key {
		case "y":
			run, label := m.confirm, m.confirmLabel
			m.confirm = nil
			return m.act(label, run)
		case "n", "esc":
			m.confirm = nil
		}
		return nil
	}
	if m.form != "" {
		return m.formKey(key, msg.Key().Text)
	}
	if m.help {
		m.help = false
		return nil
	}
	switch key {
	case m.settings.key("help"):
		m.help = true
		return nil
	case "tab", "right":
		m.changePage((m.page + 1) % page(len(pages)))
		return nil
	case "shift+tab", "left":
		m.changePage((m.page + page(len(pages)) - 1) % page(len(pages)))
		return nil
	case "1", "2", "3", "4", "5", "6", "7":
		m.changePage(page(key[0] - '1'))
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
		if m.groupName != "" {
			for _, item := range m.visibleProxies() {
				if item.Name == m.groupName {
					m.choice = min(max(0, len(item.All)-1), max(0, m.choice+delta))
					break
				}
			}
		} else {
			m.cursor = min(max(0, len(m.visibleProxies())-1), max(0, m.cursor+delta))
		}
	case "esc":
		m.groupName, m.filter = "", ""
	case m.settings.key("search"):
		if m.page == proxies || m.page == groups {
			m.form, m.input = "Filter", []rune(m.filter)
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
	switch m.page {
	case corePage:
		switch key {
		case "i":
			return m.act("Install Mihomo", func() error { return m.manager.Install(m.ctx) })
		case "a":
			return m.act("Start Mihomo", func() error { return m.manager.Start(m.ctx) })
		case "x":
			m.confirmLabel, m.confirm = "Stop Mihomo", m.manager.Stop
		case "R":
			m.confirmLabel, m.confirm = "Restart Mihomo", func() error { return m.manager.Restart(m.ctx) }
		}
	case profiles:
		switch key {
		case "a":
			m.form, m.input = "Subscription URL", nil
		case "i":
			m.form, m.input = "Local YAML path", nil
		case "u":
			m.confirmLabel, m.confirm = "Update profile", func() error { return m.manager.UpdateProfile(m.ctx) }
		}
	case groups:
		if key == "enter" {
			return m.selectGroup()
		}
	case proxies:
		if key == m.settings.key("test") {
			items := m.visibleProxies()
			if len(items) > 0 {
				name := items[min(m.cursor, len(items)-1)].Name
				m.busy = true
				m.feedback = "Testing " + safe(name) + "…"
				return func() tea.Msg {
					delay, err := m.manager.Delay(m.ctx, name, "https://www.gstatic.com/generate_204")
					return operation{label: safe(name), detail: fmt.Sprintf("%d ms", delay), err: err}
				}
			}
		}
	case logs:
		if key == m.settings.key("log-level") {
			m.logLevel = (m.logLevel + 1) % len(levels)
		}
	}
	return nil
}

func (m *model) selectGroup() tea.Cmd {
	items := m.visibleProxies()
	if m.groupName == "" && len(items) > 0 {
		item := items[min(m.cursor, len(items)-1)]
		m.groupName, m.choice = item.Name, 0
		return nil
	}
	for _, group := range items {
		if group.Name == m.groupName && len(group.All) > 0 {
			if group.Type != "Selector" {
				m.feedback = "This group chooses its option automatically"
				return nil
			}
			choice := group.All[min(m.choice, len(group.All)-1)]
			m.groupName = ""
			return m.act("Select "+safe(choice), func() error { return m.manager.Select(m.ctx, group.Name, choice) })
		}
	}
	return nil
}

func (m *model) changePage(next page) {
	m.page, m.cursor, m.groupName, m.filter = next, 0, "", ""
}

func (m *model) addInput(value string) {
	for _, r := range value {
		if len(m.input) >= 8192 {
			break
		}
		if !unicode.IsControl(r) {
			m.input = append(m.input, r)
		}
	}
}

func (m *model) formKey(key, value string) tea.Cmd {
	switch key {
	case "esc":
		m.form, m.input = "", nil
	case "backspace":
		m.input = m.input[:max(0, len(m.input)-1)]
	case "enter":
		form, input := m.form, strings.TrimSpace(string(m.input))
		m.form, m.input = "", nil
		switch form {
		case "Filter":
			m.filter, m.cursor = strings.ToLower(input), 0
		case "Subscription URL", "Local YAML path":
			if input == "" {
				return nil
			}
			label := "Import profile"
			run := func() error { return m.manager.ImportProfile(m.ctx, input) }
			if form == "Subscription URL" {
				label = "Add subscription"
				run = func() error { return m.manager.AddSubscription(m.ctx, input) }
			}
			if m.snapshot.profile.Source != "" {
				m.confirmLabel, m.confirm = label+" and replace the active profile", run
				return nil
			}
			return m.act(label, run)
		}
	default:
		m.addInput(value)
	}
	return nil
}

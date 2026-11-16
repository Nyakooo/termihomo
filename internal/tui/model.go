package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/i18n"
)

type page int

const (
	dashboard page = iota
	proxies
	profiles
	logs
	corePage
	connectionsPage
	languagePage
	generalPage
	dnsPage
	networkPage
)

var pages = []string{"Overview", "Proxies", "Profile", "Logs", "Core", "Connections", "Language", "General", "DNS", "Network"}
var levels = []string{"debug", "info", "warning", "error"}

type snapshot struct {
	toolSettings                        map[string]string
	toolSettingsErr                     error
	status                              core.Status
	profile                             core.ProfileInfo
	profiles                            []core.ProfileInfo
	overview                            core.Overview
	lanIPs                              []string
	systemProxy                         core.SystemProxy
	appResidentBytes                    int64
	proxies                             []core.Proxy
	rules                               []core.Rule
	connections                         []core.Connection
	connectionsAt                       time.Time
	err                                 error
	profileErr, proxyErr, connectionErr error
}

type refreshMsg struct{}
type trafficOffline struct{}
type logsOffline struct{}
type clipboardMsg struct {
	formID uint64
	text   string
	err    error
}
type profileSpin uint64

type operation struct {
	label  string
	detail string
	err    error
}

type hitKind int

const (
	hitTab hitKind = iota
	hitRow

	hitAction
	hitInput
	hitProxyTab
	hitProxyCard
	hitRequestFilter
	hitDNSFilter
	hitRequestOption
)

// hit is a clickable region in absolute terminal coordinates, recorded during
// View so the next mouse event can be mapped to what the user actually saw.
type hit struct {
	kind   hitKind
	x0, x1 int
	y0, y1 int
	index  int
	key    string
}

type speedProgress struct {
	generation uint64
	name       string
	result     *core.DelayResult
}

type speedDone struct {
	generation uint64
	label      string
	detail     string
	err        error
	delays     map[string]core.DelayResult
}

type formRevision struct {
	text   string
	cursor int
}

type formHistory struct {
	undo, redo []formRevision
}

type model struct {
	compactTerminal     bool
	dnsFeed             dnsFeed
	toolField           core.ToolField
	showProxyExclusions bool
	exclusionIndex      int
	exclusionOriginal   string
	send                func(tea.Msg)
	speedNodes          map[string]bool
	speedGeneration     uint64
	testCtx             context.Context
	cancelTests         context.CancelFunc
	manager             core.Manager
	lang                i18n.Language
	settings            settings
	style               styler
	ctx                 context.Context
	snapshot            snapshot
	page                page
	width, height       int
	cursor              int

	proxyGroup               string
	filter                   string
	form                     string
	formID                   uint64
	formPage                 page
	formField                core.ToolField
	input                    []rune
	formHistories            [3]formHistory
	inputCursor              int
	profileName              string
	profileLink              string
	nameFocused              bool
	profileSchedule          core.ProfileSchedule
	profileInterval          string
	intervalFocused          bool
	editProfileID            string
	editProfileSource        string
	retryProfileID           string
	retryProfileSource       string
	cardUpdating             bool
	updatingProfileID        string
	connectionSelection      map[string]bool
	requests                 requestState
	services                 servicePanel
	proxyControllerAttempted bool
	retrySchedule            core.ProfileSchedule
	nextAutoCheck            time.Time
	retryName                string
	formError                string
	retryForm                string
	retryPage                page
	retryField               core.ToolField
	retryInput               []rune
	confirm                  func() error
	confirmLabel             string
	profileUpdating          bool
	profileSpinID            uint64
	profileFrame             int
	lifecycle                string
	busy                     bool
	refreshing               bool
	refreshAgain             bool
	loaded                   bool
	help                     bool
	sortByDelay              bool
	feedback                 string
	rates                    []core.Traffic
	trafficLive              bool
	logEntries               []core.LogEntry
	logsLive                 bool
	logLevel                 int
	logPaused                bool
	logOffset                int
	delays                   map[string]core.DelayResult

	pendingG bool
	detail   bool
	hits     []hit
}

func Run(manager core.Manager, input io.Reader, output io.Writer, lang i18n.Language) error {
	settings, err := loadSettings(manager.Paths.Config)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testCtx, cancelTests := context.WithCancel(ctx)
	defer cancelTests()
	m := &model{testCtx: testCtx, cancelTests: cancelTests, manager: manager.WithRuntimeCache(), lang: lang, settings: settings, style: newStyler(), ctx: ctx, width: 80, height: 24, logLevel: 1, refreshing: true, compactTerminal: runtime.GOOS == "android"}
	program := tea.NewProgram(m, tea.WithInput(input), tea.WithOutput(output), tea.WithContext(ctx))
	m.send = program.Send
	go reconnect(ctx, func() error {
		return manager.StreamTraffic(ctx, func(event core.Traffic) { program.Send(event) })
	}, func() { program.Send(trafficOffline{}) })
	go reconnect(ctx, func() error {
		return manager.StreamLogs(ctx, "debug", func(event core.LogEntry) { program.Send(event) })
	}, func() { program.Send(logsOffline{}) })
	_, err = program.Run()
	return err
}

func (m *model) testContext() context.Context {
	if m.testCtx != nil {
		return m.testCtx
	}
	return m.ctx
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
	cmds := []tea.Cmd{m.refresh(), m.tick()}
	if !m.manager.KernelPresent() {
		cmds = append(cmds, m.act(m.lang.Text("Install Mihomo"), func() error { return m.manager.Install(m.ctx) }))
		m.feedback = m.lang.Text("Installing Mihomo") + "…"
	}
	return tea.Batch(cmds...)
}

func (m *model) tick() tea.Cmd {
	return tea.Tick(m.settings.interval(), func(time.Time) tea.Msg { return refreshMsg{} })
}

func (m *model) refresh() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 7*time.Second)
		defer cancel()
		var s snapshot
		var reads sync.WaitGroup
		reads.Go(func() {
			s.systemProxy = m.manager.SystemProxy(ctx)
			s.appResidentBytes = core.ResidentBytes(os.Getpid())
			s.lanIPs, _ = core.LocalProxyNetworks()
		})
		reads.Go(func() {
			s.profile, s.profiles, s.profileErr = m.manager.ProfileLibrary()
			s.toolSettings, s.toolSettingsErr = m.manager.ToolSettings()
		})
		runtime := m.manager.ReadRuntime(ctx)
		s.status, s.err, s.overview = runtime.Status, runtime.Err, runtime.Overview
		s.proxies, s.proxyErr, s.rules = runtime.Proxies, runtime.ProxyErr, runtime.Rules
		s.connections, s.connectionErr, s.connectionsAt = runtime.Connections, runtime.ConnectionErr, runtime.ConnectionsAt
		reads.Wait()
		return s
	}
}

func profileSpinTick(id uint64) tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return profileSpin(id) })
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
		m.hits = nil
	case refreshMsg:
		if !m.refreshing {
			m.refreshing = true
			return m, tea.Batch(m.refresh(), m.tick())
		}
		return m, m.tick()
	case snapshot:
		m.refreshing = false
		m.loaded = true
		previous := m.snapshot
		if previous.status.PID != msg.status.PID {
			m.dnsFeed.pending = nil
		}
		m.applySnapshot(msg)
		checks := m.refreshServices(previous)
		autoRun := msg.toolSettings["app.auto-run"] == "true"
		if m.ctx != nil && !m.proxyControllerAttempted && !m.busy &&
			((msg.profile.Source != "" || autoRun) && (msg.status.State == "Stopped" || msg.status.State == "Failed") ||
				autoRun && msg.status.State == "Paused") {
			m.proxyControllerAttempted = true
			return m, m.act(m.lang.Text("Load proxies"), func() error {
				if autoRun {
					return m.manager.Start(m.ctx)
				}
				return m.manager.EnsureProxyController(m.ctx)
			})
		}
		if m.proxyControllerAvailable() {
			m.proxyControllerAttempted = true
		}
		now := time.Now()
		if !m.busy && m.form == "" && m.confirm == nil && now.After(m.nextAutoCheck) &&
			slices.ContainsFunc(msg.profiles, func(p core.ProfileInfo) bool { return p.UpdateDue(now) }) {
			m.nextAutoCheck = now.Add(time.Minute)
			return m, tea.Batch(checks, m.updateAllProfiles(true))
		}
		if m.refreshAgain {
			m.refreshAgain, m.refreshing = false, true
			return m, tea.Batch(checks, m.refresh())
		}
		return m, checks
	case dnsQueryDone:
		m.recordManualDNS(msg)
	case serviceResult:
		if msg.generation == m.services.generation {
			m.services.targets[msg.index] = msg.status
			m.services.pending[msg.index] = false
		}
	case serviceBatchDone:
		if msg.generation == m.services.generation {
			if m.services.cancel != nil {
				m.services.cancel()
				m.services.cancel = nil
			}
			for i, pending := range m.services.pending {
				if pending {
					m.services.pending[i] = false
				}
			}
			m.services.err = msg.err
		}
	case speedProgress:
		if msg.generation == m.speedGeneration && m.speedNodes[msg.name] && msg.result != nil {
			delete(m.speedNodes, msg.name)
			m.mergeProxyDelays(map[string]core.DelayResult{msg.name: *msg.result})
		}
	case speedDone:
		if msg.generation == m.speedGeneration && m.speedNodes != nil {
			m.speedNodes = nil
			m.mergeProxyDelays(msg.delays)
			if !m.busy {
				if msg.err != nil {
					m.feedback = m.lang.Text("Error · ") + m.lang.Error(msg.err)
				} else {
					m.feedback = msg.label + " · " + msg.detail
				}
			}
		}
	case profileSpin:
		if m.profileUpdating && uint64(msg) == m.profileSpinID {
			m.profileFrame++
			return m, profileSpinTick(m.profileSpinID)
		}
	case operation:
		m.profileUpdating, m.cardUpdating = false, false
		m.busy = false
		m.lifecycle = ""
		m.feedback = fmt.Sprintf(m.lang.Text("%s complete"), msg.label)
		if msg.detail != "" {
			m.feedback = msg.label + " · " + msg.detail
		}
		if msg.err != nil {
			m.feedback = m.lang.Text("Error · ") + m.lang.Error(msg.err)
			if m.retryForm != "" {
				m.openForm(m.retryForm, string(m.retryInput))
				m.formPage, m.formField = m.retryPage, m.retryField
				m.profileName = m.retryName
				m.editProfileID, m.editProfileSource = m.retryProfileID, m.retryProfileSource
				m.profileSchedule = m.retrySchedule
				m.profileInterval = fmt.Sprint(m.retrySchedule.Interval())
				m.formError = m.lang.Error(msg.err)
			}
		}
		m.retryForm, m.retryInput, m.retryName = "", nil, ""
		if !m.refreshing {
			m.refreshing = true
			return m, m.refresh()
		}
		m.refreshAgain = true
	case core.Traffic:
		m.trafficLive = true
		m.rates = append(m.rates, msg)
		m.rates = m.rates[max(0, len(m.rates)-60):]
	case trafficOffline:
		m.trafficLive = false
	case core.LogEntry:
		m.recordDNSEvent(msg)
		m.logsLive = true
		if !m.logPaused {
			if m.logOffset > 0 && levelIndex(msg.Type) >= m.logLevel {
				m.logOffset = min(len(m.logEntries), m.logOffset+1)
			}
			m.logEntries = append(m.logEntries, msg)
			m.logEntries = m.logEntries[max(0, len(m.logEntries)-200):]
		}
	case logsOffline:
		m.logsLive = false
	case tea.PasteMsg:
		if m.form != "" {
			m.addInput(msg.Content)
		}
	case clipboardMsg:
		if m.form != "" && msg.formID == m.formID {
			if msg.err != nil {
				m.formError = m.lang.Error(msg.err)
				m.feedback = m.lang.Text("Error · ") + m.lang.Error(msg.err)
			} else {
				m.addInput(msg.text)
			}
		}
	case tea.KeyPressMsg:
		return m, m.key(msg)
	case tea.MouseClickMsg:
		return m, m.mouse(msg)
	case tea.MouseWheelMsg:
		return m, m.mouse(msg)
	}
	return m, nil
}

func (m *model) applySnapshot(next snapshot) {
	focusOnLoad := false
	if m.page == proxies && m.cursor == 0 && m.filter == "" {
		tabs := m.proxyTabs()
		focusOnLoad = tabs[m.proxyTabIndex(tabs)].Now == ""
	}
	var selected string
	if m.page == connectionsPage {
		items := m.visibleConnections()
		if len(items) > 0 {
			selected = items[min(m.cursor, len(items)-1)].ID
		}
	}
	if (next.status.State != "Running" && next.status.State != "Paused") || next.status.PID != m.snapshot.status.PID ||
		next.profile.ID != m.snapshot.profile.ID || !next.profile.Updated.Equal(m.snapshot.profile.Updated) {
		m.delays = nil
		if m.speedNodes != nil {
			m.speedNodes = nil
			m.speedGeneration++
		}
	}
	if len(m.connectionSelection) > 0 && next.connectionErr == nil {
		remaining := make(map[string]bool)
		for _, item := range next.connections {
			if m.connectionSelection[item.ID] {
				remaining[item.ID] = true
			}
		}
		m.connectionSelection = remaining
	}
	if next.status.State != "Running" && next.status.State != "Paused" && next.profile.Source != "" && next.profileErr == nil &&
		next.profile.ID == m.snapshot.profile.ID && next.profile.Source == m.snapshot.profile.Source &&
		next.profile.Updated.Equal(m.snapshot.profile.Updated) && len(m.snapshot.proxies) > 0 && m.snapshot.proxyErr == nil {
		next.proxies, next.proxyErr = m.snapshot.proxies, nil
	}
	m.updateRequestSamples(next)
	m.snapshot = next
	m.requests.cache = nil
	if focusOnLoad {
		m.focusSelectedProxy()
	}
	if next.connectionErr == nil {
		switch next.status.State {
		case "Running", "Paused", "Stopped":
			m.pruneRequestFilters()
		}
	}
	if selected == "" {
		return
	}
	for i, item := range m.visibleConnections() {
		if item.ID == selected {
			m.cursor = i
			return
		}
	}
	m.detail = false
	m.cursor = min(m.cursor, max(0, len(m.visibleConnections())-1))
}

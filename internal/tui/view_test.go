package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/i18n"
	"github.com/charmbracelet/x/ansi"
)

func richModel() *model {
	now := time.Now()
	m := &model{
		lang:     i18n.English,
		settings: settings{RefreshSeconds: 3},
		style:    styler{mode: 3, p: darkPalette},
		width:    120,
		height:   40,
		snapshot: snapshot{
			status:   core.Status{State: "Running", Installed: "v1.18.0", PID: 4242, Controller: "127.0.0.1:9090", Proxy: "127.0.0.1:7890", Started: now.Add(-time.Minute)},
			profile:  core.ProfileInfo{Source: "subscription", Updated: now},
			overview: core.Overview{Mode: "rule", Connections: 2},
			proxies: []core.Proxy{
				{Name: "Proxy", Type: "Selector", Now: "A", All: []string{"A", "B"}},
				{Name: "A", Type: "Shadowsocks", Alive: true, History: []struct{ Delay int }{{Delay: 80}, {Delay: 120}}},
				{Name: "B", Type: "Trojan", Alive: false},
			},
			rules: []core.Rule{
				{Index: 0, Type: "Domain", Payload: "example.org", Proxy: "Proxy"},
				{Index: 1, Type: "Match", Proxy: "DIRECT"},
			},
			connections: []core.Connection{
				{ID: "11111111-1111-1111-1111-111111111111", Upload: 1024, Download: 2048, Rule: "Domain", Chains: []string{"A", "Proxy"}},
				{ID: "22222222-2222-2222-2222-222222222222", Upload: 512, Download: 256, Rule: "Match"},
			},
		},
		rates:      []core.Traffic{{Up: 100, Down: 2000, UpTotal: 1000, DownTotal: 20000}, {Up: 150, Down: 4000, UpTotal: 1150, DownTotal: 24000}},
		logEntries: []core.LogEntry{{Type: "info", Payload: "started"}, {Type: "warning", Payload: "slow"}, {Type: "error", Payload: "boom"}},
	}
	m.snapshot.connections[0].Metadata.Host = "example.org"
	m.snapshot.connections[0].Metadata.Network = "tcp"
	m.snapshot.connections[0].Metadata.DestinationPort = "443"
	m.snapshot.connections[1].Metadata.DestinationIP = "10.0.0.1"
	m.snapshot.connections[1].Metadata.Network = "udp"
	return m
}

func TestViewBelowMinimumShowsResizeNotice(t *testing.T) {
	for _, size := range [][2]int{{79, 24}, {80, 23}, {40, 10}} {
		m := richModel()
		m.width, m.height = size[0], size[1]
		content := m.View().Content
		if !strings.Contains(content, "Terminal too small") || !strings.Contains(content, "80×24") {
			t.Fatalf("size %v: expected resize notice, got %q", size, content)
		}
	}
}

func TestViewRendersAllPagesWithinBounds(t *testing.T) {
	for _, width := range []int{80, 100, 120} {
		for _, height := range []int{24, 40} {
			for p, title := range pages {
				if title == "" {
					continue
				}
				m := richModel()
				m.width, m.height = width, height
				m.page = page(p)
				content := m.View().Content
				lines := strings.Split(content, "\n")
				if len(lines) != height {
					t.Fatalf("page %d %dx%d: got %d lines, want %d", p, width, height, len(lines), height)
				}
				for i, ln := range lines {
					if w := ansi.StringWidth(ln); w > width {
						t.Fatalf("page %d %dx%d line %d: width %d exceeds %d", p, width, height, i, w, width)
					}
				}
			}
		}
	}
}

func TestCompactTerminalRendersAllPages(t *testing.T) {
	for _, lang := range []i18n.Language{i18n.English, i18n.Chinese} {
		for index := range pages {
			m := richModel()
			m.lang, m.page = lang, page(index)
			m.compactTerminal = true
			m.width, m.height = compactMinCols, minRows
			screen := m.View().Content
			if lines := strings.Split(screen, "\n"); len(lines) != m.height {
				t.Fatalf("compact page %d has %d rows, want %d", index, len(lines), m.height)
			} else {
				for _, line := range lines {
					if width(line) > m.width {
						t.Fatalf("compact page %d exceeds %d columns: %q", index, m.width, line)
					}
				}
			}
		}
	}
}

func TestViewModalsRenderWithinBounds(t *testing.T) {
	m := richModel()
	m.help = true
	if lines := strings.Split(m.View().Content, "\n"); len(lines) != m.height {
		t.Fatalf("help modal: got %d lines", len(lines))
	}
	m.help = false
	m.form, m.input = "Subscription URL", []rune("https://secret.example/sub")
	content := m.View().Content
	if !strings.Contains(content, "secret.example") {
		t.Fatal("subscription URL should be shown in plaintext in the form")
	}
	m.form, m.input = "", nil
	m.confirmLabel, m.confirm = "Stop Mihomo", func() error { return nil }
	if lines := strings.Split(m.View().Content, "\n"); len(lines) != m.height {
		t.Fatalf("confirm modal: got %d lines", len(lines))
	}
}

func TestViewEmitsColorWhenCapable(t *testing.T) {
	m := richModel()
	if !strings.Contains(m.View().Content, "\x1b[") {
		t.Fatal("expected ANSI styling in truecolor mode")
	}
	m.style = styler{mode: 0, p: darkPalette}
	if strings.Contains(m.View().Content, "\x1b[") {
		t.Fatal("expected no ANSI styling when color is disabled")
	}
}

func TestVisibleConnectionsFilter(t *testing.T) {
	m := richModel()
	m.page = connectionsPage
	m.filter = "example"
	items := m.visibleConnections()
	if len(items) != 1 || items[0].Metadata.Host != "example.org" {
		t.Fatalf("unexpected filtered connections: %+v", items)
	}
	m.filter = "udp"
	if items := m.visibleConnections(); len(items) != 1 || items[0].Metadata.Network != "udp" {
		t.Fatalf("unexpected network filter result: %+v", items)
	}
}

func TestLogPauseStopsAppending(t *testing.T) {
	m := &model{logPaused: true}
	m.Update(core.LogEntry{Type: "info", Payload: "x"})
	if len(m.logEntries) != 0 {
		t.Fatal("paused log must not append entries")
	}
	m.logPaused = false
	m.Update(core.LogEntry{Type: "info", Payload: "y"})
	if len(m.logEntries) != 1 {
		t.Fatal("resumed log must append entries")
	}
}

func TestProfilePageFitsMinimumHeight(t *testing.T) {
	m := richModel()
	m.width, m.height = 80, 24
	body := m.height - 4
	if lines := m.profilesPage(body); len(lines) > body {
		t.Fatalf("profile page needs %d rows, only %d available", len(lines), body)
	}
}

func TestConnectionRowFitsPanel(t *testing.T) {
	m := richModel()
	m.width = 80
	c := m.snapshot.connections[0]
	c.Chains = []string{"🇭🇰 香港S01", "🔰 选择节点", "🐟 漏网之鱼"}
	c.Download, c.Upload = 459*1024, 2500
	const available = 80 - 6
	if w := ansi.StringWidth(m.connectionRow(c, available)); w > available {
		t.Fatalf("connection row is %d wide, want <= %d", w, available)
	}
}

func TestBytesLabelFitsConnectionColumn(t *testing.T) {
	for _, value := range []int64{0, 999, 1024, 88 * 1024, 459 * 1024, 1023 * 1024, 5 << 30} {
		if label := bytesLabel(value); ansi.StringWidth(label) > 8 {
			t.Fatalf("bytesLabel(%d) = %q is wider than 8 columns", value, label)
		}
	}
}

func TestLegacyAppearanceSettingsAreIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.yaml"), []byte("theme: light\nvisual: minimal\nrefresh-seconds: 5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := loadSettings(dir)
	if err != nil || config.RefreshSeconds != 5 {
		t.Fatalf("legacy settings: %+v, %v", config, err)
	}
}

func TestDashboardOverviewShowsCurrentNode(t *testing.T) {
	m := richModel()
	m.lang = i18n.Chinese
	m.width, m.height = 80, 24
	m.snapshot.rules = []core.Rule{
		{Type: "Domain", Proxy: "DIRECT"},
		{Type: "Match", Proxy: "Fallback"},
	}
	m.snapshot.proxies = append(m.snapshot.proxies, core.Proxy{Name: "Fallback", Now: "Proxy"})
	m.snapshot.proxies[0].Now = "香港"
	overviewRow := func() string {
		row := ansi.Strip(strings.Split(m.View().Content, "\n")[4])
		_, overview, _ := strings.Cut(row, "│")
		return strings.SplitN(overview, "│", 3)[1]
	}
	if screen := overviewRow(); !strings.HasSuffix(strings.TrimSpace(screen), "  香港") {
		t.Fatalf("missing resolved node in dashboard overview: %s", screen)
	}
	next := m.snapshot
	next.proxies = slices.Clone(next.proxies)
	next.proxies[0].Now = "日本"
	m.applySnapshot(next)
	if screen := overviewRow(); !strings.HasSuffix(strings.TrimSpace(screen), "  日本") {
		t.Fatalf("missing updated node in dashboard overview: %s", screen)
	}
	m.snapshot.overview.Mode = "direct"
	if screen := overviewRow(); !strings.HasSuffix(strings.TrimSpace(screen), "  DIRECT") {
		t.Fatalf("direct mode should display DIRECT: %s", screen)
	}
	m.snapshot.status.State = "Stopped"
	if screen := overviewRow(); strings.Contains(screen, "DIRECT") {
		t.Fatalf("stopped mode should clear the node: %s", screen)
	}
}

func TestViewUsesTerminalBackground(t *testing.T) {
	m := richModel()
	for _, mode := range []int{1, 2, 3} {
		m.style.mode = mode
		for p, title := range pages {
			if title == "" {
				continue
			}
			m.page = page(p)
			screen := m.View().Content
			for _, prefix := range []string{"\x1b[4", "\x1b[10"} {
				if strings.Contains(screen, prefix) {
					t.Fatalf("page %d in color mode %d overrides the terminal background", p, mode)
				}
			}
		}
	}
}

func TestChineseLabelsInProfileHelpAndLogs(t *testing.T) {
	m := richModel()
	m.lang = i18n.Chinese
	m.page = profiles
	m.snapshot.profiles = []core.ProfileInfo{{ID: "test", Name: "Example", Source: "subscription", Active: true}}
	view := ansi.Strip(m.View().Content)
	for _, label := range []string{"[e 编辑]", "[d 删除]", "[s 更新]", "Example"} {
		if !strings.Contains(view, label) {
			t.Errorf("profile card is missing %q", label)
		}
	}
	help := ansi.Strip(strings.Join(m.helpLines(), "\n"))
	if strings.Contains(help, "Profile") || strings.Contains(help, "Logs") {
		t.Error("shortcut help still contains English section labels")
	}
	log := ansi.Strip(m.logLine(core.LogEntry{Type: "warning", Payload: "original message"}))
	if !strings.Contains(log, "警告") || !strings.Contains(log, "original message") {
		t.Errorf("expected the localized log level and original message, got %q", log)
	}
}

func TestProxyPageUsesModeAndConfiguredGroupOrder(t *testing.T) {
	m := richModel()
	m.snapshot.proxies = []core.Proxy{
		{Name: "GLOBAL", Hidden: true, All: []string{"A"}},
		{Name: "选择节点", All: []string{"香港", "日本"}},
		{Name: "OneDrive", All: []string{"日本"}},
	}
	m.changePage(proxies)
	if m.proxyGroup != "选择节点" {
		t.Fatalf("default rule group = %q", m.proxyGroup)
	}
	if items := m.visibleProxies(); len(items) != 2 || items[0].Name != "香港" || items[1].Name != "日本" {
		t.Fatalf("unexpected default group members: %+v", items)
	}
	m.snapshot.overview.Mode = "global"
	m.changePage(proxies)
	if m.proxyGroup != "GLOBAL" {
		t.Fatalf("default global group = %q", m.proxyGroup)
	}
	m.snapshot.overview.Mode = "direct"
	if len(m.visibleProxies()) != 0 {
		t.Fatal("direct mode should not show nodes")
	}
}

func TestRequestsTableFiltersCategoriesAndRates(t *testing.T) {
	m := richModel()
	m.changePage(connectionsPage)
	m.lang = i18n.Chinese
	m.width = 164
	m.snapshot.connectionsAt = time.Now()
	m.snapshot.connections[0].Metadata.Process = "codex"
	m.snapshot.connections[0].Metadata.UID = 1000
	m.snapshot.connections[0].Start = m.snapshot.connectionsAt.Add(-time.Minute)
	next := m.snapshot
	next.connections = append([]core.Connection(nil), next.connections[:1]...)
	next.connectionsAt = next.connectionsAt.Add(2 * time.Second)
	next.connections[0].Download += 48
	next.connections[0].Upload += 56
	m.applySnapshot(next)

	screen := ansi.Strip(m.View().Content)
	for _, label := range []string{"运行中", "进程", "目标", "协议", "规则", "出口", "下载总量", "上传总量", "时长", "codex(1000)", "24B/s", "28B/s"} {
		if !strings.Contains(screen, label) {
			t.Errorf("connection table missing %q", label)
		}
	}
	clickAction(t, m, "c")
	m.handleKey("j", "")
	m.handleKey("enter", "")
	if m.requests.category != 1 || !strings.Contains(m.View().Content, "┄ codex(1000)") {
		t.Fatal("application category should add an application heading")
	}
	m.handleKey("v", "")
	if items := m.visibleConnections(); m.requests.menu != "" || len(items) != 1 || items[0].Metadata.Network != "tcp" {
		t.Fatalf("requests must show only active connections: %+v", items)
	}
	m.View()
	for _, h := range m.hits {
		if h.kind == hitRequestFilter && h.key == "protocol:TCP" {
			doClick(m, h)
			if m.requests.protocols["TCP"] && len(m.visibleConnections()) == 1 {
				return
			}
		}
	}
	t.Fatal("TCP filter chip should select the protocol")
}

func TestDashboardMemoryShowsBothProcessesAtMinimumWidth(t *testing.T) {
	m := richModel()
	m.lang = i18n.Chinese
	m.width, m.height = minCols, minRows
	m.snapshot.appResidentBytes = 15 * 1024 * 1024
	m.snapshot.overview.ResidentBytes = 60 * 1024 * 1024
	screen := ansi.Strip(m.View().Content)
	for _, text := range []string{"75.0MiB", "应用15.0MiB", "内核60.0MiB", "━", "─"} {
		if !strings.Contains(screen, text) {
			t.Errorf("memory card is missing %q", text)
		}
	}
}

func TestDashboardLANDisplaysActualMixedPort(t *testing.T) {
	m := richModel()
	m.width = 164
	m.snapshot.lanIPs = []string{"192.168.8.224"}
	m.snapshot.status.Proxy = "127.0.0.1:38441"
	m.snapshot.overview.LAN = true
	screen := ansi.Strip(m.View().Content)
	for _, text := range []string{"192.168.8.224", "Port 38441", "127.0.0.1:38441"} {
		if !strings.Contains(screen, text) {
			t.Errorf("dashboard is missing %q", text)
		}
	}
	if strings.Contains(screen, "192.168.8.224:") {
		t.Fatal("the LAN address and port should be displayed on separate rows")
	}
}

func TestServiceCardOpensListAndRefreshesSelectedTarget(t *testing.T) {
	m := richModel()
	m.lang = i18n.Chinese
	m.snapshot.status.Proxy = "127.0.0.1:7890"
	for i := range serviceTargets {
		m.services.targets[i] = core.ServiceStatus{Status: core.ServiceAvailable, Delay: 165 * time.Millisecond}
	}
	if !strings.Contains(m.View().Content, "全部可用") {
		t.Fatal("dashboard missing available summary")
	}
	clickAction(t, m, "e")
	for _, target := range serviceTargets {
		if !strings.Contains(m.View().Content, target.name) {
			t.Errorf("dialog missing %s", target.name)
		}
	}
	m.handleKey("j", "")
	if cmd := m.handleKey("enter", ""); cmd == nil || !m.services.pending[1] || m.services.pending[0] {
		t.Fatal("only selected service should be pending")
	}
	m.Update(serviceResult{m.services.generation, 1, core.ServiceStatus{Status: core.ServiceTimeout}})
	if m.services.pending[1] || !strings.Contains(m.View().Content, "超时") {
		t.Fatal("service timeout should replace the loading state")
	}
}

func TestServiceDialogListsAllTargetsAtMinimumSize(t *testing.T) {
	m := richModel()
	m.width, m.height = minCols, minRows
	m.handleKey("f", "")
	for i, pending := range m.services.pending {
		if pending != (i < dashboardServiceCount) {
			t.Fatalf("dashboard scheduled unexpected target %s", serviceTargets[i].name)
		}
	}
	m.handleKey("e", "")
	screen := ansi.Strip(m.View().Content)
	for _, target := range serviceTargets {
		if !strings.Contains(screen, target.name) {
			t.Errorf("service dialog missing %s", target.name)
		}
	}
	m.Update(serviceBatchDone{generation: m.services.generation})
	m.handleKey("f", "")
	for i, pending := range m.services.pending {
		if !pending {
			t.Errorf("check all did not schedule %s", serviceTargets[i].name)
		}
	}
}

func TestStoppedProxyPageKeepsOnlyTheMatchingProfile(t *testing.T) {
	m := richModel()
	m.page = proxies
	m.snapshot.profile.ID = "active"
	profile := m.snapshot.profile
	stopped := snapshot{status: core.Status{State: "Stopped"}, profile: profile}
	m.applySnapshot(stopped)
	screen := ansi.Strip(m.View().Content)
	if !strings.Contains(screen, "Proxy") || !strings.Contains(screen, "Shadowsocks") || !strings.Contains(screen, "Offline list") {
		t.Fatal("the stopped proxy page should keep the active profile's nodes")
	}
	if m.handleKey("enter", "") != nil || m.busy {
		t.Fatal("offline testing should not start a request")
	}

	stopped.profile.ID = "other"
	stopped.proxies = []core.Proxy{{Name: "New node"}}
	m.applySnapshot(stopped)
	screen = ansi.Strip(m.View().Content)
	if !strings.Contains(screen, "New node") || strings.Contains(screen, "Shadowsocks") {
		t.Fatal("the offline page should show the newly activated profile only")
	}
}

func TestQuitCancelsTestsWhileKeepingMutationGuard(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		t.Run(key, func(t *testing.T) {
			m := richModel()
			m.testCtx, m.cancelTests = context.WithCancel(t.Context())
			defer m.cancelTests()
			m.busy = true
			if cmd := m.handleKey(key, ""); cmd != nil || m.testCtx.Err() != nil {
				t.Fatal("configuration mutations should retain the exit guard")
			}
			m.busy = false
			m.beginSpeedTest([]string{"A"})
			delete(m.speedNodes, "A") // The final result may arrive before the operation message.
			cmd := m.handleKey(key, "")
			if cmd == nil || !errors.Is(m.testCtx.Err(), context.Canceled) {
				t.Fatal("quit should cancel active tests before exiting")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("expected a quit command")
			}
		})
	}
}

func TestRequestFiltersCanSelectSeveralRules(t *testing.T) {
	m := richModel()
	m.changePage(connectionsPage)
	m.snapshot.connections = []core.Connection{
		{ID: "domain", Rule: "Domain"},
		{ID: "keyword", Rule: "DomainKeyword"},
		{ID: "suffix", Rule: "DomainSuffix"},
		{ID: "udp", Rule: "Domain"},
	}
	for i := range m.snapshot.connections {
		m.snapshot.connections[i].Metadata.Network = "tcp"
	}
	m.snapshot.connections[3].Metadata.Network = "udp"

	m.View()
	clickFilter := func(key string) {
		t.Helper()
		for _, h := range m.hits {
			if h.kind == hitRequestFilter && h.key == key {
				doClick(m, h)
				m.View()
				return
			}
		}
		t.Fatalf("missing filter chip %s", key)
	}
	clickFilter("rule:Domain")
	clickFilter("rule:DomainKeyword")
	if len(m.visibleConnections()) != 3 {
		t.Fatal("selected rules should match either rule")
	}
	clickFilter("protocol:TCP")
	if items := m.visibleConnections(); len(items) != 2 {
		t.Fatalf("the selected protocol should narrow the rule results: %+v", items)
	}
	m.handleKey("ctrl+t", "")
	m.handleKey("enter", "") // Deselect Domain.
	if m.requests.menu != "Rule" || len(m.visibleConnections()) != 1 {
		t.Fatal("keyboard filtering should toggle one rule and leave the menu open")
	}
	m.handleKey("esc", "")
	m.handleKey("ctrl+p", "")
	m.handleKey("down", "")
	m.handleKey("enter", "") // Select UDP alongside TCP.
	if len(m.requests.protocols) != 2 {
		t.Fatal("keyboard filters should retain both selected protocols")
	}
}

func TestRequestsHeadingTracksProxyStatus(t *testing.T) {
	m := richModel()
	m.changePage(connectionsPage)
	m.lang = i18n.Chinese
	for _, item := range []struct{ state, label string }{
		{"Running", "运行中"},
		{"Paused", "代理已关闭"},
		{"Stopped", "已停止"},
		{"Running", "运行中"},
	} {
		m.snapshot.status.State = item.state
		row := ansi.Strip(m.connectionsPageView(m.height - 4)[0])
		if !strings.HasPrefix(row, item.label) {
			t.Errorf("state %q: expected heading %q, got %q", item.state, item.label, row)
		}
	}
}

func TestRequestFilterChipsFollowActiveConnections(t *testing.T) {
	m := richModel()
	m.changePage(connectionsPage)
	m.toggleRequestFilter("protocol:TCP")
	m.toggleRequestFilter("rule:Domain")
	m.requests.category = 4

	next := m.snapshot
	next.connections = next.connections[1:]
	m.applySnapshot(next)

	screen := ansi.Strip(m.View().Content)
	if !strings.Contains(screen, "[UDP]") || strings.Contains(screen, "[TCP]") || strings.Contains(screen, "[Domain]") {
		t.Fatal("filter chips should reflect the latest active connections")
	}
	if len(m.requests.protocols) != 0 || len(m.requests.rules) != 0 || len(m.visibleConnections()) != 1 {
		t.Fatal("disappearing filter options should not block remaining requests")
	}
	if m.requests.category != 4 {
		t.Fatal("filter changes should preserve the selected category")
	}
}

func TestProxyAddressDoesNotAppearInTopbar(t *testing.T) {
	m := richModel()
	for current := range pages {
		m.page = page(current)
		screen := ansi.Strip(m.View().Content)
		line, _, _ := strings.Cut(screen, "\n")
		if strings.Contains(line, m.snapshot.status.Proxy) {
			t.Errorf("page %d shows a proxy address in the topbar", current)
		}
		if m.page == dashboard && !strings.Contains(screen, m.snapshot.status.Proxy) {
			t.Error("the dashboard status card should show the proxy address")
		}
	}
}

func TestSidebarFitsLocalizedMenuWithoutExtraWidth(t *testing.T) {
	m := richModel()
	for _, lang := range []i18n.Language{i18n.English, i18n.Chinese} {
		m.lang = lang
		sidebar := ansi.Strip(strings.Join(m.sidebar(m.sidebarWidth(), true), "\n"))
		for _, section := range menuSections {
			if !strings.Contains(sidebar, lang.Text(section.label)) {
				t.Errorf("%s sidebar clips %q", lang, section.label)
			}
		}
		if m.sidebarWidth() >= 22 {
			t.Errorf("%s sidebar retained the old width", lang)
		}
	}
}

func TestServiceSetupErrorDoesNotCountAsFailedServices(t *testing.T) {
	m := richModel()
	m.lang = i18n.Chinese
	m.handleKey("f", "")
	m.Update(serviceBatchDone{generation: m.services.generation, err: core.ErrServiceProbeNeedsRestart})
	screen := ansi.Strip(m.View().Content)
	if !strings.Contains(screen, "需重启内核") || strings.Contains(screen, "项异常") {
		t.Fatal("a missing listener should prompt for restart instead of reporting failed sites")
	}
	m.handleKey("e", "")
	if !strings.Contains(m.View().Content, "启用指定节点") {
		t.Fatal("expected the configuration error in the service dialog")
	}
}

func TestServiceRefreshKeepsResultsWhenUnrelatedGroupChanges(t *testing.T) {
	m := richModel()
	m.snapshot.rules = []core.Rule{{Type: "Match", Proxy: "Proxy"}}
	m.snapshot.proxies = append(m.snapshot.proxies, core.Proxy{Name: "Other", Type: "Selector", Now: "B"})
	previous := m.snapshot
	m.snapshot.proxies = slices.Clone(previous.proxies)
	m.snapshot.proxies[len(m.snapshot.proxies)-1].Now = "A"
	if cmd := m.refreshServices(previous); cmd != nil {
		t.Fatal("changing an unrelated selector should not restart service probes")
	}
	m.snapshot.proxies[0].Now = "B"
	if cmd := m.refreshServices(previous); cmd == nil {
		t.Fatal("changing the effective exit should restart service probes")
	}
}

func TestGeneralAndDNSAreToolsTabs(t *testing.T) {
	m := richModel()
	m.handleKey("5", "")
	if m.page != generalPage {
		t.Fatal("Tools should open General first")
	}
	m.handleKey("enter", "")
	if m.form != "Auto-run" {
		t.Fatalf("General edit opened %q", m.form)
	}
	m.handleKey("esc", "")
	m.handleKey("down", "")
	m.handleKey("enter", "")
	if m.form != "Start on boot" {
		t.Fatalf("second General setting opened %q", m.form)
	}
	m.handleKey("esc", "")
	m.handleKey("]", "")
	if m.page != languagePage {
		t.Fatal("Language should be the second tools tab")
	}
	m.handleKey("]", "")
	if m.page != networkPage {
		t.Fatal("Network should be the third tools tab")
	}
	m.snapshot.systemProxy = core.SystemProxy{Available: true, Exclusions: []string{"localhost", "*.example.com"}}
	m.handleKey("down", "")
	m.handleKey("enter", "")
	screen := ansi.Strip(m.View().Content)
	if !m.showProxyExclusions || !strings.Contains(screen, "localhost") || !strings.Contains(screen, "*.example.com") {
		t.Fatalf("network list did not load desktop exclusions: %s", screen)
	}
	m.handleKey("down", "")
	m.handleKey("enter", "")
	if m.form != "Edit proxy exclusion" || string(m.input) != "*.example.com" {
		t.Fatalf("edit did not load the selected exclusion: %q %q", m.form, m.input)
	}
	m.handleKey("esc", "")
	m.handleKey("a", "")
	if m.form != "Add proxy exclusion" || len(m.input) != 0 {
		t.Fatalf("add should open an empty form: %q %q", m.form, m.input)
	}
	m.handleKey("esc", "")
	m.handleKey("d", "")
	if m.confirm == nil || !strings.Contains(m.confirmLabel, "*.example.com") {
		t.Fatalf("delete did not confirm the selected exclusion: %q", m.confirmLabel)
	}
	m.handleKey("esc", "")
	m.handleKey("esc", "")
	m.handleKey("]", "")
	if m.page != corePage {
		t.Fatal("expected Core after Network")
	}
	m.handleKey("]", "")
	if m.page != dnsPage {
		t.Fatal("expected DNS directly after Core")
	}
	m.handleKey("a", "")
	if m.form != "DNS query" {
		t.Fatalf("DNS query opened %q", m.form)
	}
}

func TestGeneralPortShowsDefaultAndExplicitValue(t *testing.T) {
	m := richModel()
	m.changePage(generalPage)
	if screen := ansi.Strip(m.View().Content); !strings.Contains(screen, "7890 (default)") {
		t.Fatalf("default port missing: %s", screen)
	}
	m.handleKey("down", "")
	m.handleKey("down", "")
	m.handleKey("enter", "")
	if screen := ansi.Strip(m.View().Content); !strings.Contains(screen, "Leave empty to use port 7890") {
		t.Fatalf("port edit hint missing: %s", screen)
	}
	m.handleKey("esc", "")
	m.snapshot.toolSettings = map[string]string{"mixed-port": "7891"}
	if screen := ansi.Strip(m.View().Content); !strings.Contains(screen, "7891") || strings.Contains(screen, "7890 (default)") {
		t.Fatalf("explicit port missing: %s", screen)
	}
}

func TestDNSPageShowsLiveRecordsAndFilters(t *testing.T) {
	m := richModel()
	m.changePage(dnsPage)
	now := time.Now()
	source := "udp://1.1.1.1:53"
	m.recordDNSEvent(core.LogEntry{Time: now, DNS: &core.DNSEvent{Domain: "live.example", Type: "A", Source: source, Started: true}})
	m.recordDNSEvent(core.LogEntry{Time: now.Add(25 * time.Millisecond), DNS: &core.DNSEvent{Domain: "live.example", Type: "A", Source: source, Result: "192.0.2.1"}})
	m.recordDNSEvent(core.LogEntry{Time: now, DNS: &core.DNSEvent{Domain: "cached.example", Type: "AAAA", Result: "2001:db8::1", Cached: true}})
	screen := ansi.Strip(m.View().Content)
	for _, want := range []string{"live.example", "192.0.2.1", "25 ms", "cached.example", "[Cached]"} {
		if !strings.Contains(screen, want) {
			t.Errorf("DNS page missing %q", want)
		}
	}
	m.toggleDNSFilter("Cached")
	if rows := m.visibleDNSRows(); len(rows) != 1 || rows[0].name != "cached.example" {
		t.Fatalf("unexpected filtered records: %+v", rows)
	}
}

func TestDNSDetailsAndCacheAction(t *testing.T) {
	m := richModel()
	m.lang = i18n.Chinese
	m.changePage(dnsPage)
	m.addDNSRow(dnsRow{
		at:   time.Date(2026, 11, 15, 15, 0, 54, 0, time.Local),
		name: "_https._tcp.example.com", kind: "SRV", initiator: "Manual",
		source: "udp://119.29.29.29:53", cacheKnown: true, rcode: "NXDOMAIN",
		elapsed: 122 * time.Millisecond, timed: true,
	})
	clickAction(t, m, "c")
	if m.confirm == nil || m.confirmLabel != "清除 DNS 缓存" {
		t.Fatal("cache button should open its own confirmation")
	}
	m.handleKey("esc", "")
	m.handleKey("enter", "")
	screen := ansi.Strip(m.View().Content)
	for _, want := range []string{"DNS 详情", "基础信息", "2026-11-15 15:00:54", "_https._tcp.example.com",
		"记录类型", "SRV", "发起方", "手动", "来源", "udp://119.29.29.29:53", "缓存", "否", "响应码", "NXDOMAIN", "122 ms"} {
		if !strings.Contains(screen, want) {
			t.Errorf("DNS details missing %q", want)
		}
	}
}

func TestQuitKeyClosesOnlyCurrentLayer(t *testing.T) {
	for _, tc := range []struct {
		name  string
		open  func(*model)
		shown func(*model) bool
	}{
		{"services", func(m *model) { m.services.dialog = true }, func(m *model) bool { return m.services.dialog }},
		{"help", func(m *model) { m.help = true }, func(m *model) bool { return m.help }},
		{"node detail", func(m *model) { m.page, m.detail = proxies, true }, func(m *model) bool { return m.detail }},
		{"request detail", func(m *model) { m.page, m.detail = connectionsPage, true }, func(m *model) bool { return m.detail }},
		{"DNS detail", func(m *model) { m.page, m.detail = dnsPage, true }, func(m *model) bool { return m.detail }},
		{"request filter", func(m *model) { m.page = connectionsPage; m.openRequestMenu("Category") }, func(m *model) bool { return m.requests.menu != "" }},
		{"confirmation", func(m *model) { m.confirm = func() error { t.Error("confirmation ran"); return nil } }, func(m *model) bool { return m.confirm != nil }},
		{"form", func(m *model) { m.openForm("Subscription URL", "") }, func(m *model) bool { return m.form != "" }},
		{"exclusion list", func(m *model) { m.page, m.showProxyExclusions = networkPage, true }, func(m *model) bool { return m.showProxyExclusions }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := richModel()
			tc.open(m)
			page := m.page
			if cmd := m.handleKey("q", "q"); cmd != nil || tc.shown(m) || m.page != page {
				t.Fatal("q must only close the current layer")
			}
			cmd := m.handleKey("q", "q")
			if cmd == nil {
				t.Fatal("q on the main screen must quit")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("expected a quit command on the main screen")
			}
		})
	}
}

func TestQuitKeyReturnsFromExclusionEditorToList(t *testing.T) {
	m := richModel()
	m.page, m.showProxyExclusions = networkPage, true
	m.openForm("Add proxy exclusion", "")
	m.handleKey("alt+q", "")
	if string(m.input) != "q" {
		t.Fatalf("form input = %q", m.input)
	}
	if cmd := m.handleKey("q", "q"); cmd != nil || m.form != "" || !m.showProxyExclusions {
		t.Fatal("first q must close the form and keep the list")
	}
	if cmd := m.handleKey("q", "q"); cmd != nil || m.showProxyExclusions || m.cursor != networkExclusions {
		t.Fatal("second q must return to Network settings")
	}
	m.services.dialog, m.page = true, dashboard
	cmd := m.handleKey("ctrl+c", "")
	if cmd == nil {
		t.Fatal("Ctrl+C must quit from a modal")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("expected Ctrl+C to quit")
	}
}

func TestLogHistoryStaysInPlaceDuringBurst(t *testing.T) {
	m := richModel()
	m.page, m.height, m.logLevel, m.logEntries = logs, 24, levelIndex("info"), nil
	initial := make(logBatch, 40)
	for i := range initial {
		initial[i].entry = core.LogEntry{Type: "info", Payload: "history-" + time.Unix(int64(i), 0).UTC().Format("15:04:05")}
	}
	m.Update(initial)
	m.scroll(-7)
	before := m.logsPage(m.height - 4)
	m.Update(logBatch{
		{entry: core.LogEntry{Type: "debug", Payload: "hidden"}},
		{entry: core.LogEntry{Type: "info", Payload: "newest"}},
	})
	if after := m.logsPage(m.height - 4); !slices.Equal(before, after) {
		t.Fatal("new log events moved the historical viewport")
	}
	m.scroll(100)
	if tail := strings.Join(m.logsPage(m.height-4), "\n"); !strings.Contains(tail, "newest") || strings.Contains(tail, "hidden") {
		t.Fatal("returning to the tail did not show the newest matching event")
	}
}

func TestLogHistoryClampsToRetainedRows(t *testing.T) {
	m := richModel()
	m.page, m.height, m.logLevel, m.logEntries = logs, 24, levelIndex("info"), nil
	initial := make(logBatch, 200)
	for i := range initial {
		initial[i].entry = core.LogEntry{Type: "debug", Payload: "hidden"}
		if i%2 == 0 {
			initial[i].entry.Type, initial[i].entry.Payload = "info", "visible"
		}
	}
	m.Update(initial)
	m.scroll(-1_000)
	if want := 100 - 18; m.logOffset != want {
		t.Fatalf("oldest offset = %d, want %d", m.logOffset, want)
	}
	m.Update(logBatch{
		{entry: core.LogEntry{Type: "debug", Payload: "hidden"}},
		{entry: core.LogEntry{Type: "info", Payload: "visible"}},
		{offline: true},
	})
	if m.logOffset != m.maxLogOffset() || len(m.logEntries) != 200 || m.logsLive {
		t.Fatalf("after rotation: offset=%d entries=%d live=%t", m.logOffset, len(m.logEntries), m.logsLive)
	}
	m.Update(tea.WindowSizeMsg{Width: m.width, Height: 120})
	if m.logOffset != 0 {
		t.Fatalf("offset after growing viewport = %d", m.logOffset)
	}
}

func TestLogBatchFeedsDNSWhileLogsPaused(t *testing.T) {
	m := richModel()
	m.logPaused, m.logEntries = true, nil
	now := time.Now()
	m.Update(logBatch{
		{entry: core.LogEntry{Time: now, DNS: &core.DNSEvent{Domain: "batch.example", Type: "A", Started: true}}},
		{entry: core.LogEntry{Time: now.Add(time.Millisecond), DNS: &core.DNSEvent{Domain: "batch.example", Type: "A", Result: "192.0.2.1"}}},
	})
	if len(m.logEntries) != 0 || len(m.visibleDNSRows()) != 1 {
		t.Fatal("paused logs should still process DNS events in a batch")
	}
}

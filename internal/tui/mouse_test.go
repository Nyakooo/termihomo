package tui

import (
	"fmt"
	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/charmbracelet/x/ansi"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Nyakooo/termihomo/internal/i18n"
)

func findHit(m *model, kind hitKind, index int) (hit, bool) {
	for _, h := range m.hits {
		if h.kind != kind {
			continue
		}
		if h.index == index {
			return h, true
		}
	}
	return hit{}, false
}

func doClick(m *model, h hit) {
	m.Update(tea.MouseClickMsg{X: (h.x0 + h.x1) / 2, Y: h.y0, Button: tea.MouseLeft})
}

func TestMouseWheelScrollsList(t *testing.T) {
	m := richModel()
	m.page = proxies
	m.cursor = 0
	m.Update(tea.MouseWheelMsg{X: 30, Y: 5, Button: tea.MouseWheelDown})
	if m.cursor != 1 {
		t.Fatalf("wheel down cursor=%d, want 1", m.cursor)
	}
	m.Update(tea.MouseWheelMsg{X: 30, Y: 5, Button: tea.MouseWheelUp})
	if m.cursor != 0 {
		t.Fatalf("wheel up cursor=%d, want 0", m.cursor)
	}
}

func TestPasteIntoSubscriptionForm(t *testing.T) {
	m := richModel()
	m.form = "Subscription URL"
	m.Update(tea.PasteMsg{Content: "https://example.com/sub"})
	if string(m.input) != "https://example.com/sub" {
		t.Fatalf("bracketed paste input=%q", string(m.input))
	}
	m.input = nil
	m.Update(tea.PasteMsg{Content: "https://example.com/sub\r\n"})
	if string(m.input) != "https://example.com/sub" {
		t.Fatalf("paste should strip control chars, got %q", string(m.input))
	}
}

func TestPasteRendersPlaintext(t *testing.T) {
	m := richModel()
	m.form = "Subscription URL"
	m.Update(tea.PasteMsg{Content: "https://example.com/sub"})
	if !strings.Contains(m.View().Content, "https://example.com/sub") {
		t.Fatal("pasted subscription URL should render in plaintext")
	}
}

func TestCtrlVPastesClipboard(t *testing.T) {
	orig := readClipboard
	defer func() { readClipboard = orig }()
	readClipboard = func() (string, error) { return "https://example.com/sub\r\n", nil }

	m := richModel()
	m.form = "Subscription URL"
	cmd := m.formKey("ctrl+v", "")
	if cmd == nil {
		t.Fatal("ctrl+v should return a clipboard command")
	}
	m.Update(cmd())
	if string(m.input) != "https://example.com/sub" {
		t.Fatalf("ctrl+v paste input=%q, want the URL with control chars stripped", string(m.input))
	}
}

func TestCtrlVClipboardError(t *testing.T) {
	orig := readClipboard
	defer func() { readClipboard = orig }()
	readClipboard = func() (string, error) {
		return "", i18n.Message("no clipboard tool available (install wl-clipboard or xclip)")
	}

	m := richModel()
	m.form = "Subscription URL"
	cmd := m.formKey("ctrl+v", "")
	m.Update(cmd())
	if len(m.input) != 0 {
		t.Fatalf("clipboard error should not add input, got %q", string(m.input))
	}
	if m.feedback == "" {
		t.Fatal("clipboard error should surface feedback")
	}
}

func TestCtrlVIgnoredOutsideForm(t *testing.T) {
	orig := readClipboard
	defer func() { readClipboard = orig }()
	readClipboard = func() (string, error) { return "https://example.com/sub", nil }

	m := richModel()
	m.Update(clipboardMsg{formID: m.formID, text: "https://example.com/sub"})
	if len(m.input) != 0 {
		t.Fatalf("clipboard result outside a form should be ignored, got %q", string(m.input))
	}
}

func TestFormRetainsInputOnFailure(t *testing.T) {
	m := richModel()
	m.snapshot.profile.Source = ""
	m.form, m.input = "Subscription URL", []rune("https://example.com/sub")
	m.formKey("enter", "")
	if m.form != "" || !m.busy {
		t.Fatalf("submit should clear the form and start the operation, form=%q busy=%v", m.form, m.busy)
	}
	if m.retryForm != "Subscription URL" || string(m.retryInput) != "https://example.com/sub" {
		t.Fatalf("retry not stored: %q %q", m.retryForm, string(m.retryInput))
	}
	m.Update(operation{label: "Add subscription", err: i18n.Message("subscription download failed")})
	if m.form != "Subscription URL" || string(m.input) != "https://example.com/sub" {
		t.Fatalf("failure should restore the form and input, form=%q input=%q", m.form, string(m.input))
	}
	if m.retryForm != "" || m.retryInput != nil {
		t.Fatal("retry state should be cleared after handling")
	}
}

func TestFormSuccessClearsRetry(t *testing.T) {
	m := richModel()
	m.snapshot.profile.Source = ""
	m.form, m.input = "Subscription URL", []rune("https://example.com/sub")
	m.formKey("enter", "")
	m.Update(operation{label: "Add subscription"})
	if m.form != "" || m.retryForm != "" {
		t.Fatalf("success should keep the form closed and clear retry, form=%q retry=%q", m.form, m.retryForm)
	}
}

func TestFormFailureShowsReason(t *testing.T) {
	m := richModel()
	m.snapshot.profile.Source = ""
	m.form, m.input = "Subscription URL", []rune("https://example.com/sub")
	m.formKey("enter", "")
	m.Update(operation{label: "Add subscription", err: i18n.Message("Mihomo configuration validation failed: unsupported rule type")})
	if m.formError == "" {
		t.Fatal("failure should store the error for the reopened form")
	}
	if !strings.Contains(m.View().Content, "unsupported") {
		t.Fatal("reopened form should render the failure reason")
	}
	m.formKey("esc", "")
	if m.formError != "" {
		t.Fatal("closing the form should clear the error")
	}
}

func clickAction(t *testing.T, m *model, key string) tea.Cmd {
	t.Helper()
	m.View()
	for _, region := range m.hits {
		if region.kind == hitAction && region.key == key {
			_, cmd := m.Update(tea.MouseClickMsg{X: region.x0, Y: region.y0, Button: tea.MouseLeft})
			return cmd
		}
	}
	t.Fatalf("missing mouse control for %q", key)
	return nil
}

func TestMouseFormCaretAndSave(t *testing.T) {
	m := richModel()
	m.page = proxies
	clickAction(t, m, m.settings.key("search"))
	m.Update(tea.PasteMsg{Content: "中国"})
	m.View()
	for _, region := range m.hits {
		if region.kind == hitInput {
			m.Update(tea.MouseClickMsg{X: region.x0 + 2, Y: region.y0, Button: tea.MouseLeft})
		}
	}
	m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	clickAction(t, m, "enter")
	if m.filter != "中a国" {
		t.Fatalf("filter = %q, want 中a国", m.filter)
	}
}

func TestMouseConfirmationBlocksUnderlyingActions(t *testing.T) {
	m := richModel()
	m.page = corePage
	clickAction(t, m, "x")
	if m.confirm == nil {
		t.Fatal("stop should require confirmation")
	}
	m.View()
	for _, region := range m.hits {
		if region.kind == hitTab || (region.kind == hitAction && region.key != "y" && region.key != "n") {
			t.Fatalf("unexpected active control behind confirmation: %+v", region)
		}
	}
	clickAction(t, m, "n")
	if m.confirm != nil || m.busy || m.page != corePage {
		t.Fatal("cancel should close the confirmation and leave the core page active")
	}
}

func TestOverviewShowsLifecycleControlsForCurrentState(t *testing.T) {
	m := richModel()
	clickAction(t, m, "a")
	if m.confirm == nil {
		t.Fatal("overview stop must ask for confirmation")
	}
	clickAction(t, m, "n")
	m.snapshot.status.State = "Stopped"
	m.View()
	for _, region := range m.hits {
		if region.kind == hitAction && region.key == "x" {
			t.Fatal("overview should hide stop once Mihomo has stopped")
		}
	}
	if cmd := clickAction(t, m, "a"); cmd == nil || !m.busy {
		t.Fatal("overview start should run the lifecycle command")
	}
}

func TestVimNavigationAndLogLevel(t *testing.T) {
	m := richModel()
	m.handleKey("tab", "")
	if m.page != proxies {
		t.Fatal("Tab should move to proxies")
	}
	for i := range 12 {
		name := fmt.Sprintf("Node %d", i)
		m.snapshot.proxies[0].All = append(m.snapshot.proxies[0].All, name)
		m.snapshot.proxies = append(m.snapshot.proxies, core.Proxy{Name: name})
	}
	columns := max(1, min(6, (m.width-m.sidebarWidth()+1)/23))
	for _, step := range []struct {
		key  string
		want int
	}{
		{"j", columns}, {"right", columns + 1}, {"h", columns}, {"up", 0},
		{"l", 1}, {"down", columns + 1}, {"k", 1}, {"left", 0},
	} {
		m.handleKey(step.key, "")
		if m.page != proxies || m.cursor != step.want {
			t.Fatalf("%s: page %d cursor %d, want %d", step.key, m.page, m.cursor, step.want)
		}
	}

	m.settings.Keys = map[string]string{"log-level": "l"}
	m.changePage(logs)
	m.handleKey("v", "")
	if m.logLevel != 1 || m.page != logs {
		t.Fatal("v should cycle log levels with legacy settings")
	}
	for _, p := range []page{dashboard, profiles, logs, corePage, connectionsPage} {
		m.changePage(p)
		for _, key := range []string{"left", "right", "up", "down", "h", "j", "k", "l"} {
			m.handleKey(key, "")
			if m.page != p {
				t.Fatalf("%s should stay on page %d", key, p)
			}
		}
	}
}

func TestOverviewModeControlsVisibleAtMinimumSize(t *testing.T) {
	m := richModel()
	m.width, m.height = 80, 24
	m.snapshot.status.State = "Stopped"
	m.View()
	for _, key := range []string{"b", "m", "d", "u", "p", "w"} {
		found := false
		for _, region := range m.hits {
			if region.kind == hitAction && region.key == key && region.y0 < m.height-3 {
				found = true
			}
		}
		if !found {
			t.Errorf("missing overview body control %q", key)
		}
	}
	clickAction(t, m, "m")
	if m.feedback == "" || m.busy {
		t.Fatal("a stopped kernel should show a prompt to start it")
	}
}

func TestProxySidebarHasNoSubmenus(t *testing.T) {
	m := richModel()
	m.width, m.height = 80, 24
	m.View()
	h, ok := findHit(m, hitTab, int(proxies))
	if !ok || h.x1 > m.sidebarWidth() {
		t.Fatal("missing standalone proxy sidebar entry")
	}
	if h.y1-h.y0 != 3 {
		t.Fatalf("main sidebar hit height = %d, want 3", h.y1-h.y0)
	}
	for _, y := range []int{h.y0, h.y1 - 1} {
		m.changePage(dashboard)
		m.View()
		m.Update(tea.MouseClickMsg{X: h.x1 - 1, Y: y, Button: tea.MouseLeft})
		if m.page != proxies {
			t.Fatalf("clicking sidebar padding at row %d did not open proxies", y)
		}
	}
	m.View()
	for _, region := range m.hits {
		if region.kind == hitTab && region.x0 >= m.sidebarWidth() {
			t.Fatal("proxy entry should have no sidebar submenus")
		}
	}
	clickAction(t, m, "f")
	if m.busy || m.speedNodes == nil {
		t.Fatal("batch button should start a background test")
	}
	m.Update(speedDone{generation: m.speedGeneration})
	clickAction(t, m, "t")
	if m.busy || m.speedNodes == nil {
		t.Fatal("single-node button should start a test")
	}
}

func TestProxyGridClickFocusesAndEnterAppliesNode(t *testing.T) {
	m := richModel()
	m.page = proxies
	m.View()
	tab, ok := findHit(m, hitProxyTab, 0)
	if !ok {
		t.Fatal("missing group tab")
	}
	doClick(m, tab)
	m.View()
	card, ok := findHit(m, hitProxyCard, 1)
	if !ok || m.proxyGroup != "Proxy" {
		t.Fatal("missing node card in selected group")
	}
	doClick(m, card)
	if m.cursor != 1 || m.busy {
		t.Fatal("click should focus B without applying it")
	}
	if cmd := m.handleKey("enter", ""); cmd == nil || !m.busy || m.speedNodes != nil || !strings.Contains(m.feedback, "B") {
		t.Fatalf("Enter should apply B without testing it: %q", m.feedback)
	}
}

func TestSpeedProgressMarksEveryPendingCard(t *testing.T) {
	m := richModel()
	m.page = proxies
	m.beginSpeedTest([]string{"A", "B"})
	screen := ansi.Strip(m.View().Content)
	if strings.Count(screen, "Testing…") != 2 || strings.Contains(screen, "Queued") {
		t.Fatal("the grid should show status on cards only")
	}
	for _, expected := range []string{"Testing…"} {
		if !strings.Contains(screen, expected) {
			t.Fatalf("missing %q during test", expected)
		}
	}
	m.Update(speedProgress{generation: m.speedGeneration, name: "A", result: &core.DelayResult{Milliseconds: 42}})
	if !strings.Contains(ansi.Strip(m.View().Content), "42 ms") || m.proxyDelay(m.snapshot.proxies[1]) != 42 {
		t.Fatal("completed node should show its delay before the batch finishes")
	}
}

func TestProxyTestShortcutTestsHighlightedNodeWithoutOpeningDetails(t *testing.T) {
	m := richModel()
	m.changePage(proxies)
	m.setProxyTab("Proxy")
	m.cursor = 1
	if cmd := m.handleKey("t", ""); cmd == nil || m.detail || !m.speedNodes["B"] {
		t.Fatal("t should test the highlighted node in the active group")
	}
}

func TestTabSkipsStandaloneGroupPage(t *testing.T) {
	m := richModel()
	m.changePage(proxies)
	m.handleKey("tab", "")
	if m.page != profiles {
		t.Fatal("Tab from proxies should open profiles")
	}
	m.handleKey("shift+tab", "")
	if m.page != proxies {
		t.Fatal("Shift+Tab from profiles should open proxies")
	}
}

func TestMainMenuNumbersFollowSidebarOrder(t *testing.T) {
	m := richModel()
	for i, dest := range []page{dashboard, proxies, profiles, connectionsPage, generalPage} {
		m.handleKey(fmt.Sprint(i+1), "")
		if m.page != dest {
			t.Fatalf("key %d opened page %d, want %d", i+1, m.page, dest)
		}
	}
	m.View()
	network, ok := findHit(m, hitTab, int(networkPage))
	if !ok || network.x0 < m.sidebarWidth() {
		t.Fatal("network tab should appear inside the tools content area")
	}
	doClick(m, network)
	if m.page != networkPage {
		t.Fatal("clicking the tools content tab should open network")
	}
	m.handleKey("shift+tab", "")
	if m.page != connectionsPage {
		t.Fatal("Shift+Tab should go to the preceding main menu")
	}
}

func TestReplacementOpensSubscriptionForm(t *testing.T) {
	m := richModel()
	m.page = profiles
	clickAction(t, m, "e")
	if m.form != "Edit profile" || !m.nameFocused {
		t.Fatal("edit should open the profile editor with name focused")
	}
}

func TestFooterActionsShareOneRowAtMinimumWidth(t *testing.T) {
	m := richModel()
	m.width, m.height = 80, 24
	m.changePage(profiles)
	m.View()
	for _, key := range []string{"r", "?", "q"} {
		found := false
		for _, region := range m.hits {
			if region.kind == hitAction && region.key == key && region.y0 == m.height-1 {
				found = true
			}
		}
		if !found {
			t.Errorf("missing %q on the shared action row", key)
		}
	}
}

func TestDashboardUpdateStaysOnDashboardAndAnimates(t *testing.T) {
	m := richModel()
	if cmd := clickAction(t, m, "profile-update"); cmd == nil || m.page != dashboard || !m.profileUpdating {
		t.Fatal("dashboard update should start without switching pages")
	}
	before := m.View().Content
	m.Update(profileSpin(m.profileSpinID))
	if m.View().Content == before {
		t.Fatal("update icon should change on animation tick")
	}
	m.Update(operation{label: "Update profile"})
	if m.profileUpdating {
		t.Fatal("update animation should stop on completion")
	}
}

func TestSubscriptionFormPreservesNameWhenSwitchingFields(t *testing.T) {
	m := richModel()
	m.openForm("Subscription URL", "https://example.org/profile")
	m.formKey("tab", "")
	m.addInput("我的订阅")
	m.formKey("tab", "")
	if m.nameFocused || string(m.input) != "https://example.org/profile" {
		t.Fatal("switching back should restore the link field")
	}
	m.formKey("enter", "")
	if m.retryName != "我的订阅" || !m.busy {
		t.Fatal("the custom name should survive until the import finishes")
	}
}

func TestDashboardTUNToggleDoesNotOpenConfirmation(t *testing.T) {
	for _, active := range []bool{false, true} {
		m := richModel()
		m.snapshot.overview.TUN = active
		m.snapshot.overview.TUNUnavailable = !active
		if cmd := clickAction(t, m, "u"); cmd == nil || m.confirm != nil || !m.busy {
			t.Fatalf("TUN active=%v should start the toggle directly", active)
		}
	}
}

func TestProfileEditShowsDefaultScheduleAndValidatesInterval(t *testing.T) {
	m := richModel()
	m.page = profiles
	clickAction(t, m, "e")
	if m.profileInterval != "1440" || m.profileSchedule.Disabled {
		t.Fatal("automatic updates should default to enabled every 1440 minutes")
	}
	clickAction(t, m, "profile-auto")
	if !m.profileSchedule.Disabled {
		t.Fatal("clicking the automatic update row should toggle it")
	}
	clickAction(t, m, "profile-interval")
	m.input, m.inputCursor = []rune("0"), 1
	m.formKey("enter", "")
	if m.formError == "" || !m.intervalFocused {
		t.Fatal("an invalid interval should keep the field open")
	}
}

func TestFormUndoAndRedoAcrossFields(t *testing.T) {
	m := richModel()
	m.openForm("Subscription URL", "https://example.org")
	m.Update(tea.PasteMsg{Content: "/sub"})
	m.formKey("tab", "")
	m.addInput("名称")
	m.formKey("tab", "")
	m.formKey("ctrl+z", "")
	if got := string(m.input); got != "https://example.org" || m.inputCursor != len(m.input) {
		t.Fatalf("undo returned %q at %d", got, m.inputCursor)
	}
	m.formKey("ctrl+shift+z", "")
	if got := string(m.input); got != "https://example.org/sub" {
		t.Fatalf("redo returned %q", got)
	}
	m.formKey("tab", "")
	m.formKey("ctrl+z", "")
	if len(m.input) != 0 {
		t.Fatal("name should have independent undo history")
	}
	m.formKey("ctrl+y", "")
	if string(m.input) != "名称" {
		t.Fatal("Ctrl+Y should redo the name edit")
	}
}

func TestConnectionSelectAllUsesCurrentFilter(t *testing.T) {
	m := richModel()
	m.changePage(connectionsPage)
	m.snapshot.connections[0].Metadata.Host = "matches.example"
	m.snapshot.connections[1].Metadata.Host = "excluded.example"
	m.filter = "matches"
	m.handleKey("ctrl+a", "")
	if len(m.connectionSelection) != 1 || !m.connectionSelection[m.snapshot.connections[0].ID] {
		t.Fatal("Ctrl+A should select only matching connections")
	}
	m.handleKey("x", "")
	if m.confirm == nil || !strings.Contains(m.confirmLabel, "1 selected") {
		t.Fatalf("missing batch confirmation: %q", m.confirmLabel)
	}
}

func TestLanguageSwitchByMousePersistsAndUpdatesView(t *testing.T) {
	m := richModel()
	m.manager.Paths.Config = filepath.Join(t.TempDir(), "config")
	m.manager.Paths.Data = filepath.Join(t.TempDir(), "data")
	m.manager.Paths.Logs = filepath.Join(m.manager.Paths.Data, "logs")
	m.lang, m.page = i18n.English, languagePage
	m.View()

	for _, h := range m.hits {
		if h.kind == hitAction && h.key == "c" {
			doClick(m, h)
			if m.lang != i18n.Chinese || i18n.Saved(m.manager.Paths.Config) != i18n.Chinese ||
				!strings.Contains(m.View().Content, "当前语言") {
				t.Fatalf("language=%s saved=%s feedback=%q", m.lang, i18n.Saved(m.manager.Paths.Config), m.feedback)
			}
			m.handleKey("e", "")
			if m.lang != i18n.English || i18n.Saved(m.manager.Paths.Config) != i18n.English {
				t.Fatal("switching back to English should update and save the preference")
			}
			return
		}
	}
	t.Fatal("missing Chinese language button")
}

func TestBatchTestDoesNotBlockEnterOrMoveProxyFocus(t *testing.T) {
	m := richModel()
	m.changePage(proxies)
	m.sortByDelay = true
	m.cursor = 1 // B follows A before it has a measured delay.
	if cmd := m.handleKey("f", ""); cmd == nil || m.busy {
		t.Fatal("batch test should run without locking node selection")
	}
	generation := m.speedGeneration
	m.Update(speedProgress{generation: generation, name: "B", result: &core.DelayResult{Milliseconds: 20}})
	if items := m.visibleProxies(); items[m.cursor].Name != "B" {
		t.Fatal("a delay update should keep the same node focused after sorting")
	}
	if cmd := m.handleKey("enter", ""); cmd == nil || !m.busy || !strings.Contains(m.feedback, "B") {
		t.Fatal("Enter should apply the focused node during batch testing")
	}
	selectionFeedback := m.feedback
	m.Update(speedDone{generation: generation})
	if m.speedNodes != nil || !m.busy || m.feedback != selectionFeedback {
		t.Fatal("test completion should leave the selection operation intact")
	}
}

func TestProxyDefaultsToSelectedNodeWithoutMovingManualFocus(t *testing.T) {
	m := richModel()
	m.snapshot.proxies[0].Now = "B"
	m.changePage(proxies)
	if m.cursor != 1 || m.visibleProxies()[m.cursor].Name != "B" {
		t.Fatal("opening the proxy page should focus the selected node")
	}
	m.handleKey("h", "")
	m.applySnapshot(m.snapshot)
	if m.cursor != 0 {
		t.Fatal("a regular refresh should preserve focus after user movement")
	}

	next := m.snapshot
	m.snapshot.proxies = nil
	m.changePage(proxies)
	m.applySnapshot(next)
	if m.cursor != 1 {
		t.Fatal("the first live snapshot should focus the selected node")
	}
}

func TestClipboardResultStaysWithOriginalForm(t *testing.T) {
	m := richModel()
	original := readClipboard
	t.Cleanup(func() { readClipboard = original })
	readClipboard = func() (string, error) { return "old form text", nil }
	m.openForm("Subscription URL", "")
	paste := m.handleKey("ctrl+v", "")
	m.handleKey("q", "q")
	m.openForm("DNS query", "")
	m.Update(paste())
	if len(m.input) != 0 {
		t.Fatalf("new form received old clipboard text: %q", m.input)
	}
}

func TestSubscriptionRetryKeepsOriginalActionAfterPageChange(t *testing.T) {
	m := richModel()
	m.changePage(profiles)
	m.openForm("Subscription URL", "https://example.com/sub")
	m.handleKey("enter", "")
	m.changePage(networkPage)
	m.toolField = core.NetworkToolFields[1]
	m.Update(operation{label: "Import profile", err: i18n.Message("subscription download failed")})
	if cmd := m.handleKey("enter", ""); cmd == nil || m.feedback != "Import profile…" {
		t.Fatalf("retry action on network page: %q", m.feedback)
	}
}

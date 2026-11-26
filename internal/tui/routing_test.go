package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/i18n"
)

func routingModel() *model {
	m := richModel()
	m.page = routingPage
	m.snapshot.routing = core.RoutingRules{ProfileID: "first", Revision: "rev1", Targets: []string{"DIRECT", "REJECT", "🔰 选择节点"}, Original: []string{"DOMAIN,original.example,DIRECT", "MATCH,DIRECT"}}
	m.routingDraft = m.snapshot.routing
	m.routingLoaded = true
	return m
}

func addRoutingRule(t *testing.T, m *model, value string) {
	t.Helper()
	m.handleKey("a", "")
	if m.routingStage != 1 {
		t.Fatal("type picker missing")
	}
	m.handleKey("enter", "")
	if m.form != "Rule value" {
		t.Fatal("value form missing")
	}
	m.Update(tea.PasteMsg{Content: value})
	m.handleKey("enter", "")
	if m.routingStage != 2 {
		t.Fatal("target picker missing")
	}
	m.cursor = 2
	m.handleKey("enter", "")
}

func TestRoutingEditorKeyboardAndReadOnly(t *testing.T) {
	m := routingModel()
	addRoutingRule(t, m, "imap.gmail.com")
	addRoutingRule(t, m, "smtp.gmail.com")
	if len(m.routingDraft.Custom) != 2 || m.routingDraft.Custom[0].Target != "🔰 选择节点" {
		t.Fatal("rules not created", m.routingDraft)
	}
	if len(m.snapshot.routing.Custom) != 0 {
		t.Fatal("draft mutated snapshot")
	}
	m.cursor = 1
	m.handleKey("alt+up", "")
	if m.routingDraft.Custom[0].Value != "smtp.gmail.com" {
		t.Fatal("priority not adjusted")
	}
	m.handleKey("space", "")
	if !m.routingDraft.Custom[0].Disabled {
		t.Fatal("toggle failed")
	}
	m.handleKey("e", "")
	m.handleKey("enter", "")
	m.input = []rune("mail.google.com")
	m.handleKey("enter", "")
	m.handleKey("enter", "")
	if m.routingDraft.Custom[0].Value != "mail.google.com" || !m.routingDraft.Custom[0].Disabled {
		t.Fatal("edit lost value or disabled state")
	}
	m.handleKey("d", "")
	if len(m.routingDraft.Custom) != 1 {
		t.Fatal("delete failed")
	}
	m.cursor = 1
	m.handleKey("e", "")
	if m.routingStage != 0 || m.routingError == "" {
		t.Fatal("original editable")
	}
	m.handleKey("d", "")
	m.handleKey("space", "")
	if !slices.Equal(m.routingDraft.Original, m.snapshot.routing.Original) {
		t.Fatal("original mutated")
	}
	m.handleKey("c", "")
	m.handleKey("enter", "")
	m.handleKey("enter", "")
	m.handleKey("enter", "")
	if len(m.routingDraft.Custom) != 2 || m.routingDraft.Custom[1].Value != "original.example" {
		t.Fatal("copy failed", m.routingDraft)
	}
	m.cursor = len(m.routingDraft.Custom) + 1
	m.handleKey("c", "")
	if m.routingStage != 0 || m.routingError == "" {
		t.Fatal("unsupported copy accepted")
	}
}

func TestRoutingRejectsDuplicateAndInvalidDraft(t *testing.T) {
	m := routingModel()
	addRoutingRule(t, m, "imap.gmail.com")
	addRoutingRule(t, m, "IMAP.gmail.com")
	if m.routingStage != 2 || len(m.routingDraft.Custom) != 1 || !strings.Contains(m.routingError, "Duplicate") {
		t.Fatal("duplicate accepted", m.routingError)
	}
	m.handleKey("backspace", "")
	m.input = []rune("bad:993")
	m.handleKey("enter", "")
	m.handleKey("enter", "")
	if m.routingError == "" || len(m.routingDraft.Custom) != 1 {
		t.Fatal("invalid accepted")
	}
	m.handleKey("backspace", "")
	m.input = []rune("smtp.gmail.com")
	m.handleKey("enter", "")
	m.cursor = 2
	m.handleKey("enter", "")
	if len(m.routingDraft.Custom) != 2 {
		t.Fatal("could not correct invalid value", m.routingError)
	}
}

func TestRoutingDraftSurvivesRefreshAndDetectsProfileSwitch(t *testing.T) {
	m := routingModel()
	addRoutingRule(t, m, "imap.gmail.com")
	next := m.snapshot
	next.routing = core.RoutingRules{ProfileID: "second", Revision: "rev2", Targets: []string{"DIRECT", "REJECT"}}
	m.applySnapshot(next)
	if len(m.routingDraft.Custom) != 1 || m.routingDraft.ProfileID != "first" {
		t.Fatal("refresh replaced dirty draft")
	}
	if !strings.Contains(m.View().Content, "Profile changed") {
		t.Fatal("stale draft not explained")
	}
	m.handleKey("ctrl+r", "")
	if m.routingDirty || m.routingDraft.ProfileID != "second" || len(m.routingDraft.Custom) != 0 {
		t.Fatal("reset did not load current profile")
	}
}

func TestRoutingMouseAndBilingualHints(t *testing.T) {
	for _, lang := range []i18n.Language{i18n.English, i18n.Chinese} {
		m := routingModel()
		m.lang = lang
		m.changePage(profiles)
		m.View()
		h, ok := findHit(m, hitTab, int(routingPage))
		if !ok {
			t.Fatal("no routing tab")
		}
		doClick(m, h)
		if m.page != routingPage {
			t.Fatal("tab did not open")
		}
		clickAction(t, m, "a")
		m.View()
		h, ok = findHit(m, hitRow, 0)
		if !ok {
			t.Fatal("no type row")
		}
		doClick(m, h)
		m.Update(tea.PasteMsg{Content: "imap.gmail.com"})
		m.handleKey("enter", "")
		m.View()
		h, ok = findHit(m, hitRow, 2)
		if !ok {
			t.Fatal("no target row")
		}
		doClick(m, h)
		if len(m.routingDraft.Custom) != 1 || m.routingDraft.Custom[0].Target != "🔰 选择节点" {
			t.Fatal("mouse selection failed")
		}
		clickAction(t, m, "space")
		if !m.routingDraft.Custom[0].Disabled {
			t.Fatal("mouse toggle failed")
		}
		text := m.View().Content
		if !strings.Contains(text, m.lang.Text("Routing rules")) || !strings.Contains(text, m.lang.Text("Custom rules first · original rules read-only")) {
			t.Fatal("missing localized instructions")
		}
		for _, key := range []string{"a", "d", "c", "ctrl+r", "ctrl+s"} {
			m.View()
			found := false
			for _, h := range m.hits {
				if h.kind == hitAction && h.key == key {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing mouse action %s (%s)", key, lang)
			}
		}
	}
}

func TestRoutingSaveFailureKeepsDraftAndShowsError(t *testing.T) {
	m := routingModel()
	addRoutingRule(t, m, "imap.gmail.com")
	m.busy = true
	m.Update(operation{routing: true, err: errors.New("validation fixture failed")})
	if m.busy || !m.routingDirty || len(m.routingDraft.Custom) != 1 || !strings.Contains(m.View().Content, "validation fixture failed") {
		t.Fatal("failed save lost draft or error")
	}
}

func TestRoutingMinimumTerminalLayouts(t *testing.T) {
	for _, lang := range []i18n.Language{i18n.English, i18n.Chinese} {
		for _, columns := range []int{64, 80, 120} {
			m := routingModel()
			m.width, m.height, m.lang, m.compactTerminal = columns, 24, lang, true
			addRoutingRule(t, m, "imap.gmail.com")
			addRoutingRule(t, m, "smtp.gmail.com")
			addRoutingRule(t, m, "mail.example.com")
			m.cursor = 1
			view := m.View().Content
			lines := strings.Split(view, "\n")
			if len(lines) != 24 {
				t.Fatalf("width %d: %d lines", columns, len(lines))
			}
			for _, line := range lines {
				if width(line) > columns {
					t.Fatalf("width %d overflow: %s", columns, line)
				}
			}
			for _, key := range []string{"e", "d", "space", "c", "alt+up", "alt+down", "ctrl+r", "ctrl+s"} {
				found := false
				for _, h := range m.hits {
					if h.kind == hitAction && h.key == key {
						found = true
					}
				}
				if !found {
					t.Fatalf("width %d language %s missing %s", columns, lang, key)
				}
			}
			m.View()
			h, ok := findHit(m, hitRow, 0)
			if !ok {
				t.Fatalf("width %d: no rule row", columns)
			}
			doClick(m, h)
			if m.form != "" || m.routingStage != 1 {
				t.Fatal("rule row not clickable")
			}
		}
	}
}

func TestRoutingToolbarFollowsSelectionAndKeepsFooterGlobal(t *testing.T) {
	m := routingModel()
	check := func(want, hidden []string) {
		t.Helper()
		m.View()
		for _, key := range want {
			found := false
			for _, h := range m.hits {
				if h.kind == hitAction && h.key == key {
					found = true
					if h.y0 >= m.height-1 {
						t.Fatalf("page action %s remains in footer", key)
					}
				}
			}
			if !found {
				t.Fatalf("missing page action %s", key)
			}
		}
		for _, h := range m.hits {
			if h.kind == hitAction && slices.Contains(hidden, h.key) {
				t.Fatalf("unavailable action shown: %s", h.key)
			}
		}
		for _, key := range []string{"r", "?", "q"} {
			found := false
			for _, h := range m.hits {
				if h.kind == hitAction && h.key == key && h.y0 == m.height-1 {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing global footer action %s", key)
			}
		}
	}
	// A supported original rule can be copied, but cannot be edited.
	check([]string{"a", "c", "ctrl+s", "ctrl+r"}, []string{"e", "d", "space", "alt+up", "alt+down"})
	m.cursor = 1 // MATCH is outside the supported editable rule types.
	check([]string{"a", "ctrl+s", "ctrl+r"}, []string{"c", "e", "d", "space", "alt+up", "alt+down"})
	addRoutingRule(t, m, "mail.example.com")
	check([]string{"a", "e", "d", "space", "c", "ctrl+s", "ctrl+r"}, []string{"alt+up", "alt+down"})
	m.handleKey("a", "")
	check([]string{"enter", "esc"}, []string{"a", "e", "d", "space", "c", "ctrl+s", "ctrl+r"})
}

func TestRoutingCopyIPOptionsAndCancelSave(t *testing.T) {
	m := routingModel()
	m.snapshot.routing.Original = []string{"IP-CIDR,192.0.2.0/24,DIRECT,no-resolve"}
	m.routingDraft = m.snapshot.routing
	m.handleKey("c", "")
	m.handleKey("enter", "")
	m.handleKey("enter", "")
	m.handleKey("enter", "")
	if len(m.routingDraft.Custom) != 1 || !m.routingDraft.Custom[0].NoResolve {
		t.Fatal("copy dropped no-resolve")
	}
	if !strings.Contains(m.View().Content, "no-resolve") {
		t.Fatal("preserved option not visible")
	}
	if cmd := m.handleKey("ctrl+s", ""); cmd == nil || m.cancelProfile == nil || !m.profileUpdating {
		t.Fatal("save did not use cancellable profile task")
	}
	defer m.cancelProfile()
	m.handleKey("esc", "")
	if !m.profileCancelling || !m.routingDirty {
		t.Fatal("cancel did not retain draft")
	}
}

package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/Nyakooo/termihomo/internal/core"
)

func TestSnapshotAppliesRulesAndConnections(t *testing.T) {
	m := &model{refreshing: true}
	m.Update(snapshot{
		rules:       []core.Rule{{Index: 0, Type: "Match", Proxy: "DIRECT"}},
		connections: []core.Connection{{ID: "11111111-1111-1111-1111-111111111111"}},
	})
	if m.refreshing || len(m.snapshot.rules) != 1 || len(m.snapshot.connections) != 1 {
		t.Fatalf("snapshot not applied: refreshing=%v rules=%d connections=%d", m.refreshing, len(m.snapshot.rules), len(m.snapshot.connections))
	}
}

func TestFormCursorInsertsInTheMiddle(t *testing.T) {
	m := richModel()
	m.openForm("Filter", "ac")
	m.formKey("left", "")
	m.formKey("", "b")
	if got := string(m.input); got != "abc" {
		t.Fatalf("input=%q, want abc", got)
	}
	m.input, m.inputCursor = []rune("ab cd"), 2
	m.formKey("ctrl+w", "")
	if got := string(m.input); got != " cd" {
		t.Fatalf("ctrl+w should only remove the word before the cursor, got %q", got)
	}
}

func TestLogLineShowsTimestampAndLevel(t *testing.T) {
	m := richModel()
	m.style = styler{p: darkPalette}
	stamp := time.Date(2026, 11, 12, 9, 42, 18, 0, time.Local)
	line := m.logLine(core.LogEntry{Time: stamp, Type: "warning", Payload: "slow upstream"})
	if !strings.Contains(line, "09:42:18") || !strings.Contains(line, "[WARNING]") || !strings.Contains(line, "slow upstream") {
		t.Fatalf("unexpected log line: %q", line)
	}
}

func TestProxiesPageFillsBodyHeight(t *testing.T) {
	m := richModel()
	m.page = proxies
	for _, height := range []int{20, 36} {
		lines := m.body(height)
		if len(lines) != height {
			t.Fatalf("height %d: got %d lines", height, len(lines))
		}
	}
}

func TestConnectionsPageFillsBodyHeight(t *testing.T) {
	m := richModel()
	m.page = connectionsPage
	for _, height := range []int{20, 36} {
		lines := m.connectionsPageView(height)
		if len(lines) != height {
			t.Fatalf("height %d: got %d lines", height, len(lines))
		}
	}
}

func TestConnectionRefreshKeepsDetailBoundToSelectedConnection(t *testing.T) {
	m := richModel()
	m.page = connectionsPage
	first := core.Connection{ID: "first"}
	first.Metadata.Host = "first.example"
	second := core.Connection{ID: "second"}
	second.Metadata.Host = "second.example"
	m.snapshot.connections = []core.Connection{first, second}
	m.handleKey("j", "")
	m.handleKey("enter", "")
	m.Update(snapshot{connections: []core.Connection{second}})
	if !m.detail || m.cursor != 0 || !strings.Contains(m.View().Content, "second.example") {
		t.Fatal("refresh should keep the same connection selected when its index changes")
	}
	clickAction(t, m, "x")
	if !strings.Contains(m.confirmLabel, "second.example") {
		t.Fatalf("wrong connection in confirmation: %q", m.confirmLabel)
	}
}

func TestBatchResultsCanBeSortedAndIdentifyFailures(t *testing.T) {
	m := richModel()
	m.page, m.width, m.sortByDelay = proxies, 88, true
	generation := m.beginSpeedTest([]string{"A", "B"})
	m.Update(speedDone{generation: generation, delays: map[string]core.DelayResult{
		"A": {Failed: true},
		"B": {Milliseconds: 45},
	}})
	items := m.visibleProxies()
	if items[0].Name != "B" || !strings.Contains(m.View().Content, "Test failed") || !strings.Contains(m.View().Content, "45 ms") {
		t.Fatal("expected the measured result to sort first and both statuses to fit")
	}
}

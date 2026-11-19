package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"testing"
	"time"
)

func TestQuitCancelsBusyImport(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c", "tray"} {
		t.Run(key, func(t *testing.T) {
			m := richModel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			m.ctx = ctx
			m.busy = true
			// Run stores this cancellation function for every operation using m.ctx.
			m.cancelOperation = cancel
			if key == "tray" {
				m.Update(trayExitMsg{})
			} else {
				m.handleKey(key, "")
			}
			if ctx.Err() == nil || !m.exitPending {
				t.Fatal("busy import must be cancelled on exit")
			}
			_, quit := m.Update(operation{})
			if quit == nil {
				t.Fatal("must exit after cancellation cleanup")
			}
		})
	}
}

func TestEscapeCancelsImportAndKeepsFormForRetry(t *testing.T) {
	m := richModel()
	m.ctx = t.Context()
	m.retryForm, m.retryInput, m.retryName = "Subscription URL", []rune("https://example.org/sub"), "My profile"
	started := make(chan struct{})
	cmd := m.profileAction("Import profile", func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	commands := cmd().(tea.BatchMsg)
	result := make(chan operation, 1)
	go func() { result <- commands[0]().(operation) }()
	<-started
	m.handleKey("esc", "")
	if !m.busy || !m.profileCancelling {
		t.Fatal("must wait for cancellation cleanup")
	}
	select {
	case done := <-result:
		m.Update(done)
	case <-time.After(time.Second):
		t.Fatal("cancelled import did not finish promptly")
	}
	if m.ctx.Err() != nil {
		t.Fatal("Escape cancelled the interface context")
	}
	if m.busy || m.cancelProfile != nil || m.form != "Subscription URL" || string(m.input) != "https://example.org/sub" || m.profileName != "My profile" || m.formError != "" {
		t.Fatal("cancel must preserve the form without an error and allow retry")
	}
	retry := m.profileAction("Import profile", func(ctx context.Context) error { return ctx.Err() })
	commands = retry().(tea.BatchMsg)
	if done := commands[0]().(operation); done.err != nil {
		t.Fatalf("retry inherited cancellation: %v", done.err)
	}
}

func TestProfileProgressIgnoresStaleAndCancelledStages(t *testing.T) {
	m := richModel()
	m.profileAction("Import profile", func(context.Context) error { return nil })
	defer m.cancelProfile()
	m.Update(profileProgress{generation: m.profileSpinID, stage: "Validating profile"})
	if m.feedback != "Validating profile…" {
		t.Fatal("current stage not displayed")
	}
	m.Update(profileProgress{generation: m.profileSpinID - 1, stage: "Downloading profile"})
	if m.feedback != "Validating profile…" {
		t.Fatal("stale stage overwrote current operation")
	}
	m.handleKey("esc", "")
	before := m.feedback
	m.Update(profileProgress{generation: m.profileSpinID, stage: "Saving profile"})
	if m.feedback != before {
		t.Fatal("queued progress overwrote cancellation")
	}
}

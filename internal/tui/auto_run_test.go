package tui

import "testing"

func TestAutoRunRestoresPreferencesWhenCoreAlreadyRunning(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		m := richModel()
		m.ctx = t.Context()
		m.proxyControllerAttempted = false
		m.busy = false
		msg := m.snapshot
		msg.status.State = "Running"
		msg.toolSettings = map[string]string{"app.auto-run": "false"}
		if enabled {
			msg.toolSettings["app.auto-run"] = "true"
		}
		_, cmd := m.Update(msg)
		if enabled && (cmd == nil || !m.busy) {
			t.Fatal("auto-run skipped preference restoration for running core")
		}
		if !enabled && m.busy {
			t.Fatal("disabled auto-run started a restoration operation")
		}
		if m.cancelProfile != nil {
			m.cancelProfile()
		}
	}
}

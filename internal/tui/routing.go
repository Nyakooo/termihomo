package tui

import (
	"context"
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/Nyakooo/termihomo/internal/core"
)

var routingTypes = []string{"DOMAIN", "DOMAIN-SUFFIX", "IP-CIDR", "DST-PORT"}

func (m *model) routingRowCount() int {
	switch m.routingStage {
	case 1:
		return len(routingTypes)
	case 2:
		return len(m.routingDraft.Targets)
	}
	return len(m.routingDraft.Custom) + len(m.routingDraft.Original)
}

func (m *model) routingPageView(height int) []string {
	title := m.lang.Text("Routing rules")
	header := []string{m.lang.Text("Profile") + " · " + safe(m.profileTitle())}
	header = append(header, wrapText(m.lang.Text("Rules apply only in Rule mode. Reconnect affected applications or services after applying; existing connections may keep the old route."), m.width-4)...)
	entries := []string{}
	switch m.routingStage {
	case 1:
		title = m.lang.Text("Select rule type")
		entries = routingTypes
		header = m.routingControlRows(header, []control{{"enter", "Select"}, {"esc", "Cancel"}})
	case 2:
		title = m.lang.Text("Select target")
		entries = m.routingDraft.Targets
		header = append(header, safe(m.routingEdit.Type+","+m.routingEdit.Value))
		header = m.routingControlRows(header, []control{{"enter", "Select"}, {"backspace", "Edit value"}, {"esc", "Cancel"}})
	default:
		header = append(header, m.lang.Text("Custom rules first · original rules read-only"))
		actions := []control{{"a", "Add"}}
		if m.cursor >= 0 && m.cursor < len(m.routingDraft.Custom) {
			actions = append(actions, control{"e", "Edit"}, control{"d", "Delete"}, control{"space", "Enable/disable"}, control{"c", "Copy"})
			if m.cursor > 0 {
				actions = append(actions, control{"alt+up", "Move up"})
			}
			if m.cursor+1 < len(m.routingDraft.Custom) {
				actions = append(actions, control{"alt+down", "Move down"})
			}
		} else {
			index := m.cursor - len(m.routingDraft.Custom)
			if index >= 0 && index < len(m.routingDraft.Original) {
				if _, err := core.ParseCustomRule(m.routingDraft.Original[index]); err == nil {
					actions = append(actions, control{"c", "Copy"})
				}
			}
		}
		actions = append(actions, control{"ctrl+s", "Save and apply"}, control{"ctrl+r", "Reset draft"})
		header = m.routingControlRows(header, actions)
		for i, r := range m.routingDraft.Custom {
			enabled := "on"
			if r.Disabled {
				enabled = "off"
			}
			text := fmt.Sprintf("%d [%s] %s,%s → %s", i+1, m.lang.Text(enabled), r.Type, r.Value, r.Target)
			if r.NoResolve {
				text += ",no-resolve"
			}
			entries = append(entries, text)
		}
		for i, r := range m.routingDraft.Original {
			entries = append(entries, fmt.Sprintf("%d [%s] %s", i+1, m.lang.Text("Original"), r))
		}
	}
	if m.routingDirty {
		header = append(header, m.lang.Text("Unsaved changes"))
	}
	if m.routingDraft.Revision != m.snapshot.routing.Revision && m.routingDirty {
		header = append(header, m.lang.Text("Profile changed. Reset draft before editing the active profile."))
	}
	problem := m.routingError
	if m.snapshot.routingErr != nil {
		problem = m.lang.Error(m.snapshot.routingErr)
	}
	if problem != "" {
		for _, line := range wrapText(safe(problem), m.width-4) {
			header = append(header, m.style.err(line))
		}
	}
	safeEntries := make([]string, len(entries))
	for i, e := range entries {
		safeEntries[i] = safe(e)
	}
	return m.listPanel(title, header, safeEntries, m.cursor, height, 0, m.width, m.lang.Text("No active profile"), hitRow)
}

func (m *model) routingKey(key string) tea.Cmd {
	if m.busy {
		return nil
	}
	if m.routingStage != 0 {
		if key == "esc" {
			m.routingStage, m.cursor = 0, 0
			m.routingError = ""
			return nil
		}
		if key == "backspace" && m.routingStage == 2 {
			m.routingStage = 0
			m.openForm("Rule value", m.routingEdit.Value)
			return nil
		}
		if key != "enter" {
			return nil
		}
		if m.routingStage == 1 {
			m.routingEdit.Type = routingTypes[min(m.cursor, len(routingTypes)-1)]
			if m.routingEdit.Type != "IP-CIDR" {
				m.routingEdit.NoResolve = false
			}
			m.routingStage = 0
			m.openForm("Rule value", m.routingEdit.Value)
			return nil
		}
		if len(m.routingDraft.Targets) == 0 {
			return nil
		}
		m.routingEdit.Target = m.routingDraft.Targets[min(m.cursor, len(m.routingDraft.Targets)-1)]
		candidate := slices.Clone(m.routingDraft.Custom)
		if m.routingIndex < 0 {
			candidate = append(candidate, m.routingEdit)
		} else {
			candidate[m.routingIndex] = m.routingEdit
		}
		if err := core.ValidateCustomRules(candidate, m.routingDraft.Targets); err != nil {
			m.routingError = m.lang.Error(err)
			return nil
		}
		m.routingDraft.Custom = candidate
		if m.routingIndex < 0 {
			m.routingIndex = len(candidate) - 1
		}
		m.routingDirty = true
		m.routingError = ""
		m.routingStage, m.cursor = 0, max(0, m.routingIndex)
		return nil
	}
	n := len(m.routingDraft.Custom)
	switch key {
	case "a":
		if m.snapshot.profile.Source == "" || !m.routingLoaded {
			m.routingError = m.lang.Text("No active profile")
			return nil
		}
		m.routingEdit = core.CustomRule{}
		m.routingIndex = -1
		m.routingStage, m.cursor = 1, 0
		m.routingError = ""
	case "enter", "e", "c":
		if m.cursor < n {
			m.routingEdit = m.routingDraft.Custom[m.cursor]
			m.routingIndex = m.cursor
			if key == "c" {
				m.routingIndex = -1
			}
		} else {
			if key != "c" {
				m.routingError = m.lang.Text("Original rules are read-only. Press c to copy.")
				return nil
			}
			i := m.cursor - n
			if i < 0 || i >= len(m.routingDraft.Original) {
				return nil
			}
			copied, err := core.ParseCustomRule(m.routingDraft.Original[i])
			if err != nil {
				m.routingError = m.lang.Error(err)
				return nil
			}
			m.routingEdit = copied
			m.routingIndex = -1
		}
		m.routingError = ""
		m.routingStage, m.cursor = 1, max(0, slices.Index(routingTypes, m.routingEdit.Type))
	case "d":
		if m.cursor < n {
			m.routingDraft.Custom = slices.Delete(m.routingDraft.Custom, m.cursor, m.cursor+1)
			m.routingDirty = true
			m.scroll(0)
		}
	case "space":
		if m.cursor < n {
			m.routingDraft.Custom[m.cursor].Disabled = !m.routingDraft.Custom[m.cursor].Disabled
			m.routingDirty = true
		}
	case "alt+up", "alt+down":
		to := m.cursor - 1
		if key == "alt+down" {
			to = m.cursor + 1
		}
		if m.cursor < n && to >= 0 && to < n {
			m.routingDraft.Custom[m.cursor], m.routingDraft.Custom[to] = m.routingDraft.Custom[to], m.routingDraft.Custom[m.cursor]
			m.cursor = to
			m.routingDirty = true
		}
	case "ctrl+s":
		draft := m.routingDraft
		draft.Custom = slices.Clone(draft.Custom)
		label := m.lang.Text("Save and apply")
		detail := m.lang.Text("Reconnect affected applications or services to use the new route. Rules apply only in Rule mode.")
		return m.profileTask(label, func(ctx context.Context) operation {
			return operation{label: label, detail: detail, routing: true, err: m.manager.SaveRoutingRules(ctx, draft)}
		})
	case "ctrl+r":
		m.routingDraft = m.snapshot.routing
		m.routingDraft.Custom = slices.Clone(m.snapshot.routing.Custom)
		m.routingDirty = false
		m.routingError = ""
		m.cursor = 0
	}
	return nil
}

// Wrap whole buttons so every action retains a visible mouse hit region.
func (m *model) routingControlRows(header []string, actions []control) []string {
	available := m.width - 4
	for len(actions) > 0 {
		count, used := 0, 0
		for _, action := range actions {
			size := width(m.controlText(action))
			if count > 0 {
				size++
			}
			if used+size > available && count > 0 {
				break
			}
			used += size
			count++
		}
		header = append(header, m.controls(actions[:count], 2, 4+len(header), available))
		actions = actions[count:]
	}
	return header
}

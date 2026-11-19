package tui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/i18n"
)

type profileProgress struct {
	generation uint64
	stage      string
}

func (m *model) profileAction(label string, run func(context.Context) error) tea.Cmd {
	return m.profileTask(label, func(ctx context.Context) operation {
		return operation{label: label, err: run(ctx)}
	})
}

func (m *model) profileTask(label string, run func(context.Context) operation) tea.Cmd {
	base := m.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithTimeout(base, 60*time.Second)
	m.cancelProfile = cancel
	m.profileCancelling = false
	m.profileStarted = time.Now()
	m.profileUpdating, m.profileFrame = true, 0
	m.profileSpinID++
	generation := m.profileSpinID
	send := m.send
	ctx = core.WithProfileProgress(ctx, func(stage string) {
		if send != nil {
			send(profileProgress{generation: generation, stage: stage})
		}
	})
	m.busy = true
	m.feedback = label + "…"
	return tea.Batch(func() tea.Msg {
		defer cancel()
		result := run(ctx)
		if errors.Is(result.err, context.DeadlineExceeded) {
			result.err = i18n.Message("Profile operation timed out; check the subscription or provider connection and retry")
		}
		return result
	}, profileSpinTick(generation))
}

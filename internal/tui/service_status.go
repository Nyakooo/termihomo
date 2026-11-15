package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/Nyakooo/termihomo/internal/core"
)

const dashboardServiceCount = 4

var serviceTargets = [...]struct{ name, icon string }{
	{"Google", "G"}, {"GitHub", "GH"}, {"YouTube", "▶"}, {"ChatGPT", "◎"},
	{"Claude", "✳"}, {"Gemini", "✦"}, {"Netflix", "N"}, {"Disney+", "D+"},
	{"Prime Video", "P"}, {"Spotify", "S"}, {"TikTok", "T"}, {"Discord", "D"}, {"Bilibili", "B"},
}

type serviceResult struct {
	generation uint64
	index      int
	status     core.ServiceStatus
}

type serviceBatchDone struct {
	generation uint64
	err        error
}

type servicePanel struct {
	targets    [len(serviceTargets)]core.ServiceStatus
	pending    [len(serviceTargets)]bool
	generation uint64
	dialog     bool
	cursor     int
	cancel     context.CancelFunc
	err        error
}

func (m *model) checkServices(index int) tea.Cmd {
	if m.snapshot.status.State != "Running" || m.snapshot.status.Proxy == "" {
		m.feedback = m.lang.Text("Start Mihomo to apply changes")
		return nil
	}
	limit := dashboardServiceCount
	if m.services.dialog || index >= dashboardServiceCount {
		limit = len(serviceTargets)
	}
	return m.checkServiceTargets(index, limit)
}

func serviceNode(s snapshot) string {
	name := s.outboundTarget()
	for range len(s.proxies) {
		index := slices.IndexFunc(s.proxies, func(p core.Proxy) bool { return p.Name == name })
		if index < 0 || s.proxies[index].Now == "" {
			return name
		}
		name = s.proxies[index].Now
	}
	return name
}

func (m *model) checkServiceTargets(index, limit int) tea.Cmd {
	if slices.Contains(m.services.pending[:], true) {
		return nil
	}
	node := serviceNode(m.snapshot)
	if node == "" {
		return nil
	}
	m.services.err = nil
	var targets []string
	var indices []int
	for i, target := range serviceTargets[:limit] {
		if index >= 0 && i != index {
			continue
		}
		m.services.pending[i] = true
		targets = append(targets, target.name)
		indices = append(indices, i)
	}
	ctx := m.testContext()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, m.services.cancel = context.WithCancel(ctx)
	generation, manager, send := m.services.generation, m.manager, m.send
	return func() tea.Msg {
		err := manager.CheckServices(ctx, node, targets, func(index int, result core.ServiceStatus) {
			if send != nil {
				send(serviceResult{generation, indices[index], result})
			}
		})
		return serviceBatchDone{generation, err}
	}
}

func (m *model) refreshServices(previous snapshot) tea.Cmd {
	current := m.snapshot
	changed := previous.status.PID != current.status.PID || previous.status.State != current.status.State ||
		previous.status.Proxy != current.status.Proxy || previous.overview.Mode != current.overview.Mode ||
		previous.profile.ID != current.profile.ID || !previous.profile.Updated.Equal(current.profile.Updated)
	if !changed && previous.proxyErr == nil && current.proxyErr == nil {
		changed = serviceNode(previous) != serviceNode(current)
	}
	if !changed {
		return nil
	}
	if m.services.cancel != nil {
		m.services.cancel()
		m.services.cancel = nil
	}
	m.services.generation++
	m.services.err = nil
	m.services.targets = [len(serviceTargets)]core.ServiceStatus{}
	m.services.pending = [len(serviceTargets)]bool{}
	if current.status.State == "Running" && current.status.Proxy != "" {
		return m.checkServiceTargets(-1, dashboardServiceCount)
	}
	return nil
}

func (m *model) serviceResultText(index int) (status, delay, color string) {
	p := m.style.p
	if m.services.pending[index] {
		return m.lang.Text("Testing…"), "…", p.warn
	}
	result := m.services.targets[index]
	key := result.Status
	if key == "" {
		key = "Not checked"
	}
	status, delay, color = m.lang.Text(key), "—", p.dim
	switch result.Status {
	case core.ServiceAvailable:
		color = p.ok
	case core.ServiceRestricted, core.ServiceUnsupported, core.ServiceDisallowedISP, core.ServiceBlocked, core.ServiceOriginalsOnly:
		color = p.warn
	case "":
	default:
		color = p.err
	}
	if result.Delay > 0 {
		delay = fmt.Sprintf("%d ms", max(1, result.Delay.Milliseconds()))
	}
	return
}

func (m *model) serviceCard(x, top, w int) []string {
	s := m.style
	available, completed, pending := 0, 0, false
	for i, result := range m.services.targets[:dashboardServiceCount] {
		pending = pending || m.services.pending[i]
		if result.Status != "" {
			completed++
		}
		if result.Status == core.ServiceAvailable {
			available++
		}
	}
	label := m.lang.Text("Not checked")
	color := s.p.dim
	switch {
	case errors.Is(m.services.err, core.ErrServiceProbeNeedsRestart):
		label, color = m.lang.Text("Restart core"), s.p.warn
	case m.services.err != nil:
		label, color = m.lang.Text("Check failed"), s.p.warn
	case pending:
		label, color = m.lang.Text("Testing…"), s.p.warn
	case completed == dashboardServiceCount && available == completed:
		label, color = m.lang.Text("All available"), s.p.ok
	case completed > available:
		label, color = fmt.Sprintf(m.lang.Text("%d issues"), completed-available), s.p.warn
	case completed > 0:
		label = fmt.Sprintf(m.lang.Text("%d/%d available"), available, dashboardServiceCount)
	}
	actions := []control{{"f", "⚡"}, {"e", "›"}}
	actionsWidth := width(m.controlText(actions[0])) + width(m.controlText(actions[1])) + 1
	if actionsWidth > w {
		actions[0].label = "ϟ"
		actionsWidth = width(m.controlText(actions[0])) + width(m.controlText(actions[1])) + 1
	}
	summary := s.fg(color, label)
	body := []string{summary}
	if actionsWidth+1+width(summary) <= w {
		body[0] = fit(summary, w-actionsWidth) + m.controls(actions, x+2+w-actionsWidth, top+1, actionsWidth)
	} else {
		body = append(body, m.controls(actions, x+2, top+2, w))
	}
	cellWidth := (w - 1) / 2
	for row := 0; row < 2; row++ {
		var cells []string
		for i := row * 2; i < row*2+2; i++ {
			_, value, color := m.serviceResultText(i)
			icon := serviceTargets[i].icon
			if cellWidth < 9 {
				value = strings.TrimSuffix(value, " ms")
			}
			if cellWidth < 6 && icon == "GH" {
				icon = "H"
			}
			gap := " "
			if width(icon)+1+width(value) > cellWidth {
				gap = ""
			}
			cells = append(cells, fit(s.dim(icon)+gap+s.fg(color, value), cellWidth))
		}
		body = append(body, strings.Join(cells, " "))
	}
	// Register the card after its controls so the two buttons take precedence.
	m.hits = append(m.hits, hit{kind: hitAction, key: "e", x0: x, x1: x + w + 4, y0: top, y1: top + len(body) + 2})
	return body
}

func (m *model) serviceDialog(height, w int, fieldRows map[int]string) []string {
	var lines []string
	if m.services.err != nil {
		for _, line := range wrapText(m.lang.Error(m.services.err), w) {
			lines = append(lines, m.style.warn(line))
		}
		height -= len(lines)
	}
	rowHeight := 2
	if height < 2*len(serviceTargets) {
		rowHeight = 1
	}
	visible := len(serviceTargets)
	if height < rowHeight*visible {
		visible = max(1, (height-1)/rowHeight)
	}
	start := max(0, m.services.cursor-visible+1)
	for i := start; i < start+visible; i++ {
		target := serviceTargets[i]
		status, delay, color := m.serviceResultText(i)
		marker := "  "
		if i == m.services.cursor {
			marker = m.style.accent("› ")
		}
		result := m.services.targets[i]
		details := ""
		if result.Node != "" {
			details = safe(result.Node)
		}
		if result.Region != "" {
			if details != "" {
				details += " · "
			}
			details += m.lang.Text("Region") + " " + result.Region
		}
		if result.HTTP != 0 {
			if details != "" {
				details += " · "
			}
			details += "HTTP " + strconv.Itoa(result.HTTP)
		}
		key := fmt.Sprintf("service-%d", i)
		fieldRows[len(lines)] = key
		line := marker + fit(target.name, 13) + m.style.fg(color, status) + "  " + delay
		if rowHeight == 1 {
			if details != "" {
				line += "  " + m.style.dim(details)
			}
			lines = append(lines, line)
		} else {
			fieldRows[len(lines)+1] = key
			lines = append(lines, line, "  "+fit(" ", 13)+m.style.dim(orDash(details)))
		}
	}
	if visible < len(serviceTargets) {
		lines = append(lines, m.style.dim(fmt.Sprintf("%d–%d / %d", start+1, start+visible, len(serviceTargets))))
	}
	return lines
}

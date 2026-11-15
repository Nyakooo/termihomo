package tui

import (
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/Nyakooo/termihomo/internal/core"
)

func (m *model) proxyControllerAvailable() bool {
	state := m.snapshot.status.State
	return state == "Running" || state == "Paused"
}

func (m *model) proxyDirectView() bool {
	return m.snapshot.status.State == "Running" && m.snapshot.overview.Mode == "direct"
}

func (m *model) proxyTabs() []core.Proxy {
	var tabs []core.Proxy
	for _, group := range m.snapshot.proxies {
		if group.IsGroup() && (m.snapshot.overview.Mode == "global" || (!m.proxyDirectView() && !group.Hidden)) {
			tabs = append(tabs, group)
		}
	}
	if len(tabs) == 0 {
		return []core.Proxy{{}}
	}
	return tabs
}

func (m *model) proxyTabIndex(tabs []core.Proxy) int {
	for i, tab := range tabs {
		if tab.Name == m.proxyGroup {
			return i
		}
	}
	return 0
}

func (m *model) setProxyTab(name string) {
	m.proxyGroup, m.filter, m.cursor = name, "", 0
	m.focusSelectedProxy()
}

func (m *model) focusSelectedProxy() {
	tabs := m.proxyTabs()
	selected := tabs[m.proxyTabIndex(tabs)].Now
	if selected == "" {
		return
	}
	for i, node := range m.visibleProxies() {
		if node.Name == selected {
			m.cursor = i
			return
		}
	}
}

func proxyGridColumns(width int) int {
	return max(1, min(6, (width+1)/23))
}

func (m *model) proxyGrid(height int) []string {
	s := m.style
	tabs := m.proxyTabs()
	active := m.proxyTabIndex(tabs)
	left := max(0, active-1)
	k := m.settings.key
	if m.proxyDirectView() {
		return []string{m.lang.Text("No groups")}
	}
	toolbar := m.controls([]control{{"f", "Test all"}, {"t", "Test node"},
		{k("search"), "Search"}, {k("sort"), "Sort"}}, 0, 3, m.width)
	tabLine := s.accent("[‹]")
	x := width(tabLine)
	m.hits = append(m.hits, hit{kind: hitAction, key: "[", x1: x, y0: 4, y1: 5})
	for i := left; i < len(tabs); i++ {
		label := tabs[i].Name
		if label == "" {
			label = m.lang.Text("All nodes")
		}
		label = clip(safe(label), max(7, min(24, (m.width-16)/2)))
		marker := "○ "
		if i == active {
			marker = "● "
			label = s.bold(s.accent(label))
		}
		text := marker + label
		if x+width(text)+1 > m.width-4 {
			break
		}
		x++
		m.hits = append(m.hits, hit{kind: hitProxyTab, index: i, x0: x, x1: x + width(text), y0: 4, y1: 5})
		tabLine += " " + text
		x += width(text)
	}
	m.hits = append(m.hits, hit{kind: hitAction, key: "]", x0: m.width - 3, x1: m.width, y0: 4, y1: 5})
	lines := []string{toolbar, fit(tabLine, m.width-3) + s.accent("[›]")}
	items := m.visibleProxies()
	caption := fmt.Sprintf("%d %s", len(items), m.lang.Text("Proxies"))
	group := tabs[active]
	if !m.proxyControllerAvailable() && len(m.snapshot.proxies) > 0 {
		caption += "  ·  " + m.lang.Text("Offline list")
	} else if group.Now != "" {
		caption += "  ·  " + m.lang.Text("Current exit") + " " + m.proxyChain(group.Name)
	}
	if m.filter != "" {
		caption += "  ·  / " + m.filter
	}
	lines = append(lines, s.dim(safe(caption)))
	if len(items) == 0 {
		return append(lines, m.listEmpty("No proxies", m.snapshot.proxyErr))
	}
	columns := proxyGridColumns(m.width)
	cardWidth := (m.width - columns + 1) / columns
	const cardHeight = 3
	visibleRows := max(1, (height-len(lines))/cardHeight)
	cursor := min(m.cursor, len(items)-1)
	first := max(0, cursor/columns-visibleRows+1) * columns
	for offset := first; offset < min(len(items), first+visibleRows*columns); offset += columns {
		var cards [][]string
		for i := offset; i < min(len(items), offset+columns); i++ {
			node := items[i]
			color, name := s.p.bd2, safe(node.Name)
			if m.proxyControllerAvailable() && node.Name == group.Now {
				name = "✓ " + name
				color = s.p.ok
			}
			if i == cursor {
				name = "› " + name
				color = s.p.acc
			}
			delay := m.nodeDelayStatus(node)
			line := fit(clip(s.dim(safe(node.Type)), cardWidth-5-width(delay)), cardWidth-4-width(delay)) + delay
			x := (i - offset) * (cardWidth + 1)
			y := 3 + len(lines)
			m.hits = append(m.hits, hit{kind: hitProxyCard, index: i, x0: x, x1: x + cardWidth, y0: y, y1: y + cardHeight})
			cards = append(cards, s.panelColor(name, []string{line}, cardWidth, color))
		}
		lines = append(lines, joinH(1, cards...)...)
	}
	return lines
}

// isVirtualProxy reports Mihomo's built-in bookkeeping entries, which are not
// user nodes and would otherwise clutter the proxy list.
func isVirtualProxy(p core.Proxy) bool {
	switch p.Type {
	case "Compatible", "Pass", "PassRule", "RejectDrop":
		return true
	}
	return false
}

func (m *model) visibleProxies() []core.Proxy {
	if m.proxyDirectView() {
		return nil
	}
	tabs := m.proxyTabs()
	group := tabs[m.proxyTabIndex(tabs)]
	matches := func(name string) bool {
		return strings.Contains(strings.ToLower(name), m.filter)
	}
	var filtered []core.Proxy
	if group.Name == "" {
		for _, item := range m.snapshot.proxies {
			if !item.IsGroup() && !isVirtualProxy(item) && matches(item.Name) {
				filtered = append(filtered, item)
			}
		}
	} else {
		byName := make(map[string]core.Proxy, len(m.snapshot.proxies))
		for _, item := range m.snapshot.proxies {
			byName[item.Name] = item
		}
		for _, name := range group.All {
			if matches(name) {
				item := byName[name]
				item.Name = name
				filtered = append(filtered, item)
			}
		}
	}
	if m.sortByDelay {
		sort.SliceStable(filtered, func(i, j int) bool {
			left, right := m.proxyDelay(filtered[i]), m.proxyDelay(filtered[j])
			return left > 0 && (right <= 0 || left < right)
		})
	}
	return filtered
}

func (m *model) proxyDelay(p core.Proxy) int {
	if !m.proxyControllerAvailable() {
		return 0
	}
	if result, ok := m.delays[p.Name]; ok {
		return result.Milliseconds
	}
	if len(p.History) == 0 {
		return 0
	}
	return p.History[len(p.History)-1].Delay
}

func (m *model) mergeProxyDelays(results map[string]core.DelayResult) {
	if len(results) == 0 {
		return
	}
	var focus string
	if m.page == proxies && m.sortByDelay {
		if items := m.visibleProxies(); len(items) > 0 {
			focus = items[min(m.cursor, len(items)-1)].Name
		}
	}
	if m.delays == nil {
		m.delays = make(map[string]core.DelayResult, len(results))
	}
	maps.Copy(m.delays, results)
	if focus != "" {
		for i, item := range m.visibleProxies() {
			if item.Name == focus {
				m.cursor = i
				break
			}
		}
	}
}

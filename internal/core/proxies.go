package core

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"go.yaml.in/yaml/v3"
)

type Proxy struct {
	Name    string
	Type    string
	Now     string
	All     []string
	Alive   bool
	Hidden  bool
	History []struct{ Delay int }
}

func (m Manager) runningInstance() (instance, error) {
	state, err := m.readInstance()
	if err != nil {
		return state, i18n.Message("no managed Mihomo instance is available")
	}
	process, err := state.openProcess()
	if err != nil {
		return state, i18n.Message("managed Mihomo is not running")
	}
	process.close()
	return state, nil
}

func (m Manager) Proxies(ctx context.Context) ([]Proxy, error) {
	state, err := m.runningInstance()
	if err != nil {
		return nil, err
	}
	return m.orderedProxies(ctx, state)
}

func (m Manager) orderedProxies(ctx context.Context, state instance) ([]Proxy, error) {
	proxies, err := state.proxies(ctx)
	if err != nil {
		return nil, err
	}
	m.proxyGroupOrder().apply(proxies)
	return proxies, nil
}

type proxyGroupOrder map[string]int

type proxyOrderCache struct {
	mu    sync.Mutex
	info  os.FileInfo
	order proxyGroupOrder
}

// WithRuntimeCache reuses proxy ordering and profile metadata across UI refreshes.
func (m Manager) WithRuntimeCache() Manager {
	m.proxyOrder = &proxyOrderCache{}
	m.profileSummaries = &profileSummaryCache{}
	return m
}

func (m Manager) proxyGroupOrder() proxyGroupOrder {
	file, err := os.Open(m.configPath())
	if err != nil {
		return nil
	}
	defer file.Close()
	cache := m.proxyOrder
	if cache != nil {
		cache.mu.Lock()
		defer cache.mu.Unlock()
	}
	info, err := file.Stat()
	if err != nil {
		return nil
	}
	if cache != nil && cache.info != nil && os.SameFile(info, cache.info) &&
		info.Size() == cache.info.Size() && info.ModTime().Equal(cache.info.ModTime()) {
		return cache.order
	}
	body, err := io.ReadAll(file)
	if err != nil {
		return nil
	}
	order := parseProxyGroupOrder(body)
	if cache != nil && order != nil {
		cache.info, cache.order = info, order
	}
	return order
}

// The controller returns a map. The generated configuration preserves group order.
func parseProxyGroupOrder(body []byte) proxyGroupOrder {
	var config struct {
		Groups []struct{ Name string } `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal(body, &config); err != nil {
		return nil
	}
	order := make(proxyGroupOrder, len(config.Groups))
	for i, group := range config.Groups {
		order[group.Name] = i + 1
	}
	return order
}

func (order proxyGroupOrder) apply(proxies []Proxy) {
	_, customGlobal := order["GLOBAL"]
	for i := range proxies {
		if proxies[i].Name == "GLOBAL" && !customGlobal {
			proxies[i].Hidden = true
		}
	}
	rank := func(name string) int {
		if index, ok := order[name]; ok {
			return index
		}
		if name == "GLOBAL" {
			return 0
		}
		return len(order) + 1
	}
	sort.SliceStable(proxies, func(i, j int) bool {
		return rank(proxies[i].Name) < rank(proxies[j].Name)
	})
}

func (state instance) proxies(ctx context.Context) ([]Proxy, error) {
	var reply struct{ Proxies map[string]Proxy }
	if err := state.request(ctx, http.MethodGet, "/proxies", nil, &reply); err != nil {
		return nil, err
	}
	proxies := make([]Proxy, 0, len(reply.Proxies))
	for name, p := range reply.Proxies {
		if name == serviceProbeGroup {
			continue
		}
		p.All = slices.DeleteFunc(p.All, func(member string) bool { return member == serviceProbeGroup })
		p.Name = name
		proxies = append(proxies, p)
	}
	sort.Slice(proxies, func(i, j int) bool { return proxies[i].Name < proxies[j].Name })
	return proxies, nil
}

func (m Manager) Select(ctx context.Context, group, choice string) error {
	return m.withLock(func() error {
		state, err := m.runningInstance()
		if err != nil {
			return err
		}
		path := "/proxies/" + url.PathEscape(group)
		var info Proxy
		if err := state.request(ctx, http.MethodGet, path, nil, &info); err != nil {
			return err
		}
		if info.Type != "Selector" {
			return i18n.Message("only Selector groups accept manual selection")
		}
		if err := state.selectProxy(ctx, group, choice); err != nil {
			return err
		}
		var applied Proxy
		if err := state.request(ctx, http.MethodGet, path, nil, &applied); err != nil {
			return err
		}
		if applied.Now != choice {
			return i18n.Message("Mihomo did not apply the selected node")
		}
		settings, _ := m.readToolSettings()
		if closeConnections, _ := settings["app.close-connections"].(bool); closeConnections {
			return state.request(ctx, http.MethodDelete, "/connections", nil, nil)
		}
		return nil
	})
}

func (m Manager) Delay(ctx context.Context, name, target string) (int, error) {
	state, err := m.runningInstance()
	if err != nil {
		return 0, err
	}
	defaultTarget, timeout := m.delaySettings()
	return state.delay(ctx, name, cmp.Or(target, defaultTarget), timeout)
}

type DelayResult struct {
	Milliseconds int
	Failed       bool
	TimedOut     bool
}

func NewDelayResult(delay int, err error) DelayResult {
	return DelayResult{Milliseconds: delay, Failed: err != nil || delay <= 0,
		TimedOut: errors.Is(err, controllerStatus(http.StatusGatewayTimeout))}
}

func (m Manager) TestNodes(ctx context.Context, names []string, target string, progress func(string, *DelayResult)) (map[string]DelayResult, error) {
	state, err := m.runningInstance()
	if err != nil {
		return nil, err
	}
	defaultTarget, timeout := m.delaySettings()
	return state.testNodes(ctx, names, cmp.Or(target, defaultTarget), timeout, progress)
}

func (state instance) testNodes(ctx context.Context, names []string, target string, timeout time.Duration, progress func(string, *DelayResult)) (map[string]DelayResult, error) {
	results := make([]DelayResult, len(names))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(8, len(names)) {
		workers.Go(func() {
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				delay, err := state.delay(ctx, names[i], target, timeout)
				if ctx.Err() != nil {
					return
				}
				results[i] = NewDelayResult(delay, err)
				if progress != nil {
					progress(names[i], &results[i])
				}
			}
		})
	}
dispatch:
	for i := range names {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	byName := make(map[string]DelayResult, len(names))
	for i, name := range names {
		byName[name] = results[i]
	}
	return byName, nil
}

func (state instance) delay(ctx context.Context, name, target string, timeout time.Duration) (int, error) {
	query := url.Values{"url": {target}, "timeout": {strconv.FormatInt(timeout.Milliseconds(), 10)}}
	var reply struct{ Delay int }
	path := "/proxies/" + url.PathEscape(name) + "/delay?" + query.Encode()
	err := state.requestWithin(ctx, timeout+2*time.Second, http.MethodGet, path, nil, &reply)
	return reply.Delay, err
}

func (state instance) restoreSelectors(ctx context.Context, choices []Proxy) error {
	for _, choice := range choices {
		if choice.Type == "Selector" && choice.Now != "" {
			if err := state.selectProxy(ctx, choice.Name, choice.Now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (state instance) selectProxy(ctx context.Context, group, choice string) error {
	body, err := json.Marshal(map[string]string{"name": choice})
	if err != nil {
		return err
	}
	return state.request(ctx, http.MethodPut, "/proxies/"+url.PathEscape(group), bytes.NewReader(body), nil)
}

// ConfiguredProxies reads the active profile without contacting Mihomo.
func (m Manager) ConfiguredProxies() ([]Proxy, error) {
	current, err := m.readProfile()
	if err != nil || len(current.YAML) == 0 {
		return nil, err
	}
	var config struct {
		Nodes []struct {
			Name string
			Type string
		} `yaml:"proxies"`
		Groups []struct {
			Name, Type string
			Hidden     bool
			Proxies    []string
		} `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal(current.YAML, &config); err != nil {
		return nil, i18n.Message("cannot read profile contents")
	}
	proxies := make([]Proxy, 0, len(config.Nodes)+len(config.Groups)+2)
	for _, group := range config.Groups {
		proxies = append(proxies, Proxy{Name: group.Name, Type: configuredProxyType(group.Type), All: group.Proxies, Hidden: group.Hidden})
	}
	for _, node := range config.Nodes {
		proxies = append(proxies, Proxy{Name: node.Name, Type: configuredProxyType(node.Type)})
	}
	proxies = append(proxies, Proxy{Name: "DIRECT", Type: "Direct"}, Proxy{Name: "REJECT", Type: "Reject"})
	return proxies, nil
}

func configuredProxyType(kind string) string {
	switch kind {
	case "select":
		return "Selector"
	case "url-test":
		return "URLTest"
	case "fallback":
		return "Fallback"
	case "load-balance":
		return "LoadBalance"
	case "relay":
		return "Relay"
	case "ss":
		return "Shadowsocks"
	case "ssr":
		return "ShadowsocksR"
	default:
		return kind
	}
}

func (p Proxy) IsGroup() bool {
	switch p.Type {
	case "Selector", "URLTest", "Fallback", "LoadBalance", "Relay":
		return true
	}
	return len(p.All) > 0
}

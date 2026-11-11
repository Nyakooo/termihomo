package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"time"

	"golang.org/x/sys/unix"
)

type Proxy struct {
	Name    string
	Type    string
	Now     string
	All     []string
	Alive   bool
	History []struct{ Delay int }
}

func (m Manager) runningInstance() (instance, error) {
	state, err := m.readInstance()
	if err != nil {
		return state, errors.New("no managed Mihomo instance is available")
	}
	fd, err := state.openProcess()
	if err != nil {
		return state, errors.New("managed Mihomo is not running")
	}
	unix.Close(fd)
	return state, nil
}

func (m Manager) Proxies(ctx context.Context) ([]Proxy, error) {
	state, err := m.runningInstance()
	if err != nil {
		return nil, err
	}
	return state.proxies(ctx)
}

func (state instance) proxies(ctx context.Context) ([]Proxy, error) {
	var reply struct{ Proxies map[string]Proxy }
	if err := state.request(ctx, http.MethodGet, "/proxies", nil, &reply); err != nil {
		return nil, err
	}
	proxies := make([]Proxy, 0, len(reply.Proxies))
	for name, p := range reply.Proxies {
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
			return errors.New("only Selector groups accept manual selection")
		}
		return state.selectProxy(ctx, group, choice)
	})
}

func (m Manager) Delay(ctx context.Context, name, target string) (int, error) {
	state, err := m.runningInstance()
	if err != nil {
		return 0, err
	}
	query := url.Values{"url": {target}, "timeout": {"3000"}}
	var reply struct{ Delay int }
	path := "/proxies/" + url.PathEscape(name) + "/delay?" + query.Encode()
	err = state.requestWithin(ctx, 5*time.Second, http.MethodGet, path, nil, &reply)
	return reply.Delay, err
}

func (state instance) selectProxy(ctx context.Context, group, choice string) error {
	body, err := json.Marshal(map[string]string{"name": choice})
	if err != nil {
		return err
	}
	return state.request(ctx, http.MethodPut, "/proxies/"+url.PathEscape(group), bytes.NewReader(body), nil)
}

package core

import (
	"context"
	"sync"
	"time"
)

// RuntimeSnapshot reads one managed process per refresh and reuses the rules
// fetched during its readiness check.
type RuntimeSnapshot struct {
	Status        Status
	Overview      Overview
	Proxies       []Proxy
	Rules         []Rule
	Connections   []Connection
	ConnectionsAt time.Time
	Err           error
	ProxyErr      error
	ConnectionErr error
}

func (m Manager) ReadRuntime(ctx context.Context) RuntimeSnapshot {
	status, state, rules, err := m.runtimeStatus(ctx)
	s := RuntimeSnapshot{Status: status, Rules: rules, Err: err}
	if err != nil {
		return s
	}
	if status.State != "Running" && status.State != "Paused" {
		s.Proxies, s.ProxyErr = m.ConfiguredProxies()
		return s
	}

	var reads sync.WaitGroup
	reads.Go(func() { s.Proxies, s.ProxyErr = m.orderedProxies(ctx, state) })
	reads.Go(func() { s.Overview, s.Err = state.overview(ctx) })
	if status.State == "Running" {
		reads.Go(func() {
			s.Connections, s.ConnectionErr = state.connections(ctx)
			s.ConnectionsAt = time.Now()
		})
	}
	reads.Wait()
	s.Overview.Connections = len(s.Connections)
	return s
}

package core

import (
	"context"
	"net/http"
)

// Rule contains the fields exposed by Mihomo's rules endpoint.
type Rule struct {
	Index   int    `json:"index"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Proxy   string `json:"proxy"`
	Size    int    `json:"size"`
}

func (m Manager) Rules(ctx context.Context) ([]Rule, error) {
	state, err := m.runningInstance()
	if err != nil {
		return nil, err
	}
	return state.rules(ctx)
}

func (state instance) rules(ctx context.Context) ([]Rule, error) {
	var reply struct {
		Rules []Rule `json:"rules"`
	}
	if err := state.request(ctx, http.MethodGet, "/rules", nil, &reply); err != nil {
		return nil, err
	}
	return reply.Rules, nil
}

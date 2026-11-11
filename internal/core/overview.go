package core

import (
	"context"
	"encoding/json"
	"net/http"
)

type Overview struct {
	Mode        string
	Connections int
}

func (m Manager) Overview(ctx context.Context) (Overview, error) {
	state, err := m.runningInstance()
	if err != nil {
		return Overview{}, err
	}
	var config struct{ Mode string }
	if err := state.request(ctx, http.MethodGet, "/configs", nil, &config); err != nil {
		return Overview{}, err
	}
	var stats struct{ Connections []json.RawMessage }
	if err := state.request(ctx, http.MethodGet, "/connections", nil, &stats); err != nil {
		return Overview{}, err
	}
	return Overview{Mode: config.Mode, Connections: len(stats.Connections)}, nil
}

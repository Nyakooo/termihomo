package core

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

type Connection struct {
	ID       string `json:"id"`
	Metadata struct {
		Network         string `json:"network"`
		Type            string `json:"type"`
		SourceIP        string `json:"sourceIP"`
		SourcePort      string `json:"sourcePort"`
		DestinationIP   string `json:"destinationIP"`
		DestinationPort string `json:"destinationPort"`
		Host            string `json:"host"`
		Process         string `json:"process"`
		ProcessPath     string `json:"processPath"`
		UID             uint32 `json:"uid"`
	} `json:"metadata"`
	Upload      int64     `json:"upload"`
	Download    int64     `json:"download"`
	Start       time.Time `json:"start"`
	Chains      []string  `json:"chains"`
	Rule        string    `json:"rule"`
	RulePayload string    `json:"rulePayload"`
}

var connectionID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (m Manager) Connections(ctx context.Context) ([]Connection, error) {
	state, err := m.runningInstance()
	if err != nil {
		return nil, err
	}
	return state.connections(ctx)
}

func (state instance) connections(ctx context.Context) ([]Connection, error) {
	var reply struct {
		Connections []Connection `json:"connections"`
	}
	if err := state.request(ctx, http.MethodGet, "/connections", nil, &reply); err != nil {
		return nil, err
	}
	sort.Slice(reply.Connections, func(i, j int) bool { return reply.Connections[i].ID < reply.Connections[j].ID })
	return reply.Connections, nil
}

func (m Manager) CloseConnection(ctx context.Context, id string) error {
	if !connectionID.MatchString(id) {
		return i18n.Message("invalid connection ID")
	}
	state, err := m.runningInstance()
	if err != nil {
		return err
	}
	return state.closeConnection(ctx, id)
}

func (m Manager) CloseConnections(ctx context.Context, ids []string) error {
	state, err := m.runningInstance()
	if err != nil {
		return err
	}
	var failures error
	for _, id := range ids {
		if ctx.Err() != nil {
			return errors.Join(failures, ctx.Err())
		}
		err := state.closeConnection(ctx, id)
		if !errors.Is(err, controllerStatus(http.StatusNotFound)) {
			failures = errors.Join(failures, err)
		}
	}
	return failures
}

func (state instance) closeConnection(ctx context.Context, id string) error {
	if !connectionID.MatchString(id) {
		return i18n.Message("invalid connection ID")
	}
	return state.request(ctx, http.MethodDelete, "/connections/"+id, nil, nil)
}

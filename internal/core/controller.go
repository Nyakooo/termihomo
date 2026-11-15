package core

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

var controllerHTTP = &http.Client{
	Transport:     &http.Transport{},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func (state instance) request(ctx context.Context, method, path string, body io.Reader, result any) error {
	return state.requestWithin(ctx, 2*time.Second, method, path, body, result)
}

type controllerStatus int

func (status controllerStatus) Error() string {
	return i18n.Errorf("Mihomo controller returned HTTP %d", int(status)).Error()
}

func (status controllerStatus) Unwrap() error {
	return i18n.Errorf("Mihomo controller returned HTTP %d", int(status))
}

func (state instance) requestWithin(ctx context.Context, timeout time.Duration, method, path string, body io.Reader, result any) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	host, _, err := net.SplitHostPort(state.Controller)
	if err != nil || host != "127.0.0.1" {
		return i18n.Message("invalid controller address")
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+state.Controller+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+state.Secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := controllerHTTP.Do(req)
	if err != nil {
		return i18n.Message("Mihomo controller unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return controllerStatus(resp.StatusCode)
	}
	if result != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(result)
	}
	return nil
}

func (m Manager) ready(ctx context.Context, state instance) error {
	_, err := state.readyRules(ctx)
	return err
}

func (state instance) readyRules(ctx context.Context) ([]Rule, error) {
	var version struct{ Version string }
	if err := state.request(ctx, http.MethodGet, "/version", nil, &version); err != nil {
		return nil, err
	}
	if version.Version != Version {
		return nil, i18n.Message("unexpected Mihomo version")
	}
	rules, err := state.rules(ctx)
	if err != nil {
		return nil, err
	}
	if len(rules) == 0 {
		return nil, i18n.Message("Mihomo rules are not ready")
	}
	if state.Proxy != "" && !state.Paused {
		dialer := net.Dialer{Timeout: time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", state.Proxy)
		if err != nil {
			return nil, i18n.Message("Mihomo proxy listener is not ready")
		}
		conn.Close()
	}
	return rules, nil
}

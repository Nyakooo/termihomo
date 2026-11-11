package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

var controllerHTTP = &http.Client{
	Transport:     &http.Transport{},
	Timeout:       2 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func (state instance) request(ctx context.Context, method, path string, body io.Reader, result any) error {
	host, _, err := net.SplitHostPort(state.Controller)
	if err != nil || host != "127.0.0.1" {
		return errors.New("invalid controller address")
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
		return errors.New("Mihomo controller unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Mihomo controller returned HTTP %d", resp.StatusCode)
	}
	if result != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(result)
	}
	return nil
}

func (m Manager) ready(ctx context.Context, state instance) error {
	var version struct{ Version string }
	if err := state.request(ctx, http.MethodGet, "/version", nil, &version); err != nil {
		return err
	}
	if version.Version != Version {
		return errors.New("unexpected Mihomo version")
	}
	var rules struct{ Rules []json.RawMessage }
	if err := state.request(ctx, http.MethodGet, "/rules", nil, &rules); err != nil {
		return err
	}
	if len(rules.Rules) == 0 {
		return errors.New("Mihomo rules are not ready")
	}
	if state.Proxy != "" {
		dialer := net.Dialer{Timeout: time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", state.Proxy)
		if err != nil {
			return errors.New("Mihomo proxy listener is not ready")
		}
		conn.Close()
	}
	return nil
}

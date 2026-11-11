package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

type ProfileInfo struct {
	Source  string    `json:"source"`
	Updated time.Time `json:"updated"`
}

type profile struct {
	ProfileInfo
	URL  string `json:"url,omitempty"`
	YAML []byte `json:"yaml"`
}

const profileLimit = 8 << 20

func (m Manager) profilePath() string { return filepath.Join(m.Paths.Config, "profile.json") }

func (m Manager) readProfile() (profile, error) {
	var current profile
	body, err := os.ReadFile(m.profilePath())
	if errors.Is(err, os.ErrNotExist) {
		return current, nil
	}
	if err != nil {
		return current, err
	}
	if err := json.Unmarshal(body, &current); err != nil {
		return current, errors.New("cannot read the saved profile")
	}
	return current, nil
}

func (m Manager) Profile() (ProfileInfo, error) {
	current, err := m.readProfile()
	return current.ProfileInfo, err
}

func (m Manager) ImportProfile(ctx context.Context, path string) error {
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return errors.New("cannot open the profile file")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("profile path must be a regular file")
	}
	body, err := readProfileBody(file)
	if err != nil {
		return err
	}
	return m.withLock(func() error {
		return m.applyProfile(ctx, profile{ProfileInfo: ProfileInfo{Source: "file"}, YAML: body})
	})
}

func (m Manager) AddSubscription(ctx context.Context, source string) error {
	return m.withLock(func() error { return m.fetchSubscription(ctx, source) })
}

func (m Manager) UpdateProfile(ctx context.Context) error {
	return m.withLock(func() error {
		current, err := m.readProfile()
		if err != nil {
			return err
		}
		if current.URL == "" {
			return errors.New("no subscription is configured")
		}
		return m.fetchSubscription(ctx, current.URL)
	})
}

func readProfileBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, profileLimit+1))
	if err != nil {
		return nil, errors.New("cannot read profile contents")
	}
	if len(body) == 0 || len(body) > profileLimit {
		return nil, errors.New("profile must be between 1 byte and 8 MiB")
	}
	return body, nil
}

func (m Manager) fetchSubscription(ctx context.Context, source string) error {
	parsed, err := url.Parse(source)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return errors.New("subscription requires a valid HTTP or HTTPS URL")
	}
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, previous []*http.Request) error {
			req.Header.Del("Referer")
			if len(previous) >= 10 || (parsed.Scheme == "https" && req.URL.Scheme != "https") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return errors.New("invalid subscription request")
	}
	req.Header.Set("User-Agent", "mihomo/"+Version+" Termihomo")
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("subscription download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("subscription server returned HTTP %d", resp.StatusCode)
	}
	body, err := readProfileBody(resp.Body)
	if err != nil {
		return err
	}
	return m.applyProfile(ctx, profile{ProfileInfo: ProfileInfo{Source: "subscription"}, URL: source, YAML: body})
}

func (m Manager) applyProfile(ctx context.Context, next profile) error {
	if _, err := os.Stat(m.binary()); errors.Is(err, os.ErrNotExist) {
		return errors.New("install Mihomo before importing a profile")
	} else if err != nil {
		return err
	}
	state, err := m.readInstance()
	switch {
	case errors.Is(err, os.ErrNotExist):
		state = instance{}
	case err != nil:
		return err
	default:
		fd, err := state.openProcess()
		if errors.Is(err, os.ErrProcessDone) {
			state = instance{}
		} else if err != nil {
			return err
		} else {
			defer unix.Close(fd)
		}
	}
	renderState := state
	if state.PID == 0 {
		renderState = instance{Controller: "127.0.0.1:9090", Proxy: "127.0.0.1:7890", Secret: "validation-only"}
	}
	candidate, err := renderConfig(next.YAML, renderState)
	if err != nil {
		return err
	}
	if err := m.validateConfig(ctx, candidate); err != nil {
		return err
	}
	next.Updated = time.Now()
	record, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if state.PID == 0 {
		return atomicWrite(m.profilePath(), record)
	}
	if err := m.ready(ctx, state); err != nil {
		return errors.New("cannot update an unhealthy Mihomo instance")
	}
	choices, err := state.proxies(ctx)
	if err != nil {
		return err
	}
	previous, err := os.ReadFile(m.configPath())
	if err != nil {
		return err
	}
	if err := atomicWrite(m.configPath()+".bak", previous); err != nil {
		return err
	}
	pending := profileChange{Previous: previous, Choices: choices, NextDigest: sha256.Sum256(record)}
	journal, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	if err := atomicWrite(m.pendingPath(), journal); err != nil {
		return err
	}
	if err := atomicWrite(m.configPath(), candidate); err != nil {
		_ = os.Remove(m.pendingPath())
		return err
	}
	if err := m.reload(ctx, state); err != nil {
		return m.failProfileChange(state, pending, err)
	}
	if err := atomicWrite(m.profilePath(), record); err != nil {
		return m.failProfileChange(state, pending, err)
	}
	m.recordLifecycle("Profile updated")
	return os.Remove(m.pendingPath())
}

func (m Manager) reload(ctx context.Context, state instance) error {
	if err := state.requestWithin(ctx, 20*time.Second, http.MethodPut, "/configs", bytes.NewBufferString("{}"), nil); err != nil {
		return err
	}
	return m.ready(ctx, state)
}

type profileChange struct {
	Previous   []byte
	Choices    []Proxy
	NextDigest [sha256.Size]byte
}

func (m Manager) pendingPath() string {
	return filepath.Join(m.Paths.Data, "profile-change.json")
}

func (m Manager) failProfileChange(state instance, pending profileChange, cause error) error {
	if err := m.restoreConfig(state, pending); err != nil {
		return fmt.Errorf("profile update failed and recovery is incomplete: %w", err)
	}
	if err := os.Remove(m.pendingPath()); err != nil {
		return fmt.Errorf("previous configuration restored, but could not clear recovery state: %w", err)
	}
	return fmt.Errorf("profile update failed; restored previous configuration: %w", cause)
}

func (m Manager) recoverPendingProfile() error {
	body, err := os.ReadFile(m.pendingPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var pending profileChange
	if err := json.Unmarshal(body, &pending); err != nil {
		return errors.New("cannot read pending profile change")
	}
	record, err := os.ReadFile(m.profilePath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if sha256.Sum256(record) != pending.NextDigest {
		state, err := m.readInstance()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := m.restoreConfig(state, pending); err != nil {
			return fmt.Errorf("recover interrupted profile change: %w", err)
		}
	}
	return os.Remove(m.pendingPath())
}

func (m Manager) restoreConfig(state instance, pending profileChange) error {
	if err := atomicWrite(m.configPath(), pending.Previous); err != nil {
		return fmt.Errorf("restore previous configuration file: %w", err)
	}
	if state.PID == 0 {
		return nil
	}
	// A failed HTTP request may still be applying the candidate. Replacing
	// the owned process makes rollback independent of that in-flight request.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := m.stop(); err != nil {
		return fmt.Errorf("profile update failed and Mihomo could not be stopped for recovery: %w", err)
	}
	if err := m.startOn(ctx, state.Proxy); err != nil {
		return fmt.Errorf("profile update failed and the previous profile could not be restarted: %w", err)
	}
	restored, err := m.runningInstance()
	if err != nil {
		return fmt.Errorf("profile update failed and the restored process is unavailable: %w", err)
	}
	for _, choice := range pending.Choices {
		if choice.Type == "Selector" && choice.Now != "" {
			if err := restored.selectProxy(ctx, choice.Name, choice.Now); err != nil {
				return fmt.Errorf("previous profile is running, but a selector choice could not be restored: %w", err)
			}
		}
	}
	if err := m.ready(ctx, restored); err != nil {
		return fmt.Errorf("profile update failed and recovery could not be verified: %w", err)
	}
	m.recordLifecycle("Previous profile restored after update failure")
	return nil
}

package core

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

type SystemProxy struct {
	Available, Enabled, Owned bool
	Revision                  string
	Exclusions                []string
	ExclusionsErr             error
}

type proxyOwnership struct {
	Before, Applied []string
}

func (m Manager) proxyOwnership() (proxyOwnership, string) {
	path := filepath.Join(m.Paths.Data, "system-proxy.json")
	var owner proxyOwnership
	if body, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(body, &owner)
	}
	return owner, path
}

func proxyRevision(values []string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(values, "\x00"))))
}

func (m Manager) SetSystemProxy(ctx context.Context, enable bool, externalRevision string) error {
	return m.withLock(func() error {
		if !enable {
			if err := m.disableSystemProxy(ctx, externalRevision); err != nil {
				return err
			}
			if err := m.saveNetworkPreference(systemProxyPreference, false); err != nil {
				return err
			}
			if state, err := m.runningInstance(); err == nil && state.ResumeSystemProxy {
				state.ResumeSystemProxy = false
				return m.writeInstance(state)
			}
			return nil
		}
		state, err := m.runningInstance()
		if err != nil {
			return err
		}
		if state.Paused {
			return i18n.Message("Start Mihomo to apply changes")
		}
		if err := m.enableSystemProxy(ctx, state.Proxy, false); err != nil {
			return err
		}
		if m.SystemProxy(ctx).Owned {
			return m.saveNetworkPreference(systemProxyPreference, true)
		}
		return nil
	})
}

func (m Manager) restoreSystemProxy() error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := m.disableSystemProxy(ctx, ""); err != nil && !errors.Is(err, os.ErrNotExist) {
		m.recordLifecycle("Could not restore system proxy")
		return i18n.Wrap("Could not restore system proxy: %s", err)
	}
	return nil
}

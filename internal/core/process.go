package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

type Status struct {
	State      string    `json:"state"`
	Installed  string    `json:"installed_version,omitempty"`
	PID        int       `json:"pid,omitempty"`
	Controller string    `json:"controller,omitempty"`
	Proxy      string    `json:"proxy,omitempty"`
	Started    time.Time `json:"started,omitempty,omitzero"`
}

type instance struct {
	PID          int
	Identity     string
	Controller   string
	ServiceProbe string
	Proxy        string
	Secret       string
	Started      time.Time
	Stopping     bool
	Paused       bool
}

func (m Manager) statePath() string { return filepath.Join(m.Paths.Data, "instance.json") }
func (m Manager) runDir() string    { return filepath.Join(m.Paths.Data, "mihomo") }
func (m Manager) configPath() string {
	return filepath.Join(m.runDir(), "config.yaml")
}

func atomicWrite(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".pending-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func (m Manager) readInstance() (instance, error) {
	var state instance
	body, err := os.ReadFile(m.statePath())
	if err == nil {
		err = json.Unmarshal(body, &state)
		if err == nil && (state.PID <= 0 || state.Identity == "" || state.Secret == "") {
			err = i18n.Message("invalid instance state")
		}
	}
	return state, err
}

func (m Manager) Status(ctx context.Context) (Status, error) {
	status, _, _, err := m.runtimeStatus(ctx)
	return status, err
}

func (m Manager) runtimeStatus(ctx context.Context) (Status, instance, []Rule, error) {
	status := Status{State: "NotInstalled"}
	if _, err := os.Stat(m.binary()); errors.Is(err, os.ErrNotExist) {
		return status, instance{}, nil, nil
	} else if err != nil {
		return status, instance{}, nil, err
	}
	status.State, status.Installed = "Stopped", Version
	state, err := m.readInstance()
	if errors.Is(err, os.ErrNotExist) {
		return status, instance{}, nil, nil
	}
	if err != nil {
		return status, instance{}, nil, err
	}
	process, err := state.openProcess()
	if errors.Is(err, os.ErrProcessDone) {
		if !state.Stopping {
			status.State = "Failed"
		}
		return status, instance{}, nil, nil
	}
	if err != nil {
		return status, instance{}, nil, err
	}
	process.close()
	status.PID, status.Controller, status.Proxy, status.Started = state.PID, state.Controller, state.Proxy, state.Started
	status.State = "Degraded"
	rules, readyErr := state.readyRules(ctx)
	if readyErr == nil {
		status.State = "Running"
		if state.Paused {
			status.State, status.Proxy = "Paused", ""
		}
	}
	return status, state, rules, nil
}

func (m Manager) Start(ctx context.Context) error {
	if err := m.Install(ctx); err != nil {
		return err
	}
	return m.withLock(func() error {
		if state, err := m.runningInstance(); err == nil {
			var current struct{ Version string }
			if err := state.request(ctx, http.MethodGet, "/version", nil, &current); err == nil &&
				current.Version != "" && current.Version != Version {
				if err := m.stop(); err != nil {
					return err
				}
				if err := m.startOnMode(ctx, state.Proxy, state.Paused); err != nil {
					return err
				}
			}
		}
		return m.start(ctx)
	})
}

func (m Manager) Stop() error {
	return m.locked(func() error {
		if err := m.stop(); err != nil {
			return err
		}
		m.restoreSystemProxy()
		return nil
	})
}

func (m Manager) Restart(ctx context.Context) error {
	if err := m.Install(ctx); err != nil {
		return err
	}
	return m.withLock(func() error {
		previous, stateErr := m.runningInstance()
		var choices []Proxy
		var overview Overview
		if stateErr == nil {
			// A degraded controller should not prevent a manual restart.
			choices, _ = previous.proxies(ctx)
			overview, _ = previous.overview(ctx)
		}
		address, err := m.configuredProxyAddress()
		if err != nil {
			return err
		}
		var previousConfig []byte
		if stateErr == nil {
			previousConfig, err = os.ReadFile(m.configPath())
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := m.checkRestartConfig(ctx, previous, address); err != nil {
				return err
			}
		}
		owner, _ := m.proxyOwnership()
		proxyOwned := false
		if len(owner.Applied) > 0 {
			proxyOwned = m.SystemProxy(ctx).Owned
		}
		if err := m.stop(); err != nil {
			return err
		}
		if err := m.startOnMode(ctx, address, previous.Paused && stateErr == nil); err != nil {
			if previousConfig != nil {
				recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
				defer cancel()
				restoreErr := atomicWrite(m.configPath(), previousConfig)
				if restoreErr == nil {
					previous.Started, previous.Stopping = time.Now(), false
					restoreErr = m.launchPrepared(recovery, previous)
					if restoreErr == nil {
						restored, readErr := m.runningInstance()
						if readErr != nil {
							return errors.Join(err, readErr)
						}
						if runtimeErr := m.restoreRuntime(recovery, restored, choices, overview); runtimeErr != nil {
							return errors.Join(err, i18n.Wrap("Previous Mihomo process is running, but runtime settings could not be restored: %s", runtimeErr))
						}
						return i18n.Wrap("Restart failed; restored the previous Mihomo configuration: %s", err)
					}
				}
				err = errors.Join(err, i18n.Wrap("Could not restart the previous Mihomo configuration: %s", restoreErr))
			}
			if proxyOwned {
				m.restoreSystemProxy()
			}
			return err
		}
		state, err := m.runningInstance()
		if err != nil {
			if proxyOwned {
				m.restoreSystemProxy()
			}
			return err
		}

		var proxyErr error
		if proxyOwned && state.Proxy != previous.Proxy && m.SystemProxy(ctx).Owned {
			proxyErr = m.enableSystemProxy(ctx, state.Proxy, true)
			if proxyErr != nil {
				m.restoreSystemProxy()
			}
		}
		return errors.Join(proxyErr, m.restoreRuntime(ctx, state, choices, overview))
	})
}

func (m Manager) checkRestartConfig(ctx context.Context, previous instance, address string) error {
	if address != previous.Proxy {
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			return err
		}
		defer listener.Close()
	}
	previous.Proxy = address
	current, err := m.readProfile()
	if err != nil {
		return err
	}
	candidate, err := m.renderConfig(current.YAML, previous)
	if err != nil {
		return err
	}
	return m.validateConfig(ctx, candidate)
}

func (m Manager) restoreRuntime(ctx context.Context, state instance, choices []Proxy, overview Overview) error {
	var modeErr, lanErr, tunErr error
	selectorErr := state.restoreAvailableSelectors(ctx, choices)
	if overview.Mode != "" {
		modeErr = state.updateRuntime(ctx, map[string]any{"mode": overview.Mode}, func(actual Overview) bool {
			return actual.Mode == overview.Mode
		})
	}
	if !state.Paused && overview.LAN {
		lanErr = state.setLAN(ctx, true)
	}
	if !state.Paused && overview.TUNConfigured {
		tunErr = m.setTUN(ctx, state, true)
	}
	return errors.Join(selectorErr, modeErr, lanErr, tunErr)
}

func (m Manager) withLock(fn func() error) error {
	return m.locked(func() error {
		if err := m.recoverPendingProfile(); err != nil {
			return err
		}
		return fn()
	})
}

func (m Manager) locked(fn func() error) error {
	if err := m.Paths.Ensure(); err != nil {
		return err
	}
	lock, err := m.Paths.LockInstance()
	if err != nil {
		return err
	}
	defer lock.Close()
	return fn()
}

func (m Manager) start(ctx context.Context) error {
	return m.startOn(ctx, "")
}

func (m Manager) startOn(ctx context.Context, proxyAddress string) error {
	return m.startOnMode(ctx, proxyAddress, false)
}

const DefaultProxyPort = 7890

func (m Manager) configuredProxyAddress() (string, error) {
	settings, err := m.readToolSettings()
	if err != nil {
		return "", err
	}
	port := DefaultProxyPort
	if configured, ok := settings["mixed-port"].(int); ok && configured > 0 && configured <= 65535 {
		port = configured
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), nil
}

func (m Manager) startOnMode(ctx context.Context, proxyAddress string, paused bool) error {
	status, err := m.Status(ctx)
	if err != nil {
		return err
	}
	switch status.State {
	case "NotInstalled":
		return i18n.Message("install Mihomo first with termihomo install")
	case "Running":
		return nil
	case "Paused":
		if paused {
			return nil
		}
		state, err := m.runningInstance()
		if err != nil {
			return err
		}
		return m.setProxyPaused(ctx, state, false)
	case "Degraded":
		return i18n.Message("Mihomo is alive but its controller is unavailable; use restart")
	}

	if err := os.MkdirAll(m.runDir(), 0o700); err != nil {
		return err
	}
	if proxyAddress == "" {
		proxyAddress, err = m.configuredProxyAddress()
		if err != nil {
			return err
		}
	}
	proxy, err := net.Listen("tcp4", proxyAddress)
	if err != nil {
		return err
	}
	defer proxy.Close()
	reservation, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer reservation.Close()
	controller := reservation.Addr().String()
	probe, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer probe.Close()
	var key [32]byte
	rand.Read(key[:])
	state := instance{Controller: controller, ServiceProbe: probe.Addr().String(), Proxy: proxy.Addr().String(), Secret: hex.EncodeToString(key[:]), Started: time.Now(), Paused: paused}
	if err := m.prepareConfig(ctx, state); err != nil {
		return err
	}

	if err := reservation.Close(); err != nil {
		return err
	}
	if err := probe.Close(); err != nil {
		return err
	}
	if err := proxy.Close(); err != nil {
		return err
	}
	return m.launchPrepared(ctx, state)
}

func (m Manager) launchPrepared(ctx context.Context, state instance) error {
	cmd, release, err := m.startKernel()
	if err != nil {
		return err
	}
	defer release(false)
	success := false
	defer func() {
		if !success {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	state.PID = cmd.Process.Pid
	state.Identity, err = processIdentity(state.PID)
	if err != nil {
		return err
	}
	if err := m.writeInstance(state); err != nil {
		return err
	}
	if err := release(true); err != nil {
		return err
	}

	startup, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	for {
		if m.ready(startup, state) == nil {
			success = true
			m.recordLifecycle("Mihomo started")
			go func() { _ = cmd.Wait() }()
			return nil
		}
		if _, err := processIdentity(state.PID); err != nil {
			m.recordLifecycle("Mihomo exited during startup")
			return i18n.Message("Mihomo exited during startup")
		}
		select {
		case <-startup.Done():
			m.recordLifecycle("Mihomo did not become ready")
			return i18n.Message("Mihomo did not become ready")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (m Manager) writeInstance(state instance) error {
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return atomicWrite(m.statePath(), body)
}

func (m Manager) stop() error {
	state, err := m.readInstance()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	process, err := state.openProcess()
	if errors.Is(err, os.ErrProcessDone) {
		return os.Remove(m.statePath())
	}
	if err != nil {
		return err
	}
	defer process.close()
	state.Stopping = true
	if err := m.writeInstance(state); err != nil {
		return err
	}
	if err := process.stop(state); err != nil {
		return err
	}
	m.recordLifecycle("Mihomo stopped")
	return os.Remove(m.statePath())
}

func mihomoEnv() []string {
	var env []string
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "CLASH_") {
			env = append(env, item)
		}
	}
	return env
}

func (m Manager) recordLifecycle(event string) {
	file, err := os.OpenFile(filepath.Join(m.Paths.Logs, "termihomo.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	fmt.Fprintln(file, time.Now().Format(time.RFC3339), event)
}

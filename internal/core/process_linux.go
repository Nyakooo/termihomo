package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"golang.org/x/sys/unix"
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
	PID        int
	Identity   string
	Controller string
	Proxy      string
	Secret     string
	Started    time.Time
	Stopping   bool
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

func processIdentity(pid int) (string, error) {
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", err
	}
	end := strings.LastIndexByte(string(stat), ')')
	if end < 0 {
		return "", i18n.Message("invalid process record")
	}
	fields := strings.Fields(string(stat[end+1:]))
	if len(fields) < 20 || fields[0] == "Z" || fields[0] == "X" {
		return "", os.ErrProcessDone
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(boot)) + ":" + fields[19], nil
}

// Open the PID descriptor before checking identity. It binds subsequent signals
// to that process even if the numeric PID is later reused.
func (state instance) openProcess() (int, error) {
	fd, err := unix.PidfdOpen(state.PID, 0)
	if errors.Is(err, unix.ESRCH) {
		return -1, os.ErrProcessDone
	}
	if err != nil {
		return -1, err
	}
	identity, err := processIdentity(state.PID)
	if errors.Is(err, os.ErrNotExist) || (err == nil && identity != state.Identity) {
		err = os.ErrProcessDone
	}
	if err != nil {
		unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

func (m Manager) Status(ctx context.Context) (Status, error) {
	status := Status{State: "NotInstalled"}
	if _, err := os.Stat(m.binary()); errors.Is(err, os.ErrNotExist) {
		return status, nil
	} else if err != nil {
		return status, err
	}
	status.State, status.Installed = "Stopped", Version
	state, err := m.readInstance()
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	fd, err := state.openProcess()
	if errors.Is(err, os.ErrProcessDone) {
		if !state.Stopping {
			status.State = "Failed"
		}
		return status, nil
	}
	if err != nil {
		return status, err
	}
	unix.Close(fd)
	status.PID, status.Controller, status.Proxy, status.Started = state.PID, state.Controller, state.Proxy, state.Started
	status.State = "Degraded"
	if m.ready(ctx, state) == nil {
		status.State = "Running"
	}
	return status, nil
}

func (m Manager) Start(ctx context.Context) error {
	return m.withLock(func() error { return m.start(ctx) })
}

func (m Manager) Stop() error {
	return m.locked(m.stop)
}

func (m Manager) Restart(ctx context.Context) error {
	return m.withLock(func() error {
		if err := m.stop(); err != nil {
			return err
		}
		return m.start(ctx)
	})
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
	status, err := m.Status(ctx)
	if err != nil {
		return err
	}
	switch status.State {
	case "NotInstalled":
		return i18n.Message("install Mihomo first with termihomo install")
	case "Running":
		return nil
	case "Degraded":
		return i18n.Message("Mihomo is alive but its controller is unavailable; use restart")
	}

	if err := os.MkdirAll(m.runDir(), 0o700); err != nil {
		return err
	}
	useDefaultPort := proxyAddress == ""
	if useDefaultPort {
		proxyAddress = "127.0.0.1:7890"
	}
	proxy, err := net.Listen("tcp4", proxyAddress)
	if err != nil && useDefaultPort {
		proxy, err = net.Listen("tcp4", "127.0.0.1:0")
	}
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
	var key [32]byte
	rand.Read(key[:])
	state := instance{Controller: controller, Proxy: proxy.Addr().String(), Secret: hex.EncodeToString(key[:]), Started: time.Now()}
	if err := m.prepareConfig(ctx, state); err != nil {
		return err
	}

	// The child waits for an explicit byte before exec. If this command
	// exits before recording the process identity, EOF makes the child exit.
	readGate, writeGate, err := os.Pipe()
	if err != nil {
		return err
	}
	defer readGate.Close()
	defer writeGate.Close()
	cmd := exec.Command("/bin/sh", "-c", `IFS= read -r gate <&3 && [ "$gate" = start ] && exec 3<&- && exec "$@"`,
		"termihomo-start", m.binary(), "-d", m.runDir(), "-f", m.configPath())
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.ExtraFiles = []*os.File{readGate}
	cmd.Env = mihomoEnv()
	if err := reservation.Close(); err != nil {
		return err
	}
	if err := proxy.Close(); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
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
	if _, err := writeGate.WriteString("start\n"); err != nil {
		return err
	}
	writeGate.Close()
	readGate.Close()

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
	fd, err := state.openProcess()
	if errors.Is(err, os.ErrProcessDone) {
		return os.Remove(m.statePath())
	}
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	state.Stopping = true
	if err := m.writeInstance(state); err != nil {
		return err
	}
	if err := unix.PidfdSendSignal(fd, unix.SIGTERM, nil, 0); err != nil && !errors.Is(err, unix.ESRCH) {
		return err
	}
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	if count, err := unix.Poll(fds, 4000); err != nil {
		return err
	} else if count == 0 {
		if err := unix.PidfdSendSignal(fd, unix.SIGKILL, nil, 0); err != nil && !errors.Is(err, unix.ESRCH) {
			return err
		}
		if count, err := unix.Poll(fds, 2000); err != nil || count == 0 {
			return i18n.Message("Mihomo did not exit after stop")
		}
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

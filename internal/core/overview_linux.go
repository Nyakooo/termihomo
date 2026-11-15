package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"golang.org/x/sys/unix"
)

const tunDevice = "termihomo"

func isManagedTUN(name string) bool { return name == tunDevice }

func hasNetAdmin(status string) bool {
	for _, line := range strings.Split(status, "\n") {
		if value, ok := strings.CutPrefix(line, "CapEff:"); ok {
			bits, err := strconv.ParseUint(strings.TrimSpace(value), 16, 64)
			return err == nil && bits&(1<<unix.CAP_NET_ADMIN) != 0
		}
	}
	return false
}

func (state instance) checkTUNPermission() error {
	if runtime.GOOS == "android" {
		return i18n.Message("TUN requires an Android VPN app")
	}
	if _, err := os.Stat("/dev/net/tun"); err != nil {
		return i18n.Message("Linux TUN device /dev/net/tun is unavailable")
	}
	status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", state.PID))
	if err != nil {
		return err
	}
	if !hasNetAdmin(string(status)) {
		return errTUNCapability
	}
	return nil
}

var errTUNCapability = i18n.Message("Mihomo needs CAP_NET_ADMIN to enable TUN")

func (m Manager) authorizeTUN(ctx context.Context, state instance, current Overview) (instance, error) {
	agent, err := exec.LookPath("pkexec")
	if err != nil {
		return state, i18n.Message("TUN authorization requires polkit (pkexec)")
	}
	setcap, err := exec.LookPath("setcap")
	if err != nil {
		return state, i18n.Message("TUN authorization requires the setcap utility")
	}
	// Execute only the capability grant through polkit. Mihomo remains a user process.
	choices, err := state.proxies(ctx)
	if err != nil {
		return state, err
	}
	cmd := exec.CommandContext(ctx, agent, setcap, "cap_net_admin=ep", m.binary())
	if err := cmd.Run(); err != nil {
		return state, i18n.Message("TUN authorization was declined or failed")
	}
	if err := m.stop(); err != nil {
		return state, err
	}
	// Once stopped, recover the proxy even if the authorization caller disconnects.
	restart, cancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
	defer cancel()
	if err := m.startOn(restart, state.Proxy); err != nil {
		return state, err
	}
	next, err := m.runningInstance()
	if err != nil {
		return next, err
	}
	if err := next.restoreSelectors(restart, choices); err != nil {
		return next, err
	}
	if current.Mode != "" {
		if err := next.updateRuntime(restart, map[string]any{"mode": current.Mode}, func(actual Overview) bool {
			return actual.Mode == current.Mode
		}); err != nil {
			return next, err
		}
	}
	if current.LAN {
		if err := next.setLAN(restart, true); err != nil {
			return next, err
		}
	}
	return next, next.checkTUNPermission()
}

// ResidentBytes reads the operating system’s resident memory count for a process.
func ResidentBytes(pid int) int64 {
	status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(status), "\n") {
		if value, ok := strings.CutPrefix(line, "VmRSS:"); ok {
			fields := strings.Fields(value)
			if len(fields) != 2 || fields[1] != "kB" {
				return 0
			}
			kib, _ := strconv.ParseInt(fields[0], 10, 64)
			return kib * 1024
		}
	}
	return 0
}

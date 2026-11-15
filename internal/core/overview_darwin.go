package core

import (
	"context"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"golang.org/x/sys/unix"
)

const tunDevice = "utun99"

func isManagedTUN(name string) bool { return name == tunDevice }

var errTUNCapability = i18n.Message("Mihomo needs administrator privileges to enable TUN")

func (state instance) checkTUNPermission() error {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", state.PID)
	if err != nil {
		return err
	}
	if info.Eproc.Ucred.Uid != 0 {
		return errTUNCapability
	}
	return nil
}

func (m Manager) authorizeTUN(_ context.Context, state instance, _ Overview) (instance, error) {
	return state, errTUNCapability
}

func ResidentBytes(pid int) int64 {
	out, err := exec.Command("/bin/ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	kib, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	return kib * 1024
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func runMacAdmin(ctx context.Context, command string) error {
	script := `on run argv
do shell script (item 1 of argv) with administrator privileges
end run`
	out, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script, command).CombinedOutput()
	if err != nil {
		return i18n.Errorf("macOS administrator action failed: %s", strings.TrimSpace(string(out)+" "+err.Error()))
	}
	return nil
}

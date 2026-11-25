package tray

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Nyakooo/termihomo/internal/platform"
)

// HandleAutostart is invoked inside the user's desktop session, never by the
// pre-login proxy service. Wait for a late-starting StatusNotifier host.
func HandleAutostart(args []string, state func(platform.Paths) DesktopState) bool {
	if len(args) == 0 || args[0] != "--tray-autostart" {
		return false
	}
	if len(args) != 4 {
		return true
	}
	paths := platform.Paths{Config: args[1], Data: args[2], Logs: args[3]}
	if !filepath.IsAbs(paths.Config) || !filepath.IsAbs(paths.Data) || !filepath.IsAbs(paths.Logs) {
		return true
	}
	desktop := state(paths)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if desktop.RestoreNetwork != nil {
		_ = desktop.RestoreNetwork(ctx)
	}
	if waitForDesktop(ctx, desktop.Enabled, Available, time.Second) {
		if err := paths.Ensure(); err == nil {
			_ = Start(paths)
		}
	}
	return true
}

func waitForDesktop(ctx context.Context, enabled, available func() bool, interval time.Duration) bool {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if !enabled() {
			return false
		}
		if available() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

func workerEnvironment(paths platform.Paths) []string {
	env := append(os.Environ(), "GH_TOKEN=", "GITHUB_TOKEN=")
	// Linux terminal launches also use the saved XDG directories. Native Windows
	// and macOS config/data directories cannot be represented as XDG base paths.
	if runtime.GOOS == "linux" {
		env = append(env, "XDG_CONFIG_HOME="+filepath.Dir(paths.Config),
			"XDG_DATA_HOME="+filepath.Dir(paths.Data),
			"XDG_STATE_HOME="+filepath.Dir(filepath.Dir(paths.Logs)))
	}
	return env
}

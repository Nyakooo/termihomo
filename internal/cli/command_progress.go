package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/i18n"
)

// commandProgress only annotates potentially slow operations. Query output and
// JSON remain unchanged; callers report errors through the existing CLI path.
func commandProgress(ctx context.Context, args []string, out io.Writer, lang i18n.Language) (context.Context, func()) {
	var stage string
	switch args[0] {
	case "install":
		stage = "Installing Mihomo"
	case "start":
		stage = "Starting Mihomo"
	case "restart":
		stage = "Restarting Mihomo"
	case "reload":
		stage = "Reloading configuration"
	case "stop":
		stage = "Stopping Mihomo"
	case "down":
		stage = "Stopping Termihomo"
	case "config":
		if args[1] == "check" {
			stage = "Checking configuration"
		}
	case "profile":
		switch args[1] {
		case "import", "add":
			stage = "Loading profile"
		case "update":
			stage = "Updating profile"
		}
	case "proxy":
		if args[1] == "test" {
			stage = "Testing proxy"
		}
	}
	if stage == "" {
		return ctx, func() {}
	}
	var previous string
	report := func(stage string) {
		if stage == previous {
			return
		}
		previous = stage
		fmt.Fprintln(out, lang.Text(stage)+"…")
	}
	report(stage)
	return core.WithProfileProgress(ctx, report), func() { fmt.Fprintln(out, lang.Text("Done")) }
}

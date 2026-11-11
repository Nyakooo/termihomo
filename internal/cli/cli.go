package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/platform"
	"github.com/Nyakooo/termihomo/internal/tui"
	"github.com/Nyakooo/termihomo/internal/version"
	"github.com/charmbracelet/x/term"
)

const help = `Usage: termihomo [command]

Commands:
  init       Initialize private configuration, data and log directories
  install    Download and verify the supported Mihomo release
  start      Start the managed Mihomo in the background
  stop [--yes]       Stop the managed Mihomo
  restart [--yes]    Restart the managed Mihomo
  status [--json]  Show process and controller state
  profile list        Show the active profile
  profile import FILE [--yes] Import a local Mihomo YAML file
  profile add URL [--yes]     Add a subscription (use - to read the URL from stdin)
  profile update [--yes]      Refresh the saved subscription
  proxy list          List proxies and policy groups
  proxy select GROUP OPTION  Select an option in a Selector group
  proxy test NAME [URL]      Measure a proxy's delay
  logs [-f]           Show or follow Mihomo logs
  config check        Validate the managed configuration
  config backup       Back up the generated configuration
  version    Show the Termihomo version
  help       Show this help
`

func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithInput(args, strings.NewReader(""), stdout, stderr)
}

func RunWithInput(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 && isTerminal(stdin) && isTerminal(stdout) {
		paths, err := platform.ResolvePaths()
		if err == nil {
			err = tui.Run(core.Manager{Paths: paths}, stdin, stdout)
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	switch {
	case len(args) == 0 || (len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h")):
		fmt.Fprint(stdout, help)
		return 0
	case len(args) == 1 && args[0] == "version":
		fmt.Fprintf(stdout, "termihomo %s (%s/%s)\n", version.Value, runtime.GOOS, runtime.GOARCH)
		return 0
	case len(args) == 1 && args[0] == "init":
		paths, err := platform.ResolvePaths()
		if err == nil {
			err = paths.Ensure()
		}
		// Keep raw errors, paths and user arguments out of structured logs.
		log := slog.New(slog.NewJSONHandler(stderr, nil))
		if err != nil {
			log.Error("could not initialize private application directories", "operation", "init", "hint", "check HOME, XDG paths and directory permissions")
			return 1
		}
		log.Info("application directories ready", "operation", "init", "os", runtime.GOOS, "arch", runtime.GOARCH)
		return 0
	default:
		return runCore(args, stdin, stdout, stderr)
	}
}

func runCore(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	confirmed := len(args) >= 2 && args[len(args)-1] == "--yes"
	if confirmed {
		args = args[:len(args)-1]
	}
	mayConfirm := args[0] == "stop" || args[0] == "restart" ||
		(args[0] == "profile" && len(args) >= 2 && args[1] != "list")
	if !validManagedCommand(args) || (confirmed && !mayConfirm) {
		fmt.Fprintln(stderr, "unknown command or arguments; run termihomo --help")
		return 2
	}

	paths, err := platform.ResolvePaths()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	manager := core.Manager{Paths: paths}
	needsConfirmation := mayConfirm
	if args[0] == "profile" && mayConfirm && args[1] != "update" {
		info, err := manager.Profile()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		needsConfirmation = info.Source != ""
	}
	if needsConfirmation && !confirmed {
		if !isTerminal(stdin) {
			fmt.Fprintln(stderr, "confirmation required; use --yes for non-interactive execution")
			return 2
		}
		fmt.Fprint(stderr, "Apply this change? [y/N] ")
		answer, _ := bufio.NewReader(stdin).ReadString('\n')
		if !strings.EqualFold(strings.TrimSpace(answer), "y") {
			return 0
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch args[0] {
	case "install":
		err = manager.Install(ctx)
		if err == nil {
			fmt.Fprintln(stdout, "Mihomo", core.Version, "installed")
		}
	case "start":
		err = manager.Start(ctx)
	case "stop":
		err = manager.Stop()
	case "restart":
		err = manager.Restart(ctx)
	case "logs":
		err = runLogs(ctx, manager, len(args) == 2, stdout, stderr)
	case "config":
		if args[1] == "check" {
			err = manager.CheckConfig(ctx)
		} else {
			var backup string
			backup, err = manager.BackupConfig()
			if err == nil {
				fmt.Fprintln(stdout, backup)
			}
		}
	case "profile":
		err = runProfile(ctx, manager, args[1:], stdin, stdout)
	case "proxy":
		err = runProxy(ctx, manager, args[1:], stdout)
	case "status":
		var status core.Status
		status, err = manager.Status(ctx)
		if err == nil {
			if len(args) == 2 {
				err = json.NewEncoder(stdout).Encode(status)
			} else {
				fmt.Fprint(stdout, status.State)
				if status.Installed != "" {
					fmt.Fprint(stdout, " · Mihomo ", status.Installed)
				}
				if status.PID != 0 {
					fmt.Fprintf(stdout, " · PID %d · %s", status.PID, status.Controller)
					fmt.Fprint(stdout, " · Proxy ", status.Proxy)
				}
				fmt.Fprintln(stdout)
			}
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func validManagedCommand(args []string) bool {
	switch args[0] {
	case "install", "start", "stop", "restart":
		return len(args) == 1
	case "status":
		return len(args) == 1 || (len(args) == 2 && args[1] == "--json")
	case "logs":
		return len(args) == 1 || (len(args) == 2 && args[1] == "-f")
	case "config":
		return len(args) == 2 && (args[1] == "check" || args[1] == "backup")
	case "profile":
		if len(args) < 2 {
			return false
		}
		switch args[1] {
		case "list", "update":
			return len(args) == 2
		case "import", "add":
			return len(args) == 3
		}
	case "proxy":
		if len(args) < 2 {
			return false
		}
		switch args[1] {
		case "list":
			return len(args) == 2
		case "select":
			return len(args) == 4
		case "test":
			return len(args) == 3 || len(args) == 4
		}
	}
	return false
}

func runProfile(ctx context.Context, manager core.Manager, args []string, stdin io.Reader, stdout io.Writer) error {
	switch args[0] {
	case "import":
		return manager.ImportProfile(ctx, args[1])
	case "add":
		source := args[1]
		if source == "-" {
			const limit = 16 << 10
			body, err := io.ReadAll(io.LimitReader(stdin, limit+1))
			if err != nil || len(body) > limit {
				return fmt.Errorf("could not read subscription URL from stdin")
			}
			source = strings.TrimSpace(string(body))
		}
		return manager.AddSubscription(ctx, source)
	case "update":
		return manager.UpdateProfile(ctx)
	}
	info, err := manager.Profile()
	if err != nil {
		return err
	}
	if info.Source == "" {
		fmt.Fprintln(stdout, "No profile configured")
	} else {
		fmt.Fprintf(stdout, "Active profile · %s · updated %s\n", info.Source, info.Updated.Format("2006-01-02 15:04:05"))
	}
	return nil
}

func runProxy(ctx context.Context, manager core.Manager, args []string, stdout io.Writer) error {
	switch args[0] {
	case "select":
		return manager.Select(ctx, args[1], args[2])
	case "test":
		target := "https://www.gstatic.com/generate_204"
		if len(args) == 3 {
			target = args[2]
		}
		delay, err := manager.Delay(ctx, args[1], target)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%d ms\n", delay)
	default:
		proxies, err := manager.Proxies(ctx)
		if err != nil {
			return err
		}
		for _, p := range proxies {
			fmt.Fprintf(stdout, "%q\t%s", p.Name, p.Type)
			if p.Now != "" {
				fmt.Fprintf(stdout, "\t%q", p.Now)
			}
			fmt.Fprintln(stdout)
		}
	}
	return nil
}

func runLogs(ctx context.Context, manager core.Manager, follow bool, stdout, stderr io.Writer) error {
	lines, err := manager.RecentLogs()
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Fprintln(stdout, "[termihomo]", line)
	}
	if !follow {
		return nil
	}
	for ctx.Err() == nil {
		err := manager.StreamLogs(ctx, "info", func(entry core.LogEntry) {
			fmt.Fprintf(stdout, "[mihomo %s] %s\n", entry.Type, entry.Payload)
		})
		if ctx.Err() != nil {
			break
		}
		fmt.Fprintln(stderr, err, "· retrying")
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
	return nil
}

func isTerminal(stream any) bool {
	file, ok := stream.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(file.Fd())
}

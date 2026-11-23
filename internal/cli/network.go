package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Nyakooo/termihomo/internal/core"
	"github.com/Nyakooo/termihomo/internal/i18n"
	"github.com/Nyakooo/termihomo/internal/tray"
)

func runDown(ctx context.Context, manager core.Manager) error {
	if err := manager.Paths.Ensure(); err != nil {
		return err
	}
	shutdown, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// Stop the core even if an interface fails to close.
	interfaceErr := tray.Shutdown(shutdown, manager.Paths)
	coreErr := manager.Stop()
	if interfaceErr == nil {
		return coreErr
	}
	if coreErr == nil {
		return interfaceErr
	}
	return errors.Join(interfaceErr, coreErr)
}

func runSystemProxy(ctx context.Context, manager core.Manager, args []string, confirmed bool, stdin io.Reader, stdout, stderr io.Writer, lang i18n.Language) error {
	proxy := manager.SystemProxy(ctx)
	if args[0] == "status" {
		return writeSystemProxyStatus(proxy, len(args) == 2, stdout, lang)
	}
	if !proxy.Available {
		return i18n.Message("System proxy settings unavailable")
	}
	enable := args[0] == "start"
	revision := ""
	if proxy.Enabled && !proxy.Owned {
		if enable {
			return i18n.Message("System proxy is managed by another application")
		}
		if !confirmed {
			if !isTerminal(stdin) && !isMSYSTerminal(stdin) {
				return i18n.Message("confirmation required; use --yes for non-interactive execution")
			}
			fmt.Fprint(stderr, lang.Text("Turn off the system proxy configured outside Termihomo")+"? [y/N] ")
			answer, _ := bufio.NewReader(stdin).ReadString('\n')
			if !strings.EqualFold(strings.TrimSpace(answer), "y") {
				return nil
			}
		}
		revision = proxy.Revision
	}
	if enable {
		status, err := manager.Status(ctx)
		if err != nil {
			return err
		}
		if status.State != "Running" {
			return i18n.Message("Start Mihomo to apply changes")
		}
	}
	if err := manager.SetSystemProxy(ctx, enable, revision); err != nil {
		return err
	}
	actual := manager.SystemProxy(ctx)
	if !actual.Available {
		return i18n.Message("System proxy settings unavailable")
	}
	if enable && (!actual.Enabled || !actual.Owned) {
		return i18n.Message("System proxy changed; check its current settings and try again")
	}
	return writeSystemProxyStatus(actual, false, stdout, lang)
}

func runTUN(ctx context.Context, manager core.Manager, args []string, stdout io.Writer, lang i18n.Language) error {
	if args[0] != "status" {
		enable := args[0] == "start"
		if err := manager.SetTUN(ctx, enable); err != nil {
			return err
		}
		message := "TUN disabled"
		if enable {
			message = "TUN enabled"
		}
		fmt.Fprintln(stdout, lang.Text(message))
		return nil
	}
	status, err := manager.Status(ctx)
	if err != nil {
		return err
	}
	var overview core.Overview
	if status.State == "Running" || status.State == "Paused" {
		overview, err = manager.Overview(ctx)
		if err != nil {
			return err
		}
	}
	result := struct {
		State      string `json:"state"`
		Available  bool   `json:"available"`
		Enabled    bool   `json:"enabled"`
		Configured bool   `json:"configured"`
		Paused     bool   `json:"paused"`
	}{status.State, (status.State == "Running" || status.State == "Paused") && !overview.TUNUnavailable,
		overview.TUN, overview.TUNConfigured, status.ResumeTUN && !overview.TUN}
	if len(args) == 2 {
		return json.NewEncoder(stdout).Encode(result)
	}
	state := "off"
	switch {
	case result.Enabled:
		state = "on"
	case result.Paused:
		state = "paused"
	case result.Configured:
		state = "inactive"
	case !result.Available:
		state = "Unavailable"
	}
	fmt.Fprintln(stdout, "TUN · "+lang.Text(state))
	return nil
}

func runMode(ctx context.Context, manager core.Manager, args []string, stdout io.Writer, lang i18n.Language) error {
	if args[0] != "status" {
		if err := manager.SetMode(ctx, args[0]); err != nil {
			return err
		}
	}
	overview, err := manager.Overview(ctx)
	if err != nil {
		return err
	}
	if len(args) == 2 {
		return json.NewEncoder(stdout).Encode(struct {
			Mode string `json:"mode"`
		}{overview.Mode})
	}
	fmt.Fprintln(stdout, lang.Text("Routing mode")+" · "+lang.Text(overview.Mode))
	return nil
}

func writeSystemProxyStatus(proxy core.SystemProxy, jsonOutput bool, stdout io.Writer, lang i18n.Language) error {
	status := struct {
		Available bool `json:"available"`
		Enabled   bool `json:"enabled"`
		Owned     bool `json:"owned"`
	}{proxy.Available, proxy.Enabled, proxy.Owned}
	if jsonOutput {
		return json.NewEncoder(stdout).Encode(status)
	}
	state := "Unavailable"
	if proxy.Available {
		state = "off"
		if proxy.Enabled {
			state = "on"
			if !proxy.Owned {
				state = "on (external)"
			}
		}
	}
	fmt.Fprintln(stdout, lang.Text("System proxy")+" · "+lang.Text(state))
	return nil
}

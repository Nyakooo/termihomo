//go:build (linux && !android) || windows || (darwin && cgo)

package tray

import (
	"context"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/Nyakooo/termihomo/internal/i18n"
	"github.com/Nyakooo/termihomo/internal/platform"
)

// HandleWorker runs the platform menu loop on the initial OS thread.
func HandleWorker(args []string, state func(platform.Paths) DesktopState) bool {
	if len(args) != 2 || args[0] != workerArg {
		// The tray library locks the startup thread in init.
		runtime.UnlockOSThread()
		return false
	}
	defer cleanupWorker()
	if !filepath.IsAbs(args[1]) {
		return true
	}
	paths, err := platform.ResolvePaths()
	if err != nil {
		return true
	}
	lock, err := paths.LockTray()
	if err != nil {
		return true
	}
	defer lock.Close()
	desktop := state(paths)
	exitToken := InterfaceToken(paths, ExitInterface)
	if !desktop.Enabled() {
		return true
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	systray.Run(func() {
		go watchDesktopSession(ctx)
		systray.SetTooltip("Termihomo")
		title := systray.AddMenuItem("Termihomo", "")
		systray.AddSeparator()
		var items [6]*systray.MenuItem
		for i := range items {
			items[i] = systray.AddMenuItem("—", "")
			if i > 0 {
				items[i].Disable()
			}
		}
		systray.AddSeparator()
		quit := systray.AddMenuItem(i18n.Saved(paths.Config).Text("Quit"), "")
		feedback := systray.AddMenuItem("—", "")
		feedback.Disable()
		feedback.Hide()
		refresh, test := make(chan struct{}, 1), make(chan struct{}, 1)
		var nodeMenu sync.Mutex
		requestRefresh := func() {
			select {
			case refresh <- struct{}{}:
			default:
			}
		}
		go updateSummaries(ctx, items, desktop.Read, refresh, test, requestRefresh, &nodeMenu)
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			apply := func(action func(context.Context) error) bool {
				actionCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
				defer cancel()
				if err := action(actionCtx); err != nil {
					lang := i18n.Saved(paths.Config)
					feedback.SetTitle(lang.Text("Error · ") + lang.Error(err))
					feedback.Show()
					return false
				}
				feedback.Hide()
				return true
			}
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if !desktop.Enabled() || InterfaceToken(paths, ExitInterface) != exitToken {
						systray.Quit()
						return
					}
				case <-items[1].ClickedCh:
					apply(desktop.CycleMode)
					requestRefresh()
				case <-items[3].ClickedCh:
					apply(desktop.ToggleTUN)
					requestRefresh()
				case <-items[4].ClickedCh:
					apply(desktop.ToggleLAN)
					requestRefresh()
				case <-items[2].ClickedCh:
					apply(desktop.ToggleSystemProxy)
					requestRefresh()
				case <-items[5].ClickedCh:
					feedback.Hide()
					select {
					case test <- struct{}{}:
					default:
					}
				case <-quit.ClickedCh:
					if apply(func(ctx context.Context) error {
						if err := desktop.Pause(ctx); err != nil {
							return err
						}
						return requestInterfaceAction(paths, ExitInterface)
					}) {
						systray.Quit()
						return
					}
				}
			}
		}()
		go func() {
			open := func(nodes bool) {
				item := title
				if nodes {
					item = items[0]
				}
				alreadyOpen, err := paths.TrayInterfaceOpen()
				if err == nil && alreadyOpen && nodes {
					err = requestInterfaceAction(paths, OpenNodes)
				}
				if err == nil && !alreadyOpen {
					var flags []string
					if nodes {
						flags = append(flags, NodesArg)
					}
					err = openTerminal(args[1], flags...)
					if err == nil {
						err = waitForInterface(ctx, paths)
					}
				}
				lang := i18n.Saved(paths.Config)
				if nodes {
					nodeMenu.Lock()
					defer nodeMenu.Unlock()
				}
				switch {
				case err != nil:
					item.SetTooltip(lang.Error(err))
				case alreadyOpen && !nodes:
					item.SetTooltip(lang.Text("Termihomo interface is already open"))
				default:
					item.SetTooltip("")
				}
			}
			for {
				select {
				case <-ctx.Done():
					return
				case <-title.ClickedCh:
					open(false)
				case <-items[0].ClickedCh:
					open(true)
				}
			}
		}()
	}, cancel)
	return true
}

func waitForInterface(ctx context.Context, paths platform.Paths) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if open, err := paths.TrayInterfaceOpen(); open || err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return i18n.Message("Termihomo interface did not open")
		case <-ticker.C:
		}
	}
}

func updateSummaries(ctx context.Context, items [6]*systray.MenuItem, read ReadSummary, refresh, test <-chan struct{}, requestRefresh func(), nodeMenu *sync.Mutex) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var previous Summary
	first := true
	testNow := false
	for {
		summary := read(ctx, testNow, requestRefresh)
		testNow = false
		if ctx.Err() != nil {
			return
		}
		if first || summary.Running != previous.Running {
			for _, item := range []*systray.MenuItem{items[1], items[3], items[4]} {
				if summary.Running {
					item.Enable()
				} else {
					item.Disable()
				}
			}
		}
		if first || summary.CanToggleSystemProxy != previous.CanToggleSystemProxy {
			if summary.CanToggleSystemProxy {
				items[2].Enable()
			} else {
				items[2].Disable()
			}
		}
		if first || summary.CanTest != previous.CanTest {
			if summary.CanTest {
				items[5].Enable()
			} else {
				items[5].Disable()
			}
		}
		if first || summary.Active != previous.Active {
			setIcon(summary.Active)
		}
		for i, item := range items {
			if first || summary.Lines[i] != previous.Lines[i] {
				if i == 0 {
					nodeMenu.Lock()
				}
				item.SetTitle(summary.Lines[i])
				if i == 0 {
					nodeMenu.Unlock()
				}
			}
		}
		previous, first = summary, false
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-refresh:
		case <-test:
			testNow = true
		}
	}
}

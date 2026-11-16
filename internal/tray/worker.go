//go:build (linux && !android) || windows || (darwin && cgo)

package tray

import (
	"encoding/json"
	"os"
	"runtime"

	"fyne.io/systray"
)

// HandleWorker runs the platform menu loop on the initial OS thread.
func HandleWorker(args []string) bool {
	if len(args) != 1 || args[0] != workerArg {
		// The tray library locks the startup thread in init.
		runtime.UnlockOSThread()
		return false
	}

	systray.Run(func() {
		setIcon()
		systray.SetTooltip("Termihomo")
		systray.AddMenuItem("Termihomo", "").Disable()
		systray.AddSeparator()
		var items [4]*systray.MenuItem
		for i := range items {
			items[i] = systray.AddMenuItem("—", "")
			items[i].Disable()
		}
		go readSummaries(items)
	}, nil)
	return true
}

func readSummaries(items [4]*systray.MenuItem) {
	decoder := json.NewDecoder(os.Stdin)
	for {
		var summary Summary
		if err := decoder.Decode(&summary); err != nil {
			break
		}
		for i, item := range items {
			item.SetTitle(summary[i])
		}
	}
	systray.Quit()
}

//go:build linux || darwin

package tui

import (
	"context"
	"os/exec"
	"runtime"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

// readClipboard returns the current system clipboard text. It is a package var
// so tests can substitute a deterministic source without a real clipboard.
var readClipboard = func() (string, error) {
	tools := [][]string{{"pbpaste"}}
	switch runtime.GOOS {
	case "android":
		tools = [][]string{{"termux-clipboard-get"}}
	case "linux":
		tools = [][]string{
			{"wl-paste", "--no-newline"},
			{"xclip", "-selection", "clipboard", "-o"},
			{"xsel", "--clipboard", "--output"},
		}
	}
	found := false
	for _, tool := range tools {
		path, err := exec.LookPath(tool[0])
		if err != nil {
			continue
		}
		found = true
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		out, err := exec.CommandContext(ctx, path, tool[1:]...).Output()
		cancel()
		if err == nil {
			return string(out), nil
		}
	}
	if !found {
		if runtime.GOOS == "linux" {
			return "", i18n.Message("no clipboard tool available (install wl-clipboard or xclip)")
		}
		return "", i18n.Message("System clipboard is unavailable")
	}
	return "", nil
}

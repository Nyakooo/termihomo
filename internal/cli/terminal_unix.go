//go:build linux || darwin

package cli

import (
	"io"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

func isMSYSTerminal(any) bool { return false }

func bridgeTerminal([]string, io.Reader, io.Writer, io.Writer, i18n.Language) (bool, int) {
	return false, 0
}

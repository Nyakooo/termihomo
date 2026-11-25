package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"github.com/Nyakooo/termihomo/internal/update"
)

func updateProgress(out io.Writer, lang i18n.Language, interactive bool) (func(update.Progress), func()) {
	var stage string
	var lastDraw time.Time
	width := 0
	finish := func() {
		if width > 0 {
			fmt.Fprintln(out)
			width = 0
		}
	}
	report := func(p update.Progress) {
		changed := p.Stage != stage
		if changed {
			finish()
			stage = p.Stage
			fmt.Fprintln(out, lang.Text(stage)+"…")
			lastDraw = time.Time{}
		}
		if !interactive || p.Stage != "Downloading update" || p.Downloaded == 0 {
			return
		}
		if !changed && time.Since(lastDraw) < 100*time.Millisecond && (p.Total <= 0 || p.Downloaded < p.Total) {
			return
		}
		lastDraw = time.Now()
		line := fmt.Sprintf("%s · %.1f MiB", lang.Text(p.Stage), float64(p.Downloaded)/(1<<20))
		if p.Total > 0 {
			percent := min(int(p.Downloaded*100/p.Total), 100)
			filled := percent / 5
			line = fmt.Sprintf("[%s%s] %3d%% · %.1f / %.1f MiB", strings.Repeat("#", filled), strings.Repeat("-", 20-filled), percent, float64(p.Downloaded)/(1<<20), float64(p.Total)/(1<<20))
		}
		padding := max(width-len(line), 0)
		fmt.Fprintf(out, "\r%s%s", line, strings.Repeat(" ", padding))
		width = len(line)
	}
	return report, finish
}

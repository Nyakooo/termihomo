package tui

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

// palette holds the approved modern-TUI colors as hex strings.
type palette struct {
	bd, bd2            string
	fg, dim, faint     string
	acc, ok, warn, err string
	info, up, down     string
}

var darkPalette = palette{
	bd: "#35424c", bd2: "#596a77", fg: "#edf2fa", dim: "#aeb9c9", faint: "#8693a7",
	acc: "#e8aa73", ok: "#91d39b", warn: "#ebce80", err: "#f18b94",
	info: "#7ec6c9", up: "#d4a5eb", down: "#7acbcc",
}

type styler struct {
	mode int
	p    palette
}

func newStyler() styler {
	return styler{mode: detectColorMode(), p: darkPalette}
}

func detectColorMode() int {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return 0
	}
	term := os.Getenv("TERM")
	if term == "dumb" || term == "" {
		return 0
	}
	ct := os.Getenv("COLORTERM")
	if ct == "truecolor" || ct == "24bit" {
		return 3
	}
	if strings.Contains(term, "256") {
		return 2
	}
	if strings.Contains(term, "color") || strings.HasPrefix(term, "xterm") || strings.HasPrefix(term, "screen") || strings.HasPrefix(term, "tmux") || strings.HasPrefix(term, "rxvt") {
		return 3
	}
	return 1
}

func (s *styler) setColorProfile(profile colorprofile.Profile) {
	s.mode = 0
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled || os.Getenv("TERM") == "dumb" {
		return
	}
	switch profile {
	case colorprofile.TrueColor:
		s.mode = 3
	case colorprofile.ANSI256:
		s.mode = 2
	case colorprofile.ANSI:
		s.mode = 1
	}
}

func hexRGB(hex string) (r, g, b int) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 255, 255, 255
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 255, 255, 255
	}
	return int(v >> 16 & 255), int(v >> 8 & 255), int(v & 255)
}

func to256(r, g, b int) int {
	if r == g && g == b {
		if r < 8 {
			return 16
		}
		if r > 248 {
			return 231
		}
		return 232 + (r-8)/10
	}
	q := func(v int) int { return (v*5 + 127) / 255 }
	return 16 + 36*q(r) + 6*q(g) + q(b)
}

var ansi16 = [16][3]int{
	{0, 0, 0}, {128, 0, 0}, {0, 128, 0}, {128, 128, 0}, {0, 0, 128}, {128, 0, 128}, {0, 128, 128}, {192, 192, 192},
	{128, 128, 128}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0}, {0, 0, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
}

func to16(r, g, b int) int {
	best, bd := 0, 1<<30
	for i, c := range ansi16 {
		d := (r-c[0])*(r-c[0]) + (g-c[1])*(g-c[1]) + (b-c[2])*(b-c[2])
		if d < bd {
			best, bd = i, d
		}
	}
	return best
}

func (s styler) sgr(hex string) string {
	if s.mode == 0 {
		return ""
	}
	r, g, b := hexRGB(hex)
	switch s.mode {
	case 3:
		return "\x1b[38;2;" + strconv.Itoa(r) + ";" + strconv.Itoa(g) + ";" + strconv.Itoa(b) + "m"
	case 2:
		n := to256(r, g, b)
		return "\x1b[38;5;" + strconv.Itoa(n) + "m"
	default:
		n := to16(r, g, b)
		code := 30 + n
		if n >= 8 {
			code = 90 + n - 8
		}
		return "\x1b[" + strconv.Itoa(code) + "m"
	}
}

const reset = "\x1b[0m"

func (s styler) fg(hex, text string) string {
	if s.mode == 0 || text == "" {
		return text
	}
	return s.sgr(hex) + text + reset
}

func (s styler) bold(text string) string {
	if s.mode == 0 || text == "" {
		return text
	}
	return "\x1b[1m" + text + reset
}

func (s styler) caret(text string) string {
	if s.mode == 0 {
		if text == " " {
			return "█"
		}
		return "▏" + text
	}
	return "\x1b[7m" + text + reset
}

func (s styler) accent(text string) string { return s.fg(s.p.acc, text) }
func (s styler) dim(text string) string    { return s.fg(s.p.dim, text) }
func (s styler) faint(text string) string  { return s.fg(s.p.faint, text) }
func (s styler) ok(text string) string     { return s.fg(s.p.ok, text) }
func (s styler) warn(text string) string   { return s.fg(s.p.warn, text) }
func (s styler) err(text string) string    { return s.fg(s.p.err, text) }

// width returns the display width of a string, ignoring ANSI escapes.
func width(text string) int { return ansi.StringWidth(text) }

// fit pads or truncates text to exactly w display columns.
func fit(text string, w int) string {
	if w <= 0 {
		return ""
	}
	cur := width(text)
	if cur > w {
		text = ansi.Truncate(text, w, "")
		cur = width(text)
	}
	if cur < w {
		return text + strings.Repeat(" ", w-cur)
	}
	return text
}

// clip truncates text to at most w display columns, adding an ellipsis when cut.
func clip(text string, w int) string {
	if w <= 0 {
		return ""
	}
	if width(text) > w {
		return ansi.Truncate(text, w, "…")
	}
	return text
}

// panel draws a rounded box with an embedded title and returns its lines.
// outer width includes borders; content is padded by one space each side.
func (s styler) panel(title string, lines []string, w int) []string {
	return s.panelColor(title, lines, w, s.p.bd2)
}

func (s styler) panelColor(title string, lines []string, w int, border string) []string {
	w = max(4, w)
	inner := w - 4
	out := make([]string, 0, len(lines)+2)
	title = clip(title, max(0, w-6))
	top := s.fg(border, "╭─")
	if title != "" {
		top += s.bold(s.fg(border, " "+title+" "))
	}
	top += s.fg(border, strings.Repeat("─", w-width(top)-1)+"╮")
	out = append(out, top)
	side := s.fg(border, "│")
	for _, line := range lines {
		out = append(out, side+" "+fit(line, inner)+" "+side)
	}
	return append(out, s.fg(border, "╰"+strings.Repeat("─", w-2)+"╯"))
}

// joinH places columns side by side with a gap, equalizing their heights.
func joinH(gap int, cols ...[]string) []string {
	height := 0
	widths := make([]int, len(cols))
	for i, c := range cols {
		if len(c) > height {
			height = len(c)
		}
		for _, ln := range c {
			if w := width(ln); w > widths[i] {
				widths[i] = w
			}
		}
	}
	g := strings.Repeat(" ", gap)
	out := make([]string, height)
	for row := 0; row < height; row++ {
		var b strings.Builder
		for i, c := range cols {
			ln := ""
			if row < len(c) {
				ln = c[row]
			}
			b.WriteString(fit(ln, widths[i]))
			if i < len(cols)-1 {
				b.WriteString(g)
			}
		}
		out[row] = b.String()
	}
	return out
}

func (s styler) chip(text, hex string) string {
	return s.fg(hex, " "+text+" ")
}

func msHex(s styler, delay int) string {
	switch {
	case delay <= 0:
		return s.p.faint
	case delay < 150:
		return s.p.ok
	case delay < 250:
		return s.p.warn
	default:
		return s.p.err
	}
}

var sparkBlocks = []rune("▁▂▃▄▅▆▇█")

// sparkline renders a single-row block sparkline normalized to its max.
func (s styler) sparkline(values []int64, w int, hex string) string {
	if w <= 0 || len(values) == 0 {
		return ""
	}
	if len(values) > w {
		values = values[len(values)-w:]
	}
	var maxV int64 = 1
	for _, v := range values {
		if v > maxV {
			maxV = v
		}
	}
	var b strings.Builder
	for _, v := range values {
		if v < 0 {
			v = 0
		}
		level := int(float64(v) / float64(maxV) * float64(len(sparkBlocks)-1))
		b.WriteRune(sparkBlocks[level])
	}
	return s.fg(hex, b.String())
}

// areaChart packs two samples and four vertical points into each Braille cell.
func (s styler) areaChart(values []int64, w, h int, hex string) []string {
	if w < 1 || h < 1 {
		return nil
	}
	columns := w * 2
	if len(values) > columns {
		values = values[len(values)-columns:]
	}
	peak := int64(1)
	for _, value := range values {
		peak = max(peak, value)
	}
	levels := make([]int, columns)
	for i, value := range values {
		levels[columns-len(values)+i] = int(float64(max(0, value)) / float64(peak) * float64(h*4))
	}
	dots := [2][4]rune{{64, 4, 2, 1}, {128, 32, 16, 8}}
	out := make([]string, h)
	for row := range out {
		var line strings.Builder
		base := (h - row - 1) * 4
		for x := 0; x < w; x++ {
			cell := rune(0x2800)
			for col := range dots {
				for dot := 0; dot < min(4, levels[x*2+col]-base); dot++ {
					cell |= dots[col][dot]
				}
			}
			line.WriteRune(cell)
		}
		out[row] = s.fg(hex, line.String())
	}
	return out
}

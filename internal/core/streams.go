package core

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type Traffic struct {
	Up, Down           int64
	UpTotal, DownTotal int64
}

type LogEntry struct {
	Type, Payload string
}

func (m Manager) StreamTraffic(ctx context.Context, receive func(Traffic)) error {
	return stream(ctx, m, "/traffic", receive)
}

func (m Manager) StreamLogs(ctx context.Context, level string, receive func(LogEntry)) error {
	switch level {
	case "debug", "info", "warning", "error":
	default:
		return i18n.Message("unsupported log level")
	}
	clean := m.logRedactor()
	return stream(ctx, m, "/logs?level="+level, func(entry LogEntry) {
		entry.Type, entry.Payload = clean(entry.Type), clean(entry.Payload)
		receive(entry)
	})
}

// Callers own retry policy so they can show stale data between connections.
func stream[T any](ctx context.Context, m Manager, path string, receive func(T)) error {
	state, err := m.runningInstance()
	if err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(state.Controller)
	if err != nil || host != "127.0.0.1" {
		return i18n.Message("invalid controller address")
	}
	client := &http.Client{Transport: controllerHTTP.Transport, CheckRedirect: controllerHTTP.CheckRedirect}
	dial, cancel := context.WithTimeout(ctx, 3*time.Second)
	conn, _, err := websocket.Dial(dial, "ws://"+state.Controller+path, &websocket.DialOptions{
		HTTPClient: client,
		HTTPHeader: http.Header{"Authorization": {"Bearer " + state.Secret}},
	})
	cancel()
	if err != nil {
		return i18n.Message("could not connect to Mihomo event stream")
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)
	for {
		var event T
		read := ctx
		cancel := func() {}
		if path == "/traffic" {
			read, cancel = context.WithTimeout(ctx, 8*time.Second)
		}
		err := wsjson.Read(read, conn, &event)
		cancel()
		if err != nil {
			return i18n.Message("Mihomo event stream disconnected")
		}
		receive(event)
	}
}

var logURL = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+`)

func (m Manager) logRedactor() func(string) string {
	current, _ := m.readProfile()
	state, _ := m.readInstance()
	return func(text string) string {
		for _, secret := range []string{current.URL, state.Secret} {
			if secret != "" {
				text = strings.ReplaceAll(text, secret, "[redacted]")
			}
		}
		text = logURL.ReplaceAllString(text, "[url redacted]")
		return strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, text)
	}
}

func (m Manager) RecentLogs() ([]string, error) {
	file, err := os.Open(filepath.Join(m.Paths.Logs, "termihomo.log"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	const maxRead = 256 << 10
	start := max(int64(0), info.Size()-maxRead)
	if _, err := file.Seek(start, 0); err != nil {
		return nil, err
	}
	scan := bufio.NewScanner(file)
	if start > 0 {
		scan.Scan()
	}
	var lines []string
	clean := m.logRedactor()
	for scan.Scan() {
		lines = append(lines, clean(scan.Text()))
	}
	return lines[max(0, len(lines)-100):], scan.Err()
}

package core

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

type SubscriptionQuota struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
	Expire   int64 `json:"expire"`
}

const DefaultProfileInterval = 1440

type ProfileSchedule struct {
	Disabled bool `json:"disabled,omitempty"`
	Minutes  int  `json:"minutes,omitempty"`
}

func (s ProfileSchedule) Interval() int {
	if s.Minutes == 0 {
		return DefaultProfileInterval
	}
	return s.Minutes
}

func (p ProfileInfo) UpdateDue(now time.Time) bool {
	return p.Source == "subscription" && !p.Schedule.Disabled &&
		!now.Before(p.Updated.Add(time.Duration(p.Schedule.Interval())*time.Minute))
}

type ProfileInfo struct {
	Added      time.Time          `json:"added,omitzero"`
	Schedule   ProfileSchedule    `json:"schedule,omitzero"`
	ID         string             `json:"id,omitempty"`
	Active     bool               `json:"-"`
	CustomName string             `json:"custom_name,omitempty"`
	Name       string             `json:"name,omitempty"`
	Quota      *SubscriptionQuota `json:"quota,omitempty"`
	Source     string             `json:"source"`
	Updated    time.Time          `json:"updated"`
	URL        string             `json:"-"`
	Nodes      int                `json:"-"`
}

type profile struct {
	ProfileInfo
	Nodes int    `json:"nodes,omitempty"`
	URL   string `json:"url,omitempty"`
	YAML  []byte `json:"yaml"`
}

const profileLimit = 8 << 20

func (m Manager) profilePath() string { return filepath.Join(m.Paths.Config, "profile.json") }

func (m Manager) readProfile() (profile, error) {
	current, err := readProfileAt(m.profilePath())
	if errors.Is(err, os.ErrNotExist) {
		return profile{}, nil
	}
	return current, err
}

func readProfileAt(path string) (profile, error) {
	file, err := os.Open(path)
	if err != nil {
		return profile{}, err
	}
	defer file.Close()
	return decodeSavedProfile(file)
}

func decodeSavedProfile(file io.Reader) (profile, error) {
	var current profile
	body, err := io.ReadAll(file)
	if err != nil {
		return current, err
	}
	if err := json.Unmarshal(body, &current); err != nil {
		return current, i18n.Message("cannot read the saved profile")
	}
	return current, nil
}

// ProfileEditLink returns the original URL for the interactive editor only.
func (m Manager) ProfileEditLink(id string) (string, error) {
	current, _, err := m.findProfile(id)
	return current.URL, err
}

func (m Manager) Profile() (ProfileInfo, error) {
	current, err := m.readProfile()
	return current.summary(), err
}

func (current profile) summary() ProfileInfo {
	info := current.ProfileInfo
	info.URL = maskedURL(current.URL)
	info.Nodes = current.Nodes
	if info.Nodes == 0 && len(current.YAML) > 0 {
		info.Nodes = countProfileNodes(current.YAML)
	}
	return info
}

// countProfileNodes counts top-level entries of the proxies list without
// parsing the whole document; profiles are re-read on every refresh.
func countProfileNodes(body []byte) int {
	count := 0
	inProxies := false
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), len(body)+1)
	for scanner.Scan() {
		line := scanner.Text()
		if !inProxies {
			if strings.HasPrefix(line, "proxies:") {
				inProxies = true
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "-") {
			break
		}
		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			count++
		}
	}
	return count
}

// maskedURL keeps a subscription recognizable without exposing credentials that
// commonly live in the query string or userinfo.
func maskedURL(raw string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(parsed.Scheme)
	b.WriteString("://")
	if parsed.User != nil {
		b.WriteString("***@")
	}
	b.WriteString(parsed.Host)
	// Subscription tokens commonly live in the path, so only its shape is
	// shown; keeping the full path would leak the credential.
	if path := parsed.EscapedPath(); path != "" && path != "/" {
		b.WriteString("/…")
	} else {
		b.WriteString(path)
	}
	if parsed.RawQuery != "" {
		parts := strings.Split(parsed.RawQuery, "&")
		for i, part := range parts {
			if key, _, found := strings.Cut(part, "="); found {
				parts[i] = key + "=…"
			} else {
				parts[i] = part + "=…"
			}
		}
		b.WriteString("?")
		b.WriteString(strings.Join(parts, "&"))
	}
	return b.String()
}

// normalizeSubscription accepts the two shapes subscription servers commonly
// return: plain Clash YAML and the same YAML wrapped in base64.
func normalizeSubscription(body []byte) []byte {
	body = bytes.TrimPrefix(body, []byte{0xEF, 0xBB, 0xBF})
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return body
	}
	if bytes.ContainsAny(trimmed, ":{}#") {
		return body
	}
	compact := strings.Join(strings.Fields(string(trimmed)), "")
	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding,
	} {
		decoded, err := encoding.DecodeString(compact)
		if err != nil {
			continue
		}
		decoded = bytes.TrimPrefix(decoded, []byte{0xEF, 0xBB, 0xBF})
		if utf8.Valid(decoded) && bytes.ContainsAny(decoded, ":{}\n") {
			return decoded
		}
	}
	return body
}

func (m Manager) ImportProfile(ctx context.Context, path string) error {
	current, err := m.readProfile()
	if err != nil {
		return err
	}
	next, err := loadProfileFile(path, "")
	if err != nil {
		return err
	}
	next.ID, next.Schedule, next.Added = current.ID, current.Schedule, current.addedAt()
	return m.updateProfileCandidate(ctx, current, next, true)
}

func loadProfileFile(path, name string) (profile, error) {
	file, err := openProfile(path)
	if err != nil {
		return profile{}, i18n.Message("cannot open the profile file")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return profile{}, i18n.Message("profile path must be a regular file")
	}
	body, err := readProfileBody(file)
	if err != nil {
		return profile{}, err
	}
	if name == "" {
		name = filepath.Base(path)
	}
	return profile{ProfileInfo: ProfileInfo{Source: "file", Name: name}, YAML: body}, nil
}

func (m Manager) AddSubscription(ctx context.Context, source string) error {
	return m.AddNamedSubscription(ctx, source, "")
}

func (m Manager) AddNamedSubscription(ctx context.Context, source, name string) error {
	current, err := m.readProfile()
	if err != nil {
		return err
	}
	next, err := downloadSubscription(ctx, source, "", strings.TrimSpace(name))
	if err != nil {
		return err
	}
	next.ID, next.Schedule, next.Added = current.ID, current.Schedule, current.addedAt()
	return m.updateProfileCandidate(ctx, current, next, true)
}

// EditProfile validates changed subscriptions before taking the commit lock.
func (m Manager) EditProfile(ctx context.Context, id, name, source string, schedule ProfileSchedule) error {
	current, active, err := m.findProfile(id)
	if err != nil {
		return err
	}
	if current.Source == "" {
		return i18n.Message("No active profile")
	}
	if schedule.Minutes < 0 || schedule.Minutes > 525600 {
		return i18n.Message("Update interval must be between 1 and 525600 minutes")
	}
	if source != "" && source != current.URL {
		next, err := downloadSubscription(ctx, source, "", name)
		if err != nil {
			return err
		}
		next.ID, next.Schedule, next.Added = current.ID, schedule, current.addedAt()
		return m.updateProfileCandidate(ctx, current, next, active)
	}
	return m.withLock(func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		latest, latestActive, err := m.findProfile(id)
		if err != nil {
			return err
		}
		if latestActive != active || !reflect.DeepEqual(latest, current) {
			return i18n.Message("Profile changed during the update. Run the command again.")
		}
		if name != "" && name != current.Name {
			current.Name, current.CustomName = name, name
		}
		current.Schedule = schedule
		if !active {
			return m.saveLibraryProfile(current)
		}
		body, err := json.Marshal(current)
		if err != nil {
			return err
		}
		return atomicWrite(m.profilePath(), body)
	})
}

func (m Manager) UpdateProfile(ctx context.Context) error {
	current, err := m.readProfile()
	if err != nil {
		return err
	}
	if current.URL == "" {
		return i18n.Message("no subscription is configured")
	}
	return m.updateSavedProfile(ctx, current, true)
}

func readProfileBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, profileLimit+1))
	if err != nil {
		return nil, i18n.Message("cannot read profile contents")
	}
	if len(body) == 0 || len(body) > profileLimit {
		return nil, i18n.Message("profile must be between 1 byte and 8 MiB")
	}
	return body, nil
}

func downloadSubscription(ctx context.Context, source, previousName, customName string) (profile, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	reportProfileProgress(ctx, "Downloading profile")
	parsed, err := url.Parse(source)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return profile{}, i18n.Message("subscription requires a valid HTTP or HTTPS URL")
	}
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, previous []*http.Request) error {
			req.Header.Del("Referer")
			if len(previous) >= 10 || (parsed.Scheme == "https" && req.URL.Scheme != "https") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	body, headers, err := requestSubscription(ctx, client, source, "mihomo/"+Version+" Termihomo")
	if err != nil && ctx.Err() != nil {
		return profile{}, err
	}
	if err != nil || isProxyLinkList(body) {
		// Providers may reject unknown clients at the HTTP or connection level,
		// or return node links instead of a complete Mihomo profile.
		body, headers, err = requestSubscription(ctx, client, source, "FlClash/0.9.3")
		if err != nil {
			return profile{}, err
		}
		if isProxyLinkList(body) {
			return profile{}, i18n.Message("Subscription returned node links instead of a Mihomo YAML profile")
		}
	}
	name := customName
	if name == "" {
		name = subscriptionName(headers, previousName, parsed.Hostname())
	}
	return profile{ProfileInfo: ProfileInfo{Source: "subscription", Name: name, CustomName: customName,
		Quota: parseSubscriptionQuota(headers.Get("Subscription-Userinfo"))}, URL: source, YAML: body}, nil
}

func isProxyLinkList(body []byte) bool {
	found := false
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		scheme, _, ok := bytes.Cut(line, []byte("://"))
		if !ok {
			return false
		}
		switch strings.ToLower(string(scheme)) {
		case "ss", "ssr", "vmess", "vless", "trojan", "hysteria", "hysteria2", "hy2", "tuic", "socks", "socks5", "wireguard", "anytls":
			found = true
		default:
			return false
		}
	}
	return found
}

func subscriptionName(headers http.Header, previous, hostname string) string {
	title := strings.TrimSpace(headers.Get("Profile-Title"))
	if encoded, ok := strings.CutPrefix(title, "base64:"); ok {
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			decoded, _ = base64.RawStdEncoding.DecodeString(encoded)
		}
		title = strings.TrimSpace(string(decoded))
	}
	if title != "" && utf8.ValidString(title) {
		return title
	}
	if _, params, err := mime.ParseMediaType(headers.Get("Content-Disposition")); err == nil {
		name := filepath.Base(strings.ReplaceAll(strings.TrimSpace(params["filename"]), "\\", "/"))
		if name != "." && name != "/" {
			return name
		}
	}
	if previous != "" {
		return previous
	}
	return hostname
}

func parseSubscriptionQuota(header string) *SubscriptionQuota {
	var quota SubscriptionQuota
	found := false
	for _, field := range strings.Split(header, ";") {
		key, raw, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || value < 0 {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "upload":
			quota.Upload = value
		case "download":
			quota.Download = value
		case "total":
			quota.Total = value
		case "expire":
			quota.Expire = value
		default:
			continue
		}
		found = true
	}
	if !found {
		return nil
	}
	return &quota
}

func (m Manager) applyProfile(ctx context.Context, next profile) error {
	return m.applyProfileCandidate(ctx, next, false)
}

func (m Manager) applyProfileCandidate(ctx context.Context, next profile, validated bool) error {
	if _, err := os.Stat(m.binary()); errors.Is(err, os.ErrNotExist) {
		return i18n.Message("install Mihomo before importing a profile")
	} else if err != nil {
		return err
	}
	state, err := m.readInstance()
	switch {
	case errors.Is(err, os.ErrNotExist):
		state = instance{}
	case err != nil:
		return err
	default:
		process, err := state.openProcess()
		if errors.Is(err, os.ErrProcessDone) {
			state = instance{}
		} else if err != nil {
			return err
		} else {
			defer process.close()
		}
	}
	renderState := state
	if state.PID == 0 {
		renderState = validationInstance()
	}
	candidate, err := m.renderConfig(next.YAML, renderState)
	if err != nil {
		return err
	}
	if !validated {
		if err := m.validateConfig(ctx, candidate); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	reportProfileProgress(ctx, "Saving profile")
	if next.Added.IsZero() {
		next.Added = next.addedAt()
	}
	next.Updated = time.Now()
	if next.Added.IsZero() {
		next.Added = next.Updated
	}
	next.Nodes = countProfileNodes(next.YAML)
	record, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if state.PID == 0 {
		return atomicWrite(m.profilePath(), record)
	}
	if err := m.ready(ctx, state); err != nil {
		return i18n.Message("cannot update an unhealthy Mihomo instance")
	}
	live, err := state.captureLiveConfig(ctx)
	if err != nil {
		return err
	}
	previous, err := os.ReadFile(m.configPath())
	if err != nil {
		return err
	}
	if err := atomicWrite(m.configPath()+".bak", previous); err != nil {
		return err
	}
	pending := profileChange{Previous: previous, liveConfig: live, NextDigest: sha256.Sum256(record)}
	journal, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	if err := atomicWrite(m.pendingPath(), journal); err != nil {
		return err
	}
	if err := atomicWrite(m.configPath(), candidate); err != nil {
		_ = os.Remove(m.pendingPath())
		return err
	}
	reportProfileProgress(ctx, "Applying profile")
	if err := m.applyLiveConfig(ctx, state, candidate, live); err != nil {
		return m.failProfileChange(state, pending, err)
	}
	if err := atomicWrite(m.profilePath(), record); err != nil {
		return m.failProfileChange(state, pending, err)
	}
	m.recordLifecycle("Profile updated")
	return os.Remove(m.pendingPath())
}

func (m Manager) reloadConfig(ctx context.Context, state instance, request []byte) error {
	if err := state.requestWithin(ctx, 20*time.Second, http.MethodPut, "/configs", bytes.NewReader(request), nil); err != nil {
		return err
	}
	return m.ready(ctx, state)
}

type profileChange struct {
	Previous []byte
	liveConfig
	NextDigest [sha256.Size]byte
}

func (m Manager) pendingPath() string {
	return filepath.Join(m.Paths.Data, "profile-change.json")
}

func (m Manager) failProfileChange(state instance, pending profileChange, cause error) error {
	if err := m.restoreConfig(state, pending); err != nil {
		return i18n.Wrap("profile update failed and recovery is incomplete: %s", err)
	}
	if err := os.Remove(m.pendingPath()); err != nil {
		return i18n.Wrap("previous configuration restored, but could not clear recovery state: %s", err)
	}
	return i18n.Wrap("profile update failed; restored previous configuration: %s", cause)
}

func (m Manager) recoverPendingProfile() error {
	body, err := os.ReadFile(m.pendingPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var pending profileChange
	if err := json.Unmarshal(body, &pending); err != nil {
		return i18n.Message("cannot read pending profile change")
	}
	record, err := os.ReadFile(m.profilePath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if sha256.Sum256(record) != pending.NextDigest {
		state, err := m.readInstance()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := m.restoreConfig(state, pending); err != nil {
			return i18n.Wrap("recover interrupted profile change: %s", err)
		}
	}
	return os.Remove(m.pendingPath())
}

func (m Manager) restoreConfig(state instance, pending profileChange) error {
	if err := atomicWrite(m.configPath(), pending.Previous); err != nil {
		return i18n.Wrap("restore previous configuration file: %s", err)
	}
	if state.PID == 0 {
		return nil
	}
	// A failed HTTP request may still be applying the candidate. Replacing
	// the owned process makes rollback independent of that in-flight request.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := m.stop(); err != nil {
		return i18n.Wrap("profile update failed and Mihomo could not be stopped for recovery: %s", err)
	}
	if err := m.startOnMode(ctx, state.Proxy, state.Paused); err != nil {
		return i18n.Wrap("profile update failed and the previous profile could not be restarted: %s", err)
	}
	restored, err := m.runningInstance()
	if err != nil {
		return i18n.Wrap("profile update failed and the restored process is unavailable: %s", err)
	}
	if err := m.restoreRuntime(ctx, restored, pending.Choices, Overview{
		Mode: strings.ToLower(pending.Mode), LAN: pending.LAN, TUNConfigured: pending.TUN != nil,
	}); err != nil {
		return i18n.Wrap("previous profile is running, but its runtime settings could not be restored: %s", err)
	}
	if err := m.ready(ctx, restored); err != nil {
		return i18n.Wrap("profile update failed and recovery could not be verified: %s", err)
	}
	m.recordLifecycle("Previous profile restored after update failure")
	return nil
}

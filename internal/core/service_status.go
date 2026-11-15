package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
	"github.com/Nyakooo/termihomo/internal/platform"
)

const serviceProbeGroup = "__termihomo_service_probe"

const ErrServiceProbeNeedsRestart = i18n.Message("Restart Mihomo to enable selected-node service checks")

type ServiceStatus struct {
	Status string
	Delay  time.Duration
	Region string
	Node   string
	HTTP   int
}

const (
	ServiceAvailable     = "Available"
	ServiceRestricted    = "Access restricted"
	ServiceUnavailable   = "Unavailable"
	ServiceFailed        = "Check failed"
	ServiceTimeout       = "Timeout"
	ServiceUnsupported   = "Region not supported"
	ServiceDisallowedISP = "Disallowed ISP"
	ServiceBlocked       = "Blocked"
	ServiceOriginalsOnly = "Originals only"
)

var youtubeRegion = regexp.MustCompile(`(?i)id=["']country-code["'][^>]*>\s*([a-z]{2,3})\s*<|"(?:GL|countryCode|country_code)"\s*:\s*"([a-z]{2})"`)

var serviceSlots = make(chan struct{}, 4)

var serviceEndpoints = map[string]struct {
	url   string
	limit int64
}{
	"Claude":      {"https://claude.ai/cdn-cgi/trace", 16 * 1024},
	"Gemini":      {"https://gemini.google.com/", 1024 * 1024},
	"Disney+":     {"https://www.disneyplus.com/", 512 * 1024},
	"Prime Video": {"https://www.primevideo.com/", 4 * 1024 * 1024},
	"Spotify":     {"https://www.spotify.com/api/content/v1/country-selector?platform=web&format=json", 16 * 1024},
	"TikTok":      {"https://www.tiktok.com/", 512 * 1024},
	"Discord":     {"https://discord.com/api/v9/auth/location-metadata", 16 * 1024},
}

var (
	bilibiliCode    = regexp.MustCompile(`^\s*\{\s*"code"\s*:\s*(-?\d+)`)
	geminiRegion    = regexp.MustCompile(`,2,1,200,"([a-z]{3})"`)
	territoryRegion = regexp.MustCompile(`"currentterritory"\s*:\s*"([a-z]{2,3})"`)
	serviceCountry  = regexp.MustCompile(`"country_?code"\s*:\s*"([a-z]{2})"`)
)

func (m Manager) CheckServices(ctx context.Context, node string, targets []string, report func(int, ServiceStatus)) error {
	wait, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var lock *os.File
	for {
		var err error
		lock, err = m.Paths.LockServiceProbe()
		if err == nil {
			break
		}
		if !errors.Is(err, platform.ErrInstanceBusy) {
			return err
		}
		select {
		case <-wait.Done():
			return err
		case <-time.After(30 * time.Millisecond):
		}
	}
	defer lock.Close()
	state, err := m.runningInstance()
	if err != nil {
		return err
	}
	if state.ServiceProbe == "" {
		return ErrServiceProbeNeedsRestart
	}
	if err := state.selectProxy(ctx, serviceProbeGroup, node); err != nil {
		return err
	}
	proxy := &url.URL{Scheme: "http", Host: state.ServiceProbe, User: url.UserPassword("termihomo", state.Secret)}
	transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	var workers sync.WaitGroup
	for i, target := range targets {
		workers.Go(func() {
			result := checkService(ctx, client, target)
			result.Node = node
			if ctx.Err() == nil {
				report(i, result)
			}
		})
	}
	workers.Wait()
	return ctx.Err()
}

func checkService(ctx context.Context, client *http.Client, service string) ServiceStatus {
	select {
	case serviceSlots <- struct{}{}:
		defer func() { <-serviceSlots }()
	case <-ctx.Done():
		return ServiceStatus{Status: serviceProbeError(ctx.Err())}
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	switch service {
	case "Google":
		result, _ := probeService(ctx, client, "https://www.google.com/generate_204", 0)
		return result
	case "GitHub":
		result, _ := probeService(ctx, client, "https://github.com/", 0)
		return result
	case "YouTube":
		result, body := probeService(ctx, client, "https://www.youtube.com/premium?hl=en", 1024*1024)
		if region := youtubeRegion.FindStringSubmatch(body); len(region) > 0 {
			result.Region = strings.ToUpper(region[1] + region[2])
		}
		if strings.Contains(body, "premium is not available in your country") ||
			strings.Contains(body, "premium is not available in your region") {
			result.Status = ServiceUnsupported
		}
		return result
	case "ChatGPT":
		return checkChatGPT(ctx, client)
	case "Netflix":
		return checkNetflix(ctx, client)
	case "Bilibili":
		return checkBilibili(ctx, client)
	default:
		return checkServiceEndpoint(ctx, client, service)
	}
}

func checkServiceEndpoint(ctx context.Context, client *http.Client, service string) ServiceStatus {
	target, ok := serviceEndpoints[service]
	if !ok {
		return ServiceStatus{Status: ServiceFailed}
	}
	result, body := probeService(ctx, client, target.url, target.limit)
	if result.Status == ServiceTimeout || result.Status == ServiceFailed {
		return result
	}
	switch service {
	case "Claude":
		result.Region = traceRegion(body)
		if slices.Contains([]string{"AF", "BY", "CN", "CU", "HK", "IR", "KP", "MO", "RU", "SY"}, result.Region) {
			result.Status = ServiceUnsupported
		}
	case "Gemini":
		result.Region = matchedRegion(geminiRegion, body)
		if slices.Contains([]string{"CHN", "RUS", "BLR", "CUB", "IRN", "PRK", "SYR", "HKG", "MAC"}, result.Region) {
			result.Status = ServiceUnsupported
		}
	case "Prime Video":
		result.Region = matchedRegion(territoryRegion, body)
		if strings.Contains(body, `"isservicerestricted":true`) {
			result.Status = ServiceUnsupported
		}
	case "Spotify", "Discord":
		result.Region = matchedRegion(serviceCountry, body)
	}
	if strings.Contains(body, "not available in your country") ||
		strings.Contains(body, "not available in your region") {
		result.Status = ServiceUnsupported
	}
	return result
}

func matchedRegion(pattern *regexp.Regexp, body string) string {
	if match := pattern.FindStringSubmatch(body); len(match) > 0 {
		return strings.ToUpper(match[1])
	}
	return ""
}

func traceRegion(body string) string {
	for line := range strings.SplitSeq(body, "\n") {
		if region, ok := strings.CutPrefix(line, "loc="); ok && len(strings.TrimSpace(region)) == 2 {
			return strings.ToUpper(strings.TrimSpace(region))
		}
	}
	return ""
}

func checkNetflix(ctx context.Context, client *http.Client) ServiceStatus {
	first, _ := probeService(ctx, client, "https://www.netflix.com/title/81280792", 0)
	if first.HTTP == 0 || first.Status == ServiceAvailable {
		return first
	}
	second, _ := probeService(ctx, client, "https://www.netflix.com/title/70143836", 0)
	switch {
	case second.Status == ServiceAvailable:
		return second
	case first.HTTP == 403 || second.HTTP == 403:
		first.Status = ServiceUnsupported
	case first.HTTP == 404 && second.HTTP == 404:
		first.Status = ServiceOriginalsOnly
	}
	return first
}

func checkBilibili(ctx context.Context, client *http.Client) ServiceStatus {
	for _, episode := range []struct{ id, region string }{{"307247", "CN"}, {"183799", "HK/MO/TW"}} {
		result, body := probeService(ctx, client,
			"https://api.bilibili.com/pgc/player/web/playurl?ep_id="+episode.id+"&qn=0&fnval=16&module=bangumi", 16*1024)
		if result.Status != ServiceAvailable {
			return result
		}
		// The playback response can be larger than the cap. Read only the leading code.
		var envelope map[string]json.RawMessage
		code := bilibiliCode.FindStringSubmatch(body)
		if len(code) < 2 {
			if json.Unmarshal([]byte(body), &envelope) == nil && envelope["code"] != nil {
				code = []string{"", string(envelope["code"])}
			} else {
				result.Status = ServiceFailed
				return result
			}
		}
		switch code[1] {
		case "0":
			result.Region = episode.region
			return result
		case "-10403", "-404":
			if episode.region != "CN" {
				result.Status = ServiceUnsupported
				return result
			}
		default:
			result.Status = ServiceFailed
			return result
		}
	}
	return ServiceStatus{Status: ServiceUnsupported}
}

func checkChatGPT(ctx context.Context, client *http.Client) ServiceStatus {
	result, body := probeService(ctx, client, "https://ios.chat.openai.com/", 16*1024)
	if result.HTTP != 0 && result.Status != ServiceFailed && result.Status != ServiceTimeout {
		switch {
		case strings.Contains(body, "you may be connected to a disallowed isp"):
			result.Status = ServiceDisallowedISP
		case strings.Contains(body, "sorry, you have been blocked"):
			result.Status = ServiceBlocked
		case strings.Contains(body, "request is not allowed. please try again later."):
			result.Status = ServiceAvailable
		default:
			result.Status = ServiceFailed
		}
	}
	_, trace := probeService(ctx, client, "https://chat.openai.com/cdn-cgi/trace", 16*1024)
	result.Region = traceRegion(trace)
	_, web := probeService(ctx, client, "https://api.openai.com/compliance/cookie_requirements", 16*1024)
	if strings.Contains(web, "unsupported_country") {
		result.Status = ServiceUnsupported
	}
	return result
}

func probeService(ctx context.Context, client *http.Client, target string, limit int64) (ServiceStatus, string) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ServiceStatus{Status: ServiceFailed}, ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/131.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return ServiceStatus{Status: serviceProbeError(err)}, ""
	}
	defer resp.Body.Close()
	result := ServiceStatus{Delay: time.Since(start), HTTP: resp.StatusCode, Status: ServiceUnavailable}
	switch resp.StatusCode {
	case 401, 403, 429, 451:
		result.Status = ServiceRestricted
	default:
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			result.Status = ServiceAvailable
		}
	}
	if limit == 0 {
		return result, ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		result.Status, result.Delay = serviceProbeError(err), 0
	}
	return result, strings.ToLower(string(body))
}

func serviceProbeError(err error) string {
	var timeout interface{ Timeout() bool }
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		return ServiceTimeout
	}
	return ServiceFailed
}

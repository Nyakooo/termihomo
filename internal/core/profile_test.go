package core

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nyakooo/termihomo/internal/platform"
)

func TestProfileLibraryCacheFollowsExternalChanges(t *testing.T) {
	m := (Manager{Paths: platform.Paths{Config: t.TempDir()}}).WithRuntimeCache()
	if err := os.Mkdir(m.libraryDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a", 32)
	path := filepath.Join(m.libraryDir(), id)
	write := func(name string) {
		t.Helper()
		body, err := json.Marshal(profile{ProfileInfo: ProfileInfo{ID: id, Name: name, Source: "subscription"},
			URL: "https://example.org/sub?token=secret", Nodes: 1})
		if err != nil {
			t.Fatal(err)
		}
		if err := atomicWrite(path, body); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"first", "other"} {
		write(name)
		_, cards, err := m.ProfileLibrary()
		if err != nil || len(cards) != 1 || cards[0].Name != name || strings.Contains(cards[0].URL, "secret") {
			t.Fatalf("library = %v, %v", cards, err)
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, cards, err := m.ProfileLibrary(); err != nil || len(cards) != 0 {
		t.Fatalf("deleted profile still cached: %v, %v", cards, err)
	}
}

func TestMaskedURLHidesCredentials(t *testing.T) {
	cases := map[string]string{
		"https://user:pass@example.com/sub?token=abc&flag": "https://***@example.com/…?token=…&flag=…",
		"https://example.com/sub":                          "https://example.com/…",
		"https://example.com/":                             "https://example.com/",
		"not a url":                                        "",
		"":                                                 "",
	}
	for raw, want := range cases {
		if got := maskedURL(raw); got != want {
			t.Errorf("maskedURL(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestNormalizeSubscriptionDecodesBase64(t *testing.T) {
	yaml := "proxies:\n  - name: HK-01\n    type: ss\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(yaml))
	if got := string(normalizeSubscription([]byte(encoded))); got != yaml {
		t.Fatalf("base64 subscription not decoded: %q", got)
	}
	if got := string(normalizeSubscription([]byte(yaml))); got != yaml {
		t.Fatalf("plain YAML must stay untouched: %q", got)
	}
}

func TestCountProfileNodes(t *testing.T) {
	body := []byte("mixed-port: 7890\nproxies:\n  - name: a\n    type: ss\n  - name: b\n    type: vmess\nproxy-groups:\n  - name: Proxy\n    type: select\n")
	if got := countProfileNodes(body); got != 2 {
		t.Fatalf("countProfileNodes = %d, want 2", got)
	}
}

func TestParseSubscriptionQuota(t *testing.T) {
	quota := parseSubscriptionQuota(" upload=1024; DOWNLOAD=2048; total=10240; expire=2051222400; x=invalid ")
	if quota == nil || *quota != (SubscriptionQuota{Upload: 1024, Download: 2048, Total: 10240, Expire: 2051222400}) {
		t.Fatalf("unexpected quota: %+v", quota)
	}
	if quota := parseSubscriptionQuota("total=invalid; upload=-1"); quota != nil {
		t.Fatalf("invalid metadata yielded %+v", quota)
	}
}

func TestSubscriptionNamePrefersProviderMetadata(t *testing.T) {
	h := http.Header{}
	h.Set("Content-Disposition", "attachment; filename*=UTF-8''%E9%85%8D%E7%BD%AE.yaml")
	if got := subscriptionName(h, "example.org", "example.org"); got != "配置.yaml" {
		t.Fatalf("filename = %q", got)
	}
	h.Set("Profile-Title", "base64:55So5oi36K6i6ZiF")
	if got := subscriptionName(h, "", "example.org"); got != "用户订阅" {
		t.Fatalf("title = %q", got)
	}
	if got := subscriptionName(nil, "Existing.yaml", "example.org"); got != "Existing.yaml" {
		t.Fatalf("previous name = %q", got)
	}
}

func TestSubscriptionRetriesWithCompatibleClientWhenProviderReturnsLinks(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("vless://node-a\nhysteria2://node-b\n"))
	const yaml = "proxies: []\nrules: [MATCH,DIRECT]\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.UserAgent(), "FlClash/") {
			w.Header().Set("Profile-Title", "Provider profile")
			w.Header().Set("Subscription-Userinfo", "upload=1; download=2; total=50")
			_, _ = w.Write([]byte(yaml))
			return
		}
		_, _ = w.Write([]byte(encoded[:16] + "\r\n" + encoded[16:]))
	}))
	defer server.Close()
	item, err := downloadSubscription(t.Context(), server.URL, "", "")
	if err != nil || string(item.YAML) != yaml || item.Name != "Provider profile" || item.Quota == nil || item.Quota.Total != 50 {
		t.Fatalf("expected compatible profile and its metadata, error=%v", err)
	}
}

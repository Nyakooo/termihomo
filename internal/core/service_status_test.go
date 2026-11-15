package core

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestServiceCheckUsesManagedProxy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "www.google.com:443" {
			t.Errorf("unexpected proxy request: %s %s", r.Method, r.Host)
		}
		http.Error(w, "proxy refused the tunnel", http.StatusBadGateway)
	}))
	defer server.Close()
	proxy, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	result := checkService(t.Context(), &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}}, "Google")
	if result.Status != ServiceFailed {
		t.Fatalf("expected the proxy error to be reflected, got %+v", result)
	}
}

func TestServiceProbeClassifiesRestrictedResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("Access restricted"))
	}))
	defer server.Close()
	result, body := probeService(t.Context(), server.Client(), server.URL, 1024)
	if result.HTTP != http.StatusForbidden || result.Status != ServiceRestricted || body != "access restricted" {
		t.Fatalf("unexpected service result: %+v, body=%q", result, body)
	}
}

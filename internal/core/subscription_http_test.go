package core

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSubscriptionBootstrapsWithoutSystemDNSOrProxy(t *testing.T) {
	const body = "proxies: []\nrules: [MATCH,DIRECT]\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer server.Close()
	dns, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer dns.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := dns.ReadFrom(buf)
			if err != nil {
				return
			}
			q := append([]byte(nil), buf[:n]...)
			if n < 12 {
				continue
			}
			end := 12
			for end < len(q) && q[end] != 0 {
				end += int(q[end]) + 1
			}
			if end+5 > len(q) {
				continue
			}
			kind := binary.BigEndian.Uint16(q[end+1 : end+3])
			q = q[:end+5]
			q[10], q[11] = 0, 0
			q[2], q[3] = 0x81, 0x80
			if kind != 1 {
				_, _ = dns.WriteTo(q, addr)
				continue
			}
			binary.BigEndian.PutUint16(q[6:8], 1)
			q = append(q, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 127, 0, 0, 1)
			_, _ = dns.WriteTo(q, addr)
		}
	}()
	target, _ := url.Parse(server.URL)
	target.Host = strings.Replace(target.Host, "127.0.0.1", "bootstrap.test", 1)
	client := &http.Client{Transport: &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		return nil, &net.DNSError{Err: "system DNS is unavailable", Name: "bootstrap.test"}
	}}}
	got, _, err := requestSubscriptionUsing(t.Context(), client, target.String(), "mihomo/test", []string{dns.LocalAddr().String()})
	if err != nil || string(got) != body {
		t.Fatalf("cold-start download failed: %v", err)
	}
}

func TestBootstrapDNSWaitCancelsPromptly(t *testing.T) {
	dns, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer dns.Close()
	started := make(chan struct{})
	go func() {
		buf := make([]byte, 2048)
		_, _, err := dns.ReadFrom(buf)
		if err == nil {
			close(started)
		}
	}()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	client := &http.Client{Transport: &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) {
		return nil, &net.DNSError{Err: "system DNS unavailable"}
	}}}
	go func() {
		_, _, err := requestSubscriptionUsing(ctx, client, "https://bootstrap.test/sub", "mihomo/test", []string{dns.LocalAddr().String()})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("bootstrap resolver did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("bootstrap ignored cancellation")
	}
}

func TestSubscriptionHeaderBudgetAllowsSlowBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(400 * time.Millisecond)
		_, _ = w.Write([]byte("profile"))
	}))
	defer server.Close()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	resp, err := doSubscriptionRequest(server.Client(), req, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || string(body) != "profile" {
		t.Fatalf("healthy body was truncated: %v", err)
	}
}

func TestSubscriptionHeaderBudgetCancelsStalledRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	start := time.Now()
	_, err := doSubscriptionRequest(server.Client(), req, 30*time.Millisecond)
	if err == nil || time.Since(start) > time.Second {
		t.Fatal("stalled route did not yield its budget to the next route")
	}
}

package core

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/Nyakooo/termihomo/internal/i18n"
)

func subscriptionBootstrapTransport(dnsAddress string) *http.Transport {
	resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		dialer := net.Dialer{Timeout: 2 * time.Second}
		return dialer.DialContext(ctx, network, dnsAddress)
	}}
	dialer := net.Dialer{Timeout: 3 * time.Second, Resolver: resolver}
	return &http.Transport{
		// Avoid stale environment proxies and unusable IPv6 routes left by a VPN.
		DialContext: func(ctx context.Context, _, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp4", address)
		},
		TLSHandshakeTimeout:   3 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ForceAttemptHTTP2:     true,
		DisableKeepAlives:     true,
	}
}

// Bound connection setup and response headers, without truncating a healthy body.
func doSubscriptionRequest(client *http.Client, req *http.Request, headerTimeout time.Duration) (*http.Response, error) {
	ctx, cancel := context.WithCancel(req.Context())
	timer := time.AfterFunc(headerTimeout, cancel)
	attempt := *client
	attempt.Timeout = 0 // The parent context bounds the complete download.
	resp, err := attempt.Do(req.Clone(ctx))
	timer.Stop()
	if err != nil {
		cancel()
		return resp, err
	}
	resp.Body = &subscriptionBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type subscriptionBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *subscriptionBody) Close() error { defer b.cancel(); return b.ReadCloser.Close() }

func requestSubscription(ctx context.Context, client *http.Client, source, userAgent string) ([]byte, http.Header, error) {
	return requestSubscriptionUsing(ctx, client, source, userAgent, []string{"223.5.5.5:53", "119.29.29.29:53"})
}

func requestSubscriptionUsing(ctx context.Context, client *http.Client, source, userAgent string, bootstrapDNS []string) ([]byte, http.Header, error) {
	requestCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	downloadError := func(fallback error) error {
		if errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		if requestCtx.Err() != nil {
			return i18n.Message("Subscription download timed out; check the link and retry")
		}
		return fallback
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, source, nil)
	if err != nil {
		return nil, nil, i18n.Message("invalid subscription request")
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := doSubscriptionRequest(client, req, 6*time.Second)
	if err != nil && requestCtx.Err() == nil {
		// A stale environment proxy must not prevent bootstrapping a profile.
		transport := client.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		if configured, ok := transport.(*http.Transport); ok && configured.Proxy != nil {
			proxy, proxyErr := configured.Proxy(req)
			if proxyErr == nil && proxy != nil {
				directTransport := configured.Clone()
				directTransport.Proxy = nil
				defer directTransport.CloseIdleConnections()
				directClient := *client
				directClient.Transport = directTransport
				resp, err = doSubscriptionRequest(&directClient, req, 6*time.Second)
			}
		}
	}
	// Cold-start DNS must not depend on an already running VPN. Preserve the
	// original URL so TLS still verifies the subscription hostname normally.
	for _, address := range bootstrapDNS {
		if err == nil || requestCtx.Err() != nil {
			break
		}
		transport := subscriptionBootstrapTransport(address)
		fallback := *client
		fallback.Transport = transport
		resp, err = doSubscriptionRequest(&fallback, req, 6*time.Second)
		transport.CloseIdleConnections()
	}

	if err != nil {
		return nil, nil, downloadError(subscriptionNetworkError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, i18n.Errorf("subscription server returned HTTP %d", resp.StatusCode)
	}
	body, err := readProfileBody(resp.Body)
	if err != nil {
		return nil, nil, downloadError(err)
	}
	return normalizeSubscription(body), resp.Header, nil
}

// Classify network failures without including URLs, credentials, or tokens.
func subscriptionNetworkError(err error) error {
	var dns *net.DNSError
	var certificate *tls.CertificateVerificationError
	var network *net.OpError
	switch {
	case errors.As(err, &dns):
		return i18n.Message("Subscription download failed: DNS lookup failed")
	case errors.As(err, &certificate):
		return i18n.Message("Subscription download failed: TLS certificate verification failed")
	case errors.As(err, &network):
		return i18n.Message("Subscription download failed: network or proxy connection failed")
	default:
		return i18n.Message("Subscription download failed: connection interrupted or TLS handshake failed")
	}
}

package core

import (
	"net/url"
	"regexp"
	"strings"
)

type DNSEvent struct {
	Domain, Type, Result, Source string
	Started, Cached              bool
}

var (
	dnsStart  = regexp.MustCompile(`^resolve (\S+) ([A-Z0-9]+) from (.+)$`)
	dnsAnswer = regexp.MustCompile(`^(\S+) --> (.*) ([A-Z0-9]+) from (.+)$`)
	dnsCached = regexp.MustCompile(`^cache hit (\S+) --> (.*) ([A-Z0-9]+), expire at .+$`)
)

func parseDNSEvent(payload string) *DNSEvent {
	line, ok := strings.CutPrefix(payload, "[DNS] ")
	if !ok {
		return nil
	}
	if values := dnsStart.FindStringSubmatch(line); values != nil {
		return &DNSEvent{Domain: strings.TrimSuffix(values[1], "."), Type: values[2], Source: dnsSource(values[3]), Started: true}
	}
	if values := dnsCached.FindStringSubmatch(line); values != nil {
		return &DNSEvent{Domain: strings.TrimSuffix(values[1], "."), Type: values[3], Result: values[2], Cached: true}
	}
	if values := dnsAnswer.FindStringSubmatch(line); values != nil {
		return &DNSEvent{Domain: strings.TrimSuffix(values[1], "."), Type: values[3], Result: values[2], Source: dnsSource(values[4])}
	}
	return nil
}

// Show the upstream endpoint without credentials, request parameters or paths.
func dnsSource(value string) string {
	value = strings.TrimSpace(value)
	if address, err := url.Parse(value); err == nil && address.Host != "" {
		return address.Scheme + "://" + address.Host
	}
	if strings.ContainsAny(value, "@/?#") {
		return ""
	}
	return value
}

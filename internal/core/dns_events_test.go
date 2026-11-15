package core

import "testing"

func TestParseDNSDebugEvents(t *testing.T) {
	start := parseDNSEvent("[DNS] resolve example.com A from https://user:secret@dns.example/dns-query?token=123")
	answer := parseDNSEvent("[DNS] example.com --> [192.0.2.1] A from https://user:secret@dns.example/dns-query?token=123")
	hit := parseDNSEvent("[DNS] cache hit example.com --> [192.0.2.1] A, expire at 2026-11-15 20:00:00")
	if start == nil || !start.Started || start.Source != "https://dns.example" ||
		answer == nil || answer.Result != "[192.0.2.1]" || answer.Source != start.Source ||
		hit == nil || !hit.Cached || hit.Type != "A" || hit.Source != "" {
		t.Fatalf("unexpected events: %+v %+v %+v", start, answer, hit)
	}
	if parseDNSEvent("[DNS] hijack udp:1.1.1.1:53 from 127.0.0.1:10000") != nil {
		t.Fatal("a hijack notice should not appear as a completed query")
	}
}

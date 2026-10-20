package detector

import "testing"

func TestParseWindowsProxyServer(t *testing.T) {
	t.Parallel()

	got := parseWindowsProxyServer("http=127.0.0.1:7890;https=127.0.0.1:7890")
	if got == nil || !got.Enabled || got.Address != "127.0.0.1" || got.Port != "7890" {
		t.Fatalf("unexpected: %#v", got)
	}

	got = parseWindowsProxyServer("127.0.0.1:8080")
	if got == nil || got.Address != "127.0.0.1" || got.Port != "8080" {
		t.Fatalf("unexpected: %#v", got)
	}

	got = parseWindowsProxyServer("socks=127.0.0.1:1080")
	if got == nil || got.Scheme != "socks4" || got.Port != "1080" {
		t.Fatalf("expected SOCKS4 system proxy, got=%#v", got)
	}
}

func TestParseProxyURL(t *testing.T) {
	t.Parallel()

	got := parseProxyURL("http://127.0.0.1:8080")
	if got == nil || got.Address != "127.0.0.1" || got.Port != "8080" {
		t.Fatalf("unexpected: %#v", got)
	}

	got = parseProxyURL("127.0.0.1:8888")
	if got == nil || got.Address != "127.0.0.1" || got.Port != "8888" {
		t.Fatalf("unexpected: %#v", got)
	}

	got = parseProxyURL("not-a-proxy")
	if got != nil {
		t.Fatalf("expected nil, got=%#v", got)
	}
}

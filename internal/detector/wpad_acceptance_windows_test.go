//go:build windows && acceptance

package detector

import (
	"net/url"
	"syscall"
	"testing"
)

func TestNativeWPADMissingDiscoveryUsesStaticProxy(t *testing.T) {
	target, _ := url.Parse("https://example.test/")
	rules := &proxyRules{autoDetect: true, servers: "https=127.0.0.1:3128"}
	routes, err := autoProxyFailure(rules, target, syscall.Errno(12180))
	if err != nil {
		t.Fatalf("missing WPAD should use the configured static proxy: %v", err)
	}
	if len(routes) != 1 || routes[0].URL == nil || routes[0].URL.Host != "127.0.0.1:3128" {
		t.Fatalf("unexpected fallback routes: %+v", routes)
	}
}

func TestNativeWPADFailureDoesNotSilentlyGoDirect(t *testing.T) {
	target, _ := url.Parse("https://example.test/")
	routes, err := autoProxyFailure(&proxyRules{autoDetect: true}, target, syscall.Errno(12007))
	if err == nil || len(routes) != 0 {
		t.Fatalf("unexpected WPAD failure fallback: routes=%+v err=%v", routes, err)
	}
}

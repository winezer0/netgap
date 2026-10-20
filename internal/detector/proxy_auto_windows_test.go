//go:build windows

package detector

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestNativePACPerURLAndOrderedFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		w.Write([]byte(`function FindProxyForURL(url, host) { if (host == "direct.example") return "DIRECT"; return "PROXY 127.0.0.1:9; DIRECT"; }`))
	}))
	defer server.Close()
	p := &ProxyConfig{Enabled: true, rules: &proxyRules{pacURL: server.URL + "/proxy.pac"}}
	for _, tc := range []struct {
		host  string
		count int
	}{{"direct.example", 1}, {"proxy.example", 2}} {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		u, _ := url.Parse("https://" + tc.host + "/")
		routes, err := p.Routes(ctx, u)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if len(routes) != tc.count || routes[len(routes)-1].URL != nil {
			t.Fatalf("unexpected PAC routes: %+v", routes)
		}
		if tc.count == 2 && (routes[0].URL == nil || routes[0].URL.Host != "127.0.0.1:9") {
			t.Fatal("lost proxy order")
		}
	}
}

func TestNativePACCancellationAndBadScript(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("invalid javascript {{{")) }))
	defer server.Close()
	p := &ProxyConfig{Enabled: true, rules: &proxyRules{pacURL: server.URL + "/bad.pac"}}
	u, _ := url.Parse("https://public.example/")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if routes, err := p.Routes(ctx, u); err == nil || len(routes) != 0 {
		t.Fatal("bad PAC became a direct route")
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := p.Routes(canceled, u); !errors.Is(err, context.Canceled) {
		t.Fatal("ignored canceled context", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		pending := 0
		proxyPending.Range(func(_, _ any) bool { pending++; return true })
		if pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d native callback requests leaked", pending)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestNativePACCancelsInFlight(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		w.Write([]byte(`function FindProxyForURL(url, host) { return "DIRECT"; }`))
	}))
	defer server.Close()
	defer close(release)
	p := &ProxyConfig{Enabled: true, rules: &proxyRules{pacURL: server.URL + "/cancel.pac"}}
	u, _ := url.Parse("https://cancel.example/")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := p.Routes(ctx, u); done <- err }()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("PAC download did not start: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("native PAC cancellation blocked")
	}
}

package ipchecker

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"netgap/internal/detector"
)

type testRouteResolver func(context.Context, *url.URL) ([]detector.ProxyRoute, error)

func (f testRouteResolver) Routes(ctx context.Context, u *url.URL) ([]detector.ProxyRoute, error) {
	return f(ctx, u)
}

func TestTransportUsesOnlySystemSelectedPaths(t *testing.T) {
	var originCalls atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { originCalls.Add(1); w.Write([]byte("origin")) }))
	defer origin.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := listener.Addr().String()
	listener.Close()
	proxy, _ := url.Parse("http://" + dead)
	for _, allowDirect := range []bool{false, true} {
		resolver := testRouteResolver(func(context.Context, *url.URL) ([]detector.ProxyRoute, error) {
			r := []detector.ProxyRoute{{URL: proxy}}
			if allowDirect {
				r = append(r, detector.ProxyRoute{})
			}
			return r, nil
		})
		client := &http.Client{Transport: &systemTransport{config: resolver}, Timeout: time.Second}
		before := originCalls.Load()
		resp, err := client.Get(origin.URL)
		if allowDirect {
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if originCalls.Load() != before+1 {
				t.Fatal("missed explicit DIRECT fallback")
			}
		} else if !errors.Is(err, ErrProxyUnavailable) || originCalls.Load() != before {
			t.Fatal("implicitly bypassed unavailable proxy", err)
		}
	}
}
func TestTransportProxyAuthAndRedirectRules(t *testing.T) {
	var authenticated atomic.Bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authenticated.Store(r.Header.Get("Proxy-Authorization") == "Basic YWxpY2U6c2VjcmV0")
		if r.URL.Host == "first.example" {
			http.Redirect(w, r, "http://second.example/final", http.StatusFound)
			return
		}
		w.Write([]byte("proxy"))
	}))
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	u.User = url.UserPassword("alice", "secret")
	var seen []string
	resolver := testRouteResolver(func(_ context.Context, target *url.URL) ([]detector.ProxyRoute, error) {
		seen = append(seen, target.Host)
		return []detector.ProxyRoute{{URL: u}}, nil
	})
	client := &http.Client{Transport: &systemTransport{config: resolver}, Timeout: time.Second}
	resp, err := client.Get("http://first.example/start")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !authenticated.Load() || string(body) != "proxy" || strings.Join(seen, ",") != "first.example,second.example" {
		t.Fatalf("auth or redirect routing lost: %v %s", seen, body)
	}
}
func TestDirectTransportIgnoresEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("direct")) }))
	defer origin.Close()
	var actual *detector.ProxyConfig
	ctx := ObserveRoute(context.Background(), func(p *detector.ProxyConfig) { actual = p })
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, origin.URL, nil)
	resp, err := (&http.Client{Transport: &systemTransport{config: &detector.ProxyConfig{Enabled: false}}}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if actual == nil || actual.Enabled {
		t.Fatal("did not record actual DIRECT path")
	}
}

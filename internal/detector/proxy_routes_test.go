package detector

import (
	"context"
	"net/url"
	"testing"
)

func TestSystemRoutesPerURL(t *testing.T) {
	p := &ProxyConfig{Enabled: true, rules: &proxyRules{servers: "http=http://alice:secret@127.0.0.1:8001;https=127.0.0.1:8002;socks=127.0.0.1:8003", bypass: "*.internal;<local>"}}
	for _, tc := range []struct {
		target, scheme, port string
		direct               bool
	}{
		{"http://public.example/", "http", "8001", false},
		{"https://public.example/", "http", "8002", false},
		{"http://host.internal/", "", "", true},
		{"https://intranet/", "", "", true},
		{"http://127.0.0.1/", "", "", true},
	} {
		t.Run(tc.target, func(t *testing.T) {
			u, _ := url.Parse(tc.target)
			routes, err := p.Routes(context.Background(), u)
			if err != nil || len(routes) != 1 {
				t.Fatalf("routes=%v err=%v", routes, err)
			}
			if tc.direct {
				if routes[0].URL != nil {
					t.Fatal("expected DIRECT")
				}
				return
			}
			if routes[0].URL == nil || routes[0].URL.Scheme != tc.scheme || routes[0].URL.Port() != tc.port {
				t.Fatalf("wrong route: %+v", routes)
			}
			if tc.port == "8001" {
				pass, _ := routes[0].URL.User.Password()
				if routes[0].URL.User.Username() != "alice" || pass != "secret" {
					t.Fatal("lost credentials")
				}
			}
		})
	}
}
func TestSystemSOCKSAndUnsupportedRules(t *testing.T) {
	u, _ := url.Parse("https://public.example/")
	for _, scheme := range []string{"socks4", "socks4a", "socks5", "socks5h", "https"} {
		p := &ProxyConfig{Enabled: true, rules: &proxyRules{servers: "socks=" + scheme + "://127.0.0.1:1080"}}
		r, err := p.Routes(context.Background(), u)
		if err != nil || r[0].URL.Scheme != scheme {
			t.Fatalf("%s: %v %v", scheme, r, err)
		}
	}
	p := &ProxyConfig{Enabled: true, rules: &proxyRules{servers: "http=127.0.0.1:80"}}
	r, err := p.Routes(context.Background(), u)
	if err != nil || r[0].URL != nil {
		t.Fatal("HTTPS-only missing mapping should be DIRECT")
	}
	p.rules.servers = "https=unknown://127.0.0.1:80"
	if _, err := p.Routes(context.Background(), u); err == nil {
		t.Fatal("unsupported protocol became DIRECT")
	}
}
func TestEnvironmentRoutesRespectSchemeAndBypass(t *testing.T) {
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"} {
		t.Setenv(name, "")
	}
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:8101")
	t.Setenv("HTTPS_PROXY", "socks5://127.0.0.1:8102")
	t.Setenv("NO_PROXY", ".internal")
	p := &ProxyConfig{Enabled: true, rules: &proxyRules{environment: true}}
	for _, tc := range []struct{ target, port string }{{"http://public.example", "8101"}, {"https://public.example", "8102"}, {"https://a.internal", ""}} {
		u, _ := url.Parse(tc.target)
		r, err := p.Routes(context.Background(), u)
		if err != nil {
			t.Fatal(err)
		}
		if tc.port == "" {
			if r[0].URL != nil {
				t.Fatal("ignored NO_PROXY")
			}
		} else if r[0].URL == nil || r[0].URL.Port() != tc.port {
			t.Fatal("wrong protocol proxy")
		}
	}
}

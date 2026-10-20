package detector

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"golang.org/x/net/http/httpproxy"
)

// ProxyRoute is one system-selected path. A nil URL means explicit DIRECT.
type ProxyRoute struct{ URL *url.URL }

var ErrProxyRules = errors.New("proxy rule resolution failed")

type proxyRules struct {
	servers, bypass, pacURL string
	autoDetect, environment bool
}

func (p *ProxyConfig) UsesURLRules() bool { return p != nil && p.rules != nil }

// Routes evaluates the rules for this URL, including redirect destinations.
// It never appends an implicit direct fallback to a configured proxy.
func (p *ProxyConfig) Routes(ctx context.Context, target *url.URL) (routes []ProxyRoute, err error) {
	defer func() {
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, ErrProxyRules) {
			err = fmt.Errorf("%w: %w", ErrProxyRules, err)
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p != nil && p.configErr != nil {
		return nil, p.configErr
	}
	if p == nil || !p.Enabled {
		return []ProxyRoute{{}}, nil
	}
	if p.rules != nil {
		r := p.rules
		if r.environment {
			cfg := httpproxy.FromEnvironment()
			all := envValue("ALL_PROXY")
			if cfg.HTTPProxy == "" {
				cfg.HTTPProxy = all
			}
			if cfg.HTTPSProxy == "" {
				cfg.HTTPSProxy = all
			}
			configured := cfg.HTTPProxy
			if target.Scheme == "https" {
				configured = cfg.HTTPSProxy
			}
			if configured != "" {
				if !strings.Contains(configured, "://") {
					configured = "http://" + configured
				}
				parsed := parseProxyURL(configured)
				if parsed == nil {
					return nil, fmt.Errorf("invalid environment proxy for %s", target.Scheme)
				}
				if _, err := parsed.Routes(ctx, target); err != nil {
					return nil, err
				}
			}
			u, err := cfg.ProxyFunc()(target)
			if err != nil {
				return nil, fmt.Errorf("resolve environment proxy: %w", err)
			}
			return checkedRoute(u)
		}
		if r.pacURL != "" || r.autoDetect {
			return resolveAutoProxy(ctx, r, target)
		}
		return staticRoutes(r, target)
	}
	u := &url.URL{Scheme: p.Scheme, Host: net.JoinHostPort(p.Address, p.Port), User: p.credentials}
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	return checkedRoute(u)
}

func checkedRoute(u *url.URL) ([]ProxyRoute, error) {
	if u == nil {
		return []ProxyRoute{{}}, nil
	}
	copyURL := *u
	u = &copyURL
	u.Scheme = strings.ToLower(u.Scheme)
	switch u.Scheme {
	case "http", "https", "socks4", "socks4a", "socks5", "socks5h":
	default:
		return nil, fmt.Errorf("unsupported proxy protocol %q", u.Scheme)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("proxy host is empty")
	}
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			port = "1080"
		}
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return nil, fmt.Errorf("invalid proxy port")
	}
	u.Host = net.JoinHostPort(u.Hostname(), port)
	return []ProxyRoute{{URL: u}}, nil
}

func staticRoutes(r *proxyRules, target *url.URL) ([]ProxyRoute, error) {
	if bypassProxy(r.bypass, target) {
		return []ProxyRoute{{}}, nil
	}
	servers := make(map[string]string)
	for _, item := range strings.Split(r.servers, ";") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if key, value, ok := strings.Cut(item, "="); ok && !strings.Contains(key, "://") {
			key = strings.ToLower(strings.TrimSpace(key))
			switch key {
			case "http", "https", "ftp", "socks", "all":
			default:
				return nil, fmt.Errorf("invalid system proxy mapping")
			}
			if strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("empty system proxy mapping")
			}
			servers[key] = strings.TrimSpace(value)
		} else {
			servers["all"] = item
		}
	}
	address := servers[strings.ToLower(target.Scheme)]
	if address == "" {
		address = servers["all"]
	}
	if address == "" && servers["socks"] != "" {
		address = servers["socks"]
		if !strings.Contains(address, "://") {
			address = "socks4://" + address
		}
	}
	if address == "" {
		return []ProxyRoute{{}}, nil
	}
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	p := parseProxyURL(address)
	if p == nil {
		return nil, fmt.Errorf("invalid system proxy for %s", target.Scheme)
	}
	p.rules = nil
	return p.Routes(context.Background(), target)
}

func bypassProxy(patterns string, target *url.URL) bool {
	host := strings.ToLower(target.Hostname())
	// Windows implicitly bypasses loopback destinations.
	implicit := !strings.Contains(strings.ToLower(patterns), "<-loopback>")
	if implicit && (host == "localhost" || strings.HasSuffix(host, ".localhost")) {
		return true
	}
	if ip := net.ParseIP(host); implicit && ip != nil && ip.IsLoopback() {
		return true
	}
	for _, pattern := range strings.FieldsFunc(strings.ToLower(patterns), func(c rune) bool { return c == ';' || c == ',' || c == ' ' }) {
		if pattern == "<local>" && !strings.ContainsAny(host, ".:") {
			return true
		}
		candidate := host
		if strings.HasPrefix(pattern, "<") {
			continue
		}
		if strings.Contains(pattern, "://") {
			u, err := url.Parse(pattern)
			if err != nil || u.Scheme != target.Scheme {
				continue
			}
			pattern = u.Host
		}
		if _, _, err := net.SplitHostPort(pattern); err == nil {
			port := target.Port()
			if port == "" {
				if target.Scheme == "https" {
					port = "443"
				} else {
					port = "80"
				}
			}
			candidate = net.JoinHostPort(host, port)
		}
		if pattern == candidate {
			return true
		}
		if matched, _ := path.Match(pattern, candidate); matched {
			return true
		}
	}
	return false
}

func envValue(name string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return os.Getenv(strings.ToLower(name))
}

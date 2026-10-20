package detector

import (
	"net"
	"net/url"
	"strconv"
	"strings"
)

func parseProxyURL(raw string) *ProxyConfig {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil
	}
	if parsed.Host == "" {
		return nil
	}
	h, p, err := net.SplitHostPort(parsed.Host)
	if err != nil && parsed.Port() == "" && strings.Contains(raw, "://") {
		h = parsed.Hostname()
		switch parsed.Scheme {
		case "http":
			p = "80"
		case "https":
			p = "443"
		case "socks4", "socks4a", "socks5", "socks5h":
			p = "1080"
		}
	} else if err != nil {
		return nil
	}
	port, portErr := strconv.Atoi(p)
	if strings.TrimSpace(h) == "" || portErr != nil || port <= 0 || port > 65535 || parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil
	}
	return &ProxyConfig{
		Type:        "port",
		Address:     h,
		Port:        p,
		Enabled:     true,
		Scheme:      parsed.Scheme,
		credentials: parsed.User,
	}
}

func parseWindowsProxyServer(proxyServer string) *ProxyConfig {
	trimmed := strings.TrimSpace(proxyServer)
	if trimmed == "" {
		return nil
	}

	parts := strings.Split(trimmed, ";")
	candidates := make(map[string]string, 4)
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		if strings.Contains(item, "=") && !strings.Contains(strings.SplitN(item, "=", 2)[0], "://") {
			kv := strings.SplitN(item, "=", 2)
			key := strings.ToLower(strings.TrimSpace(kv[0]))
			val := strings.TrimSpace(kv[1])
			if key != "" && val != "" {
				candidates[key] = val
			}
			continue
		}
		candidates["all"] = item
	}

	for _, key := range []string{"http", "https", "all", "socks"} {
		if val := candidates[key]; val != "" {
			if key == "socks" && !strings.Contains(val, "://") {
				val = "socks4://" + val
			}
			if !strings.Contains(val, "://") {
				val = "http://" + val
			}
			p := parseProxyURL(val)
			if p != nil {
				p.rules = &proxyRules{servers: trimmed}
			}
			return p
		}
	}
	return nil
}

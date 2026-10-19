package detector

import (
	"net"
	"net/url"
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
	host := parsed.Host
	if strings.Contains(host, "@") {
		parts := strings.SplitN(host, "@", 2)
		host = parts[1]
	}
	h, p, err := net.SplitHostPort(host)
	if err != nil {
		return nil
	}
	if strings.TrimSpace(h) == "" || strings.TrimSpace(p) == "" {
		return nil
	}
	return &ProxyConfig{
		Type:    "port",
		Address: h,
		Port:    p,
		Enabled: true,
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
		if strings.Contains(item, "=") {
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

	for _, key := range []string{"http", "https", "all"} {
		if val := candidates[key]; val != "" {
			return parseProxyURL(val)
		}
	}
	return nil
}


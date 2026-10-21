package detector

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

const gnomeProxySchema = "org.gnome.system.proxy"

// GNOME rules deliberately do not reuse Windows mapping or implicit loopback
// bypass semantics. See glib-networking's GProxyResolverGnome and GLib's
// GSimpleProxyResolver for precedence and SOCKS candidate expansion.
type gnomeProxyRules struct {
	protocols    map[string]*url.URL
	defaultProxy *url.URL
	ignore       []gnomeIgnoreHost
	state        string
}

type gnomeIgnoreHost struct {
	host, port string
	network    netip.Prefix
}

func (r *gnomeProxyRules) routes(ctx context.Context, target *url.URL) ([]ProxyRoute, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if target == nil || target.Hostname() == "" {
		return nil, errors.New("missing GNOME proxy target host")
	}
	ip, ipErr := netip.ParseAddr(target.Hostname())
	if ipErr == nil {
		ip = ip.WithZone("")
	}
	host := target.Hostname()
	if ipErr != nil {
		var err error
		host, err = idna.Lookup.ToASCII(host)
		if err != nil {
			return nil, errors.New("invalid target hostname")
		}
	}
	for _, rule := range r.ignore {
		if rule.network.IsValid() {
			if ipErr == nil && rule.network.Contains(ip) {
				return []ProxyRoute{{}}, nil
			}
			continue
		}
		// GLib's port-specific exclusions require an explicit port in the URL.
		if rule.port != "" && rule.port != target.Port() {
			continue
		}
		if ipErr == nil {
			if rule.host == ip.String() {
				return []ProxyRoute{{}}, nil
			}
		} else if strings.EqualFold(host, rule.host) || strings.HasSuffix(strings.ToLower(host), "."+strings.ToLower(rule.host)) {
			return []ProxyRoute{{}}, nil
		}
	}
	selected := r.protocols[strings.ToLower(target.Scheme)]
	if selected == nil {
		selected = r.defaultProxy
	}
	if selected == nil {
		return []ProxyRoute{{}}, nil
	}
	if selected.Scheme != "socks" {
		return checkedRoute(selected)
	}
	var routes []ProxyRoute
	for _, scheme := range []string{"socks5", "socks4a", "socks4"} {
		proxy := *selected
		proxy.Scheme = scheme
		checked, err := checkedRoute(&proxy)
		if err != nil {
			return nil, err
		}
		routes = append(routes, checked...)
	}
	return routes, nil
}

// parseGnomeSnapshot consumes only gsettings list-recursively output. It never
// executes a shell, evaluates values as code, or prints private settings.
func parseGnomeSnapshot(output string) (*ProxyConfig, error) {
	values := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || (fields[0] != gnomeProxySchema && !strings.HasPrefix(fields[0], gnomeProxySchema+".")) {
			return nil, errors.New("invalid GNOME proxy snapshot record")
		}
		key := fields[0] + " " + fields[1]
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate GNOME setting %s", key)
		}
		// Locate the value without splitting quoted strings containing spaces.
		rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), fields[0]))
		values[key] = strings.TrimSpace(strings.TrimPrefix(rest, fields[1]))
	}
	get := func(schema, key string) (string, error) {
		value, ok := values[schema+" "+key]
		if !ok {
			return "", fmt.Errorf("missing GNOME setting %s %s", schema, key)
		}
		return value, nil
	}
	text := func(schema, key string) (string, error) {
		value, err := get(schema, key)
		if err != nil {
			return "", err
		}
		parsed, rest, err := gvariantString(strings.TrimSpace(value))
		if err != nil || strings.TrimSpace(rest) != "" {
			return "", fmt.Errorf("invalid string setting %s %s", schema, key)
		}
		return parsed, nil
	}
	boolean := func(schema, key string) (bool, error) {
		value, err := get(schema, key)
		if err != nil {
			return false, err
		}
		switch value {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return false, fmt.Errorf("invalid boolean setting %s %s", schema, key)
		}
	}
	mode, err := text(gnomeProxySchema, "mode")
	if err != nil {
		return nil, err
	}
	if mode == "none" {
		return nil, nil
	}
	if mode == "auto" {
		data, _ := json.Marshal(values)
		return systemRouteObservation(fmt.Sprintf("gnome:%x", sha256.Sum256(data))), nil
	}
	if mode != "manual" {
		return nil, errors.New("unknown Linux GNOME proxy mode")
	}
	shared, err := boolean(gnomeProxySchema, "use-same-proxy")
	if err != nil {
		return nil, err
	}
	ignoreValue, err := get(gnomeProxySchema, "ignore-hosts")
	if err != nil {
		return nil, err
	}
	ignored, err := gvariantStrings(ignoreValue)
	if err != nil {
		return nil, errors.New("invalid GNOME ignore-hosts list")
	}
	r := &gnomeProxyRules{protocols: make(map[string]*url.URL)}
	for _, value := range ignored {
		rule, err := parseGnomeIgnore(value)
		if err != nil {
			return nil, err
		}
		r.ignore = append(r.ignore, rule)
	}
	for _, protocol := range []string{"http", "https", "socks"} {
		schema := gnomeProxySchema + "." + protocol
		host, err := text(schema, "host")
		if err != nil {
			return nil, err
		}
		portText, err := get(schema, "port")
		if err != nil {
			return nil, err
		}
		port, err := strconv.Atoi(portText)
		if err != nil || port < 0 || port > 65535 {
			return nil, fmt.Errorf("invalid GNOME %s proxy port", protocol)
		}
		// A blank host or port zero disables this protocol, per the schema.
		if host == "" || port == 0 {
			continue
		}
		if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
			host = host[1 : len(host)-1]
		}
		if _, err := netip.ParseAddr(host); err != nil {
			if strings.ContainsAny(host, ":[]/\\?#@ \t\r\n") {
				return nil, fmt.Errorf("invalid GNOME %s proxy host", protocol)
			}
			host, err = idna.Lookup.ToASCII(host)
			if err != nil {
				return nil, fmt.Errorf("invalid GNOME %s proxy hostname", protocol)
			}
		}
		proxy := &url.URL{Scheme: "http", Host: net.JoinHostPort(host, strconv.Itoa(port))}
		if protocol == "socks" {
			proxy.Scheme = "socks"
			r.defaultProxy = proxy
			continue
		}
		if protocol == "http" {
			auth, err := boolean(schema, "use-authentication")
			if err != nil {
				return nil, err
			}
			if auth {
				user, err := text(schema, "authentication-user")
				if err != nil {
					return nil, err
				}
				password, err := text(schema, "authentication-password")
				if err != nil {
					return nil, err
				}
				proxy.User = url.UserPassword(user, password)
			}
			if shared {
				r.defaultProxy = proxy
			}
		}
		r.protocols[protocol] = proxy
	}
	if r.protocols["https"] == nil {
		r.protocols["https"] = r.protocols["http"]
	}
	// Include unused/disabled settings too; mutations must invalidate a query.
	data, _ := json.Marshal(values)
	r.state = fmt.Sprintf("%x", sha256.Sum256(data))
	return &ProxyConfig{Enabled: true, Type: "system", rules: &proxyRules{gnome: r}}, nil
}

func parseGnomeIgnore(value string) (gnomeIgnoreHost, error) {
	value = strings.TrimRight(value, " \t\r\n")
	if prefix, err := netip.ParsePrefix(value); err == nil {
		return gnomeIgnoreHost{network: prefix}, nil
	}
	if ip, err := netip.ParseAddr(value); err == nil {
		return gnomeIgnoreHost{host: ip.String()}, nil
	}
	host, port := value, ""
	if strings.Contains(value, ":") {
		var err error
		host, port, err = net.SplitHostPort(value)
		if err != nil {
			return gnomeIgnoreHost{}, errors.New("invalid GNOME ignore-hosts address")
		}
		p, err := strconv.Atoi(port)
		if err != nil || p < 1 || p > 65535 {
			return gnomeIgnoreHost{}, errors.New("invalid GNOME ignore-hosts port")
		}
		port = strconv.Itoa(p)
	}
	host = strings.TrimPrefix(strings.TrimPrefix(host, "*."), ".")
	if host == "" || strings.ContainsAny(host, "*/\\?#@ \t\r\n") {
		return gnomeIgnoreHost{}, errors.New("unsupported GNOME ignore-hosts pattern")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		host = ip.String()
	}
	return gnomeIgnoreHost{host: host, port: port}, nil
}

// A deliberately small GVariant text parser for the schema's strings and
// string arrays. It rejects truncation and trailing data rather than using Trim
// or shell unquoting, which corrupt escaped quotes, spaces and credentials.
func gvariantString(value string) (string, string, error) {
	if len(value) == 0 || (value[0] != '\'' && value[0] != '"') {
		return "", "", errors.New("expected quoted GVariant string")
	}
	quote := value[0]
	var result strings.Builder
	for i := 1; i < len(value); {
		if value[i] == quote {
			text := result.String()
			if !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
				return "", "", errors.New("invalid GVariant Unicode string")
			}
			return text, value[i+1:], nil
		}
		if value[i] != '\\' {
			result.WriteByte(value[i])
			i++
			continue
		}
		i++
		if i >= len(value) {
			break
		}
		c := value[i]
		i++
		switch c {
		case 'u', 'U':
			size := 4
			if c == 'U' {
				size = 8
			}
			if i+size > len(value) {
				return "", "", errors.New("truncated GVariant Unicode escape")
			}
			r, err := strconv.ParseUint(value[i:i+size], 16, 32)
			if err != nil || !utf8.ValidRune(rune(r)) || r == 0 {
				return "", "", errors.New("invalid GVariant Unicode escape")
			}
			result.WriteRune(rune(r))
			i += size
		case 'a':
			result.WriteByte('\a')
		case 'b':
			result.WriteByte('\b')
		case 'f':
			result.WriteByte('\f')
		case 'n':
			result.WriteByte('\n')
		case 'r':
			result.WriteByte('\r')
		case 't':
			result.WriteByte('\t')
		case 'v':
			result.WriteByte('\v')
		case '\n':
		default:
			result.WriteByte(c)
		}
	}
	return "", "", errors.New("unterminated GVariant string")
}

func gvariantStrings(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimSpace(strings.TrimPrefix(value, "@as"))
	if !strings.HasPrefix(value, "[") {
		return nil, errors.New("expected GVariant string array")
	}
	value = strings.TrimSpace(value[1:])
	var result []string
	for !strings.HasPrefix(value, "]") {
		text, rest, err := gvariantString(value)
		if err != nil {
			return nil, err
		}
		result = append(result, text)
		value = strings.TrimSpace(rest)
		if strings.HasPrefix(value, "]") {
			break
		}
		if !strings.HasPrefix(value, ",") {
			return nil, errors.New("missing GVariant array separator")
		}
		value = strings.TrimSpace(value[1:])
	}
	if !strings.HasPrefix(value, "]") || strings.TrimSpace(value[1:]) != "" {
		return nil, errors.New("trailing GVariant array data")
	}
	return result, nil
}

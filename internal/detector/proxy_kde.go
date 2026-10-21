package detector

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// KDE rules are deliberately separate from GNOME/Windows bypass semantics.
// Only ordinary fixed settings are supported, not PAC or KConfig expressions.
type kdeProxyRules struct {
	protocols map[string]*url.URL
	socks     *url.URL
	ignore    []kdeBypass
	local     bool
	reverse   bool
	state     string
}

type kdeBypass struct {
	suffix  string
	network netip.Prefix
}

func parseKDESettings(values map[string]string, state string) (*ProxyConfig, error) {
	mode := values["ProxyType"]
	if mode == "" || mode == "0" {
		return nil, nil
	}
	if mode == "2" || mode == "3" {
		return systemRouteObservation("kde:" + state), nil
	}
	if mode != "1" {
		return nil, errors.New("Linux KDE environment-indirection or unknown proxy mode is unsupported; select no proxy or a fixed proxy")
	}
	r := &kdeProxyRules{protocols: make(map[string]*url.URL), state: state}
	switch strings.ToLower(values["ReversedException"]) {
	case "", "false", "0":
	case "true", "1":
		r.reverse = true
	default:
		return nil, errors.New("invalid KDE reverse bypass setting")
	}
	for _, protocol := range []string{"http", "https", "socks"} {
		value := strings.TrimSpace(values[protocol+"Proxy"])
		if value == "" {
			continue
		}
		// Older KDE versions store the port after a space.
		if fields := strings.Fields(value); len(fields) == 2 {
			port, err := strconv.Atoi(fields[1])
			if err != nil || port < 1 || port > 65535 {
				return nil, errors.New("invalid KDE proxy port")
			}
			value = fields[0] + ":" + fields[1]
		} else if len(fields) != 1 {
			return nil, errors.New("invalid KDE proxy address")
		}
		if !strings.Contains(value, "://") {
			value = "http://" + value
		}
		u, err := url.Parse(value)
		if err != nil || u.Opaque != "" || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || strings.ContainsAny(value, "\r\n\x00\t ") {
			return nil, errors.New("invalid KDE proxy URL")
		}
		if protocol == "socks" {
			// KDE's generic socks endpoint is a SOCKS5 proxy.
			if u.Scheme == "http" || u.Scheme == "socks" {
				u.Scheme = "socks5"
			}
			if u.Scheme != "socks5" && u.Scheme != "socks5h" && u.Scheme != "socks4" && u.Scheme != "socks4a" {
				return nil, errors.New("invalid KDE SOCKS proxy protocol")
			}
			r.socks = u
		} else {
			if u.Scheme != "http" && u.Scheme != "https" {
				return nil, errors.New("invalid KDE HTTP proxy protocol")
			}
			r.protocols[protocol] = u
		}
		if _, err := checkedRoute(u); err != nil {
			return nil, err
		}
	}
	for _, value := range strings.FieldsFunc(values["NoProxyFor"], func(c rune) bool { return c == ',' || c == ' ' }) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if value == "<local>" {
			r.local = true
			continue
		}
		// Wildcards are not interpreted as shell patterns by KDE.
		if strings.Contains(value, "*") {
			return nil, errors.New("unsupported KDE bypass wildcard")
		}
		if network, err := netip.ParsePrefix(value); err == nil {
			r.ignore = append(r.ignore, kdeBypass{network: network})
			continue
		}
		if address, err := netip.ParseAddr(value); err == nil {
			r.ignore = append(r.ignore, kdeBypass{network: netip.PrefixFrom(address, address.BitLen())})
			continue
		}
		if strings.Contains(value, "://") {
			u, err := url.Parse(value)
			if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
				return nil, errors.New("unsupported KDE bypass URL")
			}
			value = u.Hostname()
			if u.Port() != "" {
				value += ":" + u.Port()
			}
		}
		if _, err := parseGnomeIgnore(value); err != nil {
			return nil, fmt.Errorf("invalid KDE bypass rule: %w", err)
		}
		// KDE matches literal suffixes: preserve a leading dot and do not
		// import GNOME's implicit label boundary into this rule.
		r.ignore = append(r.ignore, kdeBypass{suffix: strings.ToLower(value)})
	}
	return &ProxyConfig{Enabled: true, Type: "system", rules: &proxyRules{kde: r}}, nil
}

func (r *kdeProxyRules) routes(ctx context.Context, target *url.URL) ([]ProxyRoute, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if target == nil || target.Hostname() == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return nil, errors.New("invalid KDE proxy target")
	}
	host := strings.ToLower(target.Hostname())
	ip, ipErr := netip.ParseAddr(host)
	matched := r.local && !strings.Contains(host, ".")
	var addresses []netip.Addr
	resolved := false
	for _, rule := range r.ignore {
		if matched {
			break
		}
		if rule.network.IsValid() {
			if ipErr == nil {
				matched = rule.network.Contains(ip.WithZone(""))
				continue
			}
			// KDE CIDR exclusions also apply to resolved hostnames. Honor the
			// query deadline and report DNS failure instead of guessing a path.
			if !resolved {
				var err error
				dnsCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				addresses, err = net.DefaultResolver.LookupNetIP(dnsCtx, "ip", host)
				cancel()
				if err != nil {
					return nil, fmt.Errorf("resolve KDE bypass target: %w", err)
				}
				resolved = true
			}
			if len(addresses) != 0 {
				matched = rule.network.Contains(addresses[0].Unmap())
			}
			continue
		}
		matched = strings.HasSuffix(host, rule.suffix) || (target.Port() != "" && strings.HasSuffix(host+":"+target.Port(), rule.suffix))
	}
	if matched != r.reverse {
		return []ProxyRoute{{}}, nil
	}
	var result []ProxyRoute
	for _, selected := range []*url.URL{r.protocols[target.Scheme], r.socks} {
		if selected != nil {
			paths, err := checkedRoute(selected)
			if err != nil {
				return nil, err
			}
			result = append(result, paths...)
		}
	}
	if len(result) == 0 {
		return []ProxyRoute{{}}, nil
	}
	return result, nil
}

package detector

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strings"
	"testing"
)

// Fixtures never read or modify the machine's GNOME configuration.
func gnomeFixture(overrides map[string]string) string {
	values := map[string]string{
		"mode": "'manual'", "use-same-proxy": "false", "ignore-hosts": "@as []",
		"http.host": "'192.0.2.10'", "http.port": "8101", "http.use-authentication": "false",
		"http.authentication-user": "''", "http.authentication-password": "''",
		"https.host": "'192.0.2.20'", "https.port": "8102",
		"socks.host": "'192.0.2.30'", "socks.port": "1080",
	}
	for key, value := range overrides {
		values[key] = value
	}
	var lines []string
	for key, value := range values {
		schema := gnomeProxySchema
		if protocol, name, ok := strings.Cut(key, "."); ok {
			schema += "." + protocol
			key = name
		}
		lines = append(lines, schema+" "+key+" "+value)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

func gnomeConfig(t *testing.T, overrides map[string]string) *ProxyConfig {
	t.Helper()
	p, err := parseGnomeSnapshot(gnomeFixture(overrides))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func gnomePaths(t *testing.T, p *ProxyConfig, target string) []string {
	t.Helper()
	u, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	routes, err := p.Routes(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, route := range routes {
		if route.URL == nil {
			paths = append(paths, "DIRECT")
		} else {
			paths = append(paths, route.URL.String())
		}
	}
	return paths
}

func TestGnomeManualProtocolSelection(t *testing.T) {
	for _, tc := range []struct {
		name, target, want string
		settings           map[string]string
	}{
		{"http", "http://public.example", "http://192.0.2.10:8101", nil},
		{"https", "https://public.example", "http://192.0.2.20:8102", nil},
		{"https-falls-back-to-http", "https://public.example", "http://192.0.2.10:8101", map[string]string{"https.host": "''"}},
		{"socks-only", "https://public.example", "socks5://192.0.2.30:1080,socks4a://192.0.2.30:1080,socks4://192.0.2.30:1080", map[string]string{"http.host": "''", "https.host": "''"}},
		{"shared-http-default", "custom://public.example", "http://192.0.2.10:8101", map[string]string{"use-same-proxy": "true", "socks.host": "''"}},
		{"explicit-https-overrides-shared", "https://public.example", "http://192.0.2.20:8102", map[string]string{"use-same-proxy": "true"}},
		{"socks-overrides-shared-default", "custom://public.example", "socks5://192.0.2.30:1080,socks4a://192.0.2.30:1080,socks4://192.0.2.30:1080", map[string]string{"use-same-proxy": "true"}},
		{"unmapped-direct", "custom://public.example", "DIRECT", map[string]string{"socks.host": "''"}},
		{"zero-port-disables-http", "http://public.example", "DIRECT", map[string]string{"http.port": "0", "socks.host": "''"}},
		{"no-implicit-loopback-bypass", "http://127.0.0.1", "http://192.0.2.10:8101", nil},
		{"ipv6-proxy", "https://public.example", "http://[2001:db8::1]:8102", map[string]string{"https.host": "'2001:db8::1'"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(gnomePaths(t, gnomeConfig(t, tc.settings), tc.target), ",")
			if got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestGnomeIgnoreHosts(t *testing.T) {
	for _, tc := range []struct {
		pattern, target string
		direct          bool
	}{
		{"example.com", "http://example.com", true},
		{"example.com", "http://a.example.com", true},
		{".example.com", "http://example.com", true},
		{"*.example.com", "http://EXAMPLE.com", true},
		{"example.com", "http://notexample.com", false},
		{"example.com:443", "https://a.example.com:443", true},
		{"example.com:443", "https://a.example.com", false},
		{"example.com:443", "http://example.com:80", false},
		{"192.0.2.0/24", "http://192.0.2.9", true},
		{"192.0.2.0/24", "http://host.example", false},
		{"2001:db8::/32", "https://[2001:db8::9]", true},
		{"[::1]:443", "https://[::1]:443", true},
		{"[::1]:443", "https://[::1]", false},
		{"::1", "https://[::1]", true},
		{"127.0.0.1", "http://localhost", false},
		{"xn--bcher-kva.example", "http://bücher.example", true},
	} {
		t.Run(tc.pattern+"/"+tc.target, func(t *testing.T) {
			p := gnomeConfig(t, map[string]string{"ignore-hosts": "['" + tc.pattern + "']"})
			paths := gnomePaths(t, p, tc.target)
			if (paths[0] == "DIRECT") != tc.direct {
				t.Fatalf("unexpected paths: %v", paths)
			}
		})
	}
}

func TestGnomeCredentialsAndFingerprint(t *testing.T) {
	settings := map[string]string{"http.use-authentication": "true", "http.authentication-user": `'a\'lice'`, "http.authentication-password": `'s\\ecret\u00e9'`, "https.host": "''"}
	p := gnomeConfig(t, settings)
	u, _ := url.Parse("https://public.example")
	routes, err := p.Routes(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	user := routes[0].URL.User
	password, _ := user.Password()
	if user.Username() != "a'lice" || password != "s\\ecreté" {
		t.Fatal("credentials lost while decoding GVariant")
	}
	again := gnomeConfig(t, settings)
	if p.rules.fingerprint() != again.rules.fingerprint() {
		t.Fatal("pointer addresses changed fingerprint")
	}
	settings["http.authentication-password"] = "'changed'"
	if p.rules.fingerprint() == gnomeConfig(t, settings).rules.fingerprint() {
		t.Fatal("credential change not fingerprinted")
	}
	// An explicit HTTPS endpoint does not inherit HTTP credentials.
	settings["https.host"] = "'192.0.2.20'"
	p = gnomeConfig(t, settings)
	routes, err = p.Routes(context.Background(), u)
	if err != nil || routes[0].URL.User != nil {
		t.Fatal("credentials leaked to independent HTTPS proxy", err)
	}
}

func TestGnomeInvalidSettingsFailClosed(t *testing.T) {
	for _, settings := range []map[string]string{
		{"mode": "'auto'"}, {"mode": "manual"}, {"use-same-proxy": "1"},
		{"https.port": "65536"}, {"http.port": "-1"}, {"http.host": "'host/path'"},
		{"http.host": "'[::1'"}, {"ignore-hosts": "['unterminated]"}, {"ignore-hosts": "['host:bad']"},
		{"http.authentication-user": "'unterminated", "http.use-authentication": "true"},
	} {
		if _, err := parseGnomeSnapshot(gnomeFixture(settings)); err == nil {
			t.Fatal("invalid settings became valid rules")
		}
	}
	output := gnomeFixture(nil)
	if _, err := parseGnomeSnapshot(output + output); err == nil {
		t.Fatal("accepted duplicate snapshot")
	}
	if _, err := parseGnomeSnapshot("org.gnome.system.proxy mode 'manual'\n"); err == nil {
		t.Fatal("accepted incomplete snapshot")
	}
	p := gnomeConfig(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	u, _ := url.Parse("https://public.example")
	if _, err := p.Routes(ctx, u); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestGVariantStrings(t *testing.T) {
	for input, want := range map[string]string{`'a\'b'`: "a'b", `"a\"b"`: `a"b`, `'a\\b'`: `a\b`, `'\u00e9\U0001f600'`: "é😀", `'a\nb'`: "a\nb"} {
		got, rest, err := gvariantString(input)
		if err != nil || rest != "" || got != want {
			t.Fatalf("GVariant decode failed: %v", err)
		}
	}
	for _, input := range []string{"'unterminated", `'\uD800'`, `'\U00110000'`, `'\u0000'`, "unquoted"} {
		if _, _, err := gvariantString(input); err == nil {
			t.Fatal("accepted malformed GVariant string")
		}
	}
	for _, input := range []string{"[]", "@as []", "['one', \"two\",]"} {
		if _, err := gvariantStrings(input); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []string{"['one' 'two']", "['one'] garbage", "[1]", "["} {
		if _, err := gvariantStrings(input); err == nil {
			t.Fatal("accepted malformed GVariant array")
		}
	}
}

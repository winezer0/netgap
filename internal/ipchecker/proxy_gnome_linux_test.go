//go:build linux

package ipchecker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"netgap/internal/detector"
)

// Only the test process sees this helper and settings file. Every listener is
// on loopback; no real desktop configuration, DNS or route is modified.
func linuxGnomeTransport(t *testing.T, httpProxy, httpsProxy, bypass string) *http.Client {
	t.Helper()
	httpURL, _ := url.Parse(httpProxy)
	httpsURL, _ := url.Parse(httpsProxy)
	output := fmt.Sprintf(`org.gnome.system.proxy mode 'manual'
org.gnome.system.proxy use-same-proxy false
org.gnome.system.proxy ignore-hosts %s
org.gnome.system.proxy.http host '%s'
org.gnome.system.proxy.http port %s
org.gnome.system.proxy.http use-authentication false
org.gnome.system.proxy.https host '%s'
org.gnome.system.proxy.https port %s
org.gnome.system.proxy.socks host ''
org.gnome.system.proxy.socks port 0
`, bypass, httpURL.Hostname(), httpURL.Port(), httpsURL.Hostname(), httpsURL.Port())
	dir := t.TempDir()
	file := filepath.Join(dir, "settings")
	if err := os.WriteFile(file, []byte(output), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gsettings"), []byte("#!/bin/sh\nexec /bin/cat \"$NETGAP_TEST_GNOME\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	t.Setenv("NETGAP_TEST_GNOME", file)
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	p, source, err := detector.DetectProxyWithSource(context.Background())
	if err != nil || source != "system" {
		t.Fatalf("source=%s err=%v", source, err)
	}
	return &http.Client{Transport: &systemTransport{config: p}, Timeout: 2 * time.Second}
}

func TestLinuxGnomeTransportRedirectUsesBypass(t *testing.T) {
	var proxyCalls, originCalls atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originCalls.Add(1)
		w.Write([]byte("direct after redirect"))
	}))
	defer origin.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls.Add(1)
		http.Redirect(w, r, origin.URL, http.StatusFound)
	}))
	defer proxy.Close()
	client := linuxGnomeTransport(t, proxy.URL, proxy.URL, "['127.0.0.1']")
	response, err := client.Get("http://first.example/start")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "direct after redirect" || proxyCalls.Load() != 1 || originCalls.Load() != 1 {
		t.Fatalf("redirect selection failed: proxy=%d origin=%d err=%v", proxyCalls.Load(), originCalls.Load(), err)
	}
}

func TestLinuxGnomeTransportRedirectUsesHTTPSProxy(t *testing.T) {
	var httpCalls, connectCalls atomic.Int32
	httpsProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect && r.Host == "final.example:443" {
			connectCalls.Add(1)
		}
		// Deliberately stop at CONNECT: never dial or resolve the target host.
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer httpsProxy.Close()
	httpProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalls.Add(1)
		http.Redirect(w, r, "https://final.example/", http.StatusFound)
	}))
	defer httpProxy.Close()
	client := linuxGnomeTransport(t, httpProxy.URL, httpsProxy.URL, "@as []")
	if response, err := client.Get("http://first.example/"); err == nil {
		response.Body.Close()
		t.Fatal("unexpected success after rejected CONNECT")
	}
	if httpCalls.Load() != 1 || connectCalls.Load() != 1 {
		t.Fatalf("wrong proxy: http=%d https=%d", httpCalls.Load(), connectCalls.Load())
	}
}

func TestLinuxGnomeTransportRefusedProxyDoesNotDialDirect(t *testing.T) {
	var originCalls atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { originCalls.Add(1) }))
	defer origin.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadProxy := "http://" + listener.Addr().String()
	listener.Close()
	client := linuxGnomeTransport(t, deadProxy, deadProxy, "@as []")
	if response, err := client.Get(origin.URL); !errors.Is(err, ErrProxyUnavailable) {
		if response != nil {
			response.Body.Close()
		}
		t.Fatal("missing proxy unavailable error", err)
	}
	if originCalls.Load() != 0 {
		t.Fatal("refused proxy implicitly became DIRECT")
	}
}

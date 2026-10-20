package ipchecker

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"netgap/internal/detector"
)

type recoveryFetcher struct {
	name  string
	fetch func(context.Context, *http.Client) (string, error)
}

func (f *recoveryFetcher) Name() string { return f.name }
func (f *recoveryFetcher) Key() string  { return f.name }
func (f *recoveryFetcher) Fetch(ctx context.Context, client *http.Client) (string, error) {
	return f.fetch(ctx, client)
}

func registerRecoveryFetcher(t *testing.T, suffix string, fetch func(context.Context, *http.Client) (string, error)) *recoveryFetcher {
	t.Helper()
	initFetchers()
	f := &recoveryFetcher{name: t.Name() + suffix, fetch: fetch}
	RegisterFetcher(f)
	t.Cleanup(func() {
		fetchersMux.Lock()
		defer fetchersMux.Unlock()
		delete(ipFetchers, f.Name())
		delete(errorCounts, f.Name())
		delete(cooldownUntil, f.Name())
	})
	return f
}

func fetchRecoveryIP(ctx context.Context, client *http.Client, endpoint string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return string(body), err
}

func TestRecoveryNetworkFailurePreservesAPIs(t *testing.T) {
	recovered := false
	var keys []string
	for _, suffix := range []string{"-a", "-b"} {
		f := registerRecoveryFetcher(t, suffix, func(context.Context, *http.Client) (string, error) {
			if recovered {
				return "203.0.113.7", nil
			}
			return "", context.DeadlineExceeded
		})
		keys = append(keys, f.Key())
	}
	for i := 0; i < maxErrorCount+2; i++ {
		if _, err, _ := GetExternalIP(nil, keys, nil, ""); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected transient timeout, got %v", err)
		}
	}
	for _, key := range keys {
		fetchersMux.RLock()
		count := errorCounts[key]
		_, registered := ipFetchers[key]
		_, cooling := cooldownUntil[key]
		fetchersMux.RUnlock()
		if !registered || count != 0 || cooling {
			t.Fatalf("network failure penalized %s", key)
		}
	}
	recovered = true
	if ip, err, _ := GetExternalIP(nil, keys, nil, ""); err != nil || ip != "203.0.113.7" {
		t.Fatalf("failed to recover automatically: ip=%q err=%v", ip, err)
	}
}

func TestRecoveryAPICooldownExpires(t *testing.T) {
	recovered := false
	calls := 0
	f := registerRecoveryFetcher(t, "", func(context.Context, *http.Client) (string, error) {
		calls++
		if recovered {
			return "203.0.113.8", nil
		}
		return "not-an-IP", nil
	})
	for i := 0; i <= maxErrorCount; i++ {
		GetExternalIP(nil, []string{f.Key()}, nil, "")
	}
	recovered = true
	if _, err, _ := GetExternalIP(nil, []string{f.Key()}, nil, ""); !errors.Is(err, ErrNoAvailableFetchers) {
		t.Fatalf("expected explicit cooldown error, got %v", err)
	}
	if calls != maxErrorCount+1 {
		t.Fatalf("cooled API was queried: %d calls", calls)
	}
	// Advance only this API's deadline; the test never waits for a real cooldown.
	fetchersMux.Lock()
	_, registered := ipFetchers[f.Name()]
	cooldownUntil[f.Name()] = time.Now().Add(-time.Second)
	fetchersMux.Unlock()
	if !registered {
		t.Fatal("API was permanently removed")
	}
	if ip, err, _ := GetExternalIP(nil, []string{f.Key()}, nil, ""); err != nil || ip != "203.0.113.8" {
		t.Fatalf("API failed to recover after cooldown: ip=%q err=%v", ip, err)
	}
}

func TestRecoveryRefusedProxyCanRestart(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "203.0.113.9")
	}))
	address := server.Listener.Addr().String()
	server.Listener.Close()
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	proxy := &detector.ProxyConfig{Enabled: true, Type: "port", Address: host, Port: port}
	f := registerRecoveryFetcher(t, "", func(ctx context.Context, client *http.Client) (string, error) {
		return fetchRecoveryIP(ctx, client, "http://example.invalid/ip")
	})
	_, err, _ = GetExternalIP(proxy, []string{f.Key()}, nil, "")
	if !errors.Is(err, ErrProxyUnavailable) {
		t.Fatalf("proxy refusal was not classified: %v", err)
	}
	if errorCounts[f.Name()] != 0 {
		t.Fatal("proxy refusal penalized the API")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	server.Listener = listener
	server.Start()
	defer server.Close()
	if ip, err, _ := GetExternalIP(proxy, []string{f.Key()}, nil, ""); err != nil || ip != "203.0.113.9" {
		t.Fatalf("proxy restart did not recover: ip=%q err=%v", ip, err)
	}
}

func TestRecoveryCancellationAbortsHTTPQuery(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	f := registerRecoveryFetcher(t, "", func(ctx context.Context, client *http.Client) (string, error) {
		return fetchRecoveryIP(ctx, client, server.URL)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err, _ := GetExternalIPContext(ctx, nil, []string{f.Key()}, nil, ""); done <- err }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("query did not honor cancellation")
	}
}

func TestRecoveryEmptySelectionReturnsError(t *testing.T) {
	if _, err, _ := GetExternalIP(nil, []string{t.Name()}, nil, ""); !errors.Is(err, ErrNoAvailableFetchers) {
		t.Fatalf("expected no API error, got %v", err)
	}
}

package monitor

import (
	"context"
	"errors"
	"testing"
	"time"

	"netgap/internal/config"
	"netgap/internal/detector"
	"netgap/internal/ipchecker/types"
	"netgap/internal/iproute"
)

type recoveryResolver struct{}

func (recoveryResolver) Resolve(ip string) (*types.IPInfo, error) {
	return &types.IPInfo{IP: ip, Region: "test-local"}, nil
}

func TestPendingBackoffStillQueriesWithoutNewDisconnect(t *testing.T) {
	c := newRecoveryChecker()
	c.state = nil
	c.pending = &iproute.DisconnectTask{ID: "test-capability"}
	c.pendingLoaded = true
	c.nextRetry = time.Now().Add(time.Hour)
	queries := 0
	c.lookupIP = func(context.Context, *detector.ProxyConfig, []string, []string, string) (string, error, string) {
		queries++
		return "203.0.113.12", nil, "test"
	}
	for range 3 {
		_, err, _ := c.CheckProxyStatus(context.Background(), &config.Config{Interval: 1, Shutdown: true, Regions: []string{"test-local"}})
		var pending *iproute.PendingDisconnectError
		if !errors.As(err, &pending) {
			t.Fatal(err)
		}
	}
	if queries != 3 || c.pending == nil || c.pending.ID != "test-capability" {
		t.Fatal("pending task starved queries or changed scope", queries)
	}
	if h := c.Health(); h.LastSuccessAt == 0 || h.ConsecutiveFailures != 0 {
		t.Fatal(h)
	}
}

func newRecoveryChecker() *Checker {
	c := NewChecker(recoveryResolver{})
	c.state = func(context.Context) (string, error) { return "fixture-network", nil }
	c.detectProxy = func(context.Context) (*detector.ProxyConfig, string, error) {
		return &detector.ProxyConfig{}, "none", nil
	}
	c.lookupIP = func(context.Context, *detector.ProxyConfig, []string, []string, string) (string, error, string) {
		return "203.0.113.12", nil, "test"
	}
	return c
}

func TestPendingResumeFailureStillQueries(t *testing.T) {
	c := newRecoveryChecker()
	c.pending = &iproute.DisconnectTask{ID: "fixture"}
	c.pendingLoaded = true
	resumes, queries := 0, 0
	c.resumeDisconnect = func(ctx context.Context, task *iproute.DisconnectTask) error {
		resumes++
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing action deadline")
		}
		return &iproute.PendingDisconnectError{Task: task, Err: ErrRoutePermission}
	}
	c.lookupIP = func(context.Context, *detector.ProxyConfig, []string, []string, string) (string, error, string) {
		queries++
		return "203.0.113.12", nil, "test"
	}
	for range 2 {
		_, err, _ := c.CheckProxyStatus(context.Background(), &config.Config{Interval: 1, Shutdown: true, Regions: []string{"test-local"}})
		var pending *iproute.PendingDisconnectError
		if !errors.As(err, &pending) {
			t.Fatal(err)
		}
	}
	if resumes != 1 || queries != 2 || c.pending.ID != "fixture" {
		t.Fatal(resumes, queries)
	}
	if h := c.Health(); h.LastSuccessAt == 0 || h.QueryFailure != "" || h.DisconnectFailure == "" {
		t.Fatal(h)
	}
}

func TestRecoveryCLIContinuesAfterRouteFailure(t *testing.T) {
	c := newRecoveryChecker()
	removals := 0
	c.removeRoute = func() error {
		removals++
		if removals == 1 {
			return errors.New("temporary route failure")
		}
		return nil
	}
	ticks := make(chan time.Time, 3)
	for i := 0; i < 3; i++ {
		ticks <- time.Now()
	}
	close(ticks)
	runMonitoring(context.Background(), &config.Config{Regions: []string{"test-local"}, Shutdown: true}, c, ticks)
	if removals != 2 {
		t.Fatalf("expected retry and stop only on success, got %d attempts", removals)
	}
}

func TestRecoveryCLIRouteFailureReturnsError(t *testing.T) {
	c := newRecoveryChecker()
	want := errors.New("route failure")
	c.removeRoute = func() error { return want }
	_, err, _ := c.CheckProxyStatus(context.Background(), &config.Config{Regions: []string{"test-local"}, Shutdown: true})
	if !errors.Is(err, want) {
		t.Fatalf("route failure was hidden: %v", err)
	}
}

func TestRecoveryCheckerPropagatesCancellation(t *testing.T) {
	c := newRecoveryChecker()
	ctx, cancel := context.WithCancel(context.Background())
	c.lookupIP = func(received context.Context, _ *detector.ProxyConfig, _ []string, _ []string, _ string) (string, error, string) {
		if received.Done() != ctx.Done() {
			t.Error("lookup did not receive the monitor context")
		}
		cancel()
		return "", received.Err(), "test"
	}
	defer cancel()
	_, err := c.CheckProxyStatusResultContext(ctx, &config.Config{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

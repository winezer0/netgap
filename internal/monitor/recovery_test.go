package monitor

import (
	"context"
	"errors"
	"testing"
	"time"

	"netgap/internal/config"
	"netgap/internal/detector"
	"netgap/internal/ipchecker/types"
)

type recoveryResolver struct{}

func (recoveryResolver) Resolve(ip string) (*types.IPInfo, error) {
	return &types.IPInfo{IP: ip, Region: "test-local"}, nil
}

func newRecoveryChecker() *Checker {
	c := NewChecker(recoveryResolver{})
	c.lookupIP = func(context.Context, *detector.ProxyConfig, []string, []string, string) (string, error, string) {
		return "203.0.113.12", nil, "test"
	}
	return c
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
	runMonitoring(&config.Config{Regions: []string{"test-local"}, Shutdown: true}, c, ticks)
	if removals != 2 {
		t.Fatalf("expected retry and stop only on success, got %d attempts", removals)
	}
}

func TestRecoveryCLIRouteFailureReturnsError(t *testing.T) {
	c := newRecoveryChecker()
	want := errors.New("route failure")
	c.removeRoute = func() error { return want }
	_, err, _ := c.CheckProxyStatus(&config.Config{Regions: []string{"test-local"}, Shutdown: true})
	if !errors.Is(err, want) {
		t.Fatalf("route failure was hidden: %v", err)
	}
}

func TestRecoveryCheckerPropagatesCancellation(t *testing.T) {
	c := newRecoveryChecker()
	ctx, cancel := context.WithCancel(context.Background())
	c.lookupIP = func(received context.Context, _ *detector.ProxyConfig, _ []string, _ []string, _ string) (string, error, string) {
		if received != ctx {
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

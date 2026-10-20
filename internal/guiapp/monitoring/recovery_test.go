package monitoring

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"netgap/internal/config"
	"netgap/internal/detector"
	"netgap/internal/ipchecker"
	"netgap/internal/ipchecker/types"
	monitorcore "netgap/internal/monitor"
)

func awaitRunSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("monitor did not reach expected state")
	}
}

func currentRun(m *Manager) *monitorRun {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

func localRecoveryResult() *monitorcore.CheckResult {
	return &monitorcore.CheckResult{IPInfo: &types.IPInfo{IP: "203.0.113.10"}, IsLocal: true}
}

func TestRecoveryStopCancelsPendingCheck(t *testing.T) {
	entered := make(chan struct{})
	m := NewManager(nil, func(ctx context.Context, cfg *config.Config) (*monitorcore.CheckResult, error) {
		close(entered)
		<-ctx.Done()
		return localRecoveryResult(), nil
	})
	var removals atomic.Int32
	m.removeRoute = func() (string, error) { removals.Add(1); return "", nil }
	if err := m.Start(&config.Config{Interval: 60, RemoveRouteOnMatch: true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Stop() })
	run := currentRun(m)
	awaitRunSignal(t, entered)
	if !m.Stop() {
		t.Fatal("stop failed")
	}
	awaitRunSignal(t, run.done)
	if removals.Load() != 0 {
		t.Fatal("a stopped check removed the route")
	}
	if m.Running() {
		t.Fatal("stopped monitor still reported running")
	}
}

func TestRecoveryOldRunCannotStopNewRun(t *testing.T) {
	oldEntered, newEntered := make(chan struct{}), make(chan struct{})
	releaseOld := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-releaseOld:
		default:
			close(releaseOld)
		}
	})
	var calls, removals atomic.Int32
	m := NewManager(nil, func(ctx context.Context, cfg *config.Config) (*monitorcore.CheckResult, error) {
		if calls.Add(1) == 1 {
			close(oldEntered)
			<-releaseOld // Simulate a dependency returning after cancellation.
			return localRecoveryResult(), nil
		}
		close(newEntered)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	m.removeRoute = func() (string, error) { removals.Add(1); return "", nil }
	cfg := &config.Config{Interval: 60, RemoveRouteOnMatch: true}
	if err := m.Start(cfg); err != nil {
		t.Fatal(err)
	}
	old := currentRun(m)
	t.Cleanup(func() { m.Stop() })
	awaitRunSignal(t, oldEntered)
	if !m.Stop() {
		t.Fatal("old run did not stop")
	}
	if err := m.Start(cfg); err != nil {
		t.Fatal(err)
	}
	next := currentRun(m)
	awaitRunSignal(t, newEntered)
	close(releaseOld)
	awaitRunSignal(t, old.done)
	if !m.Running() || currentRun(m) != next {
		t.Fatal("old completion corrupted the new run")
	}
	if removals.Load() != 0 {
		t.Fatal("old result performed route removal")
	}
	if !m.Stop() {
		t.Fatal("new run could not be stopped")
	}
	awaitRunSignal(t, next.done)
}

func TestRecoveryFailedRouteRemovalRetries(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var checks atomic.Int32
	m := NewManager(nil, func(context.Context, *config.Config) (*monitorcore.CheckResult, error) {
		if checks.Add(1) == 1 {
			close(entered)
			<-release
		}
		return localRecoveryResult(), nil
	})
	m.tickDuration = func(int) time.Duration { return 5 * time.Millisecond }
	var removals atomic.Int32
	m.removeRoute = func() (string, error) {
		if removals.Add(1) == 1 {
			return "", errors.New("temporary route failure")
		}
		return "removed", nil
	}
	if err := m.Start(&config.Config{Interval: 1, RemoveRouteOnMatch: true}); err != nil {
		t.Fatal(err)
	}
	run := currentRun(m)
	t.Cleanup(func() { m.Stop() })
	awaitRunSignal(t, entered)
	close(release)
	awaitRunSignal(t, run.done)
	if removals.Load() != 2 {
		t.Fatalf("expected retry then success, got %d attempts", removals.Load())
	}
	if m.Running() {
		t.Fatal("monitor did not stop after successful protection")
	}
}

func TestRecoveryDirectSuccessDoesNotReportProxyRestored(t *testing.T) {
	var logs []string
	calls := 0
	m := NewManager(func(level, message string) { logs = append(logs, message) }, func(context.Context, *config.Config) (*monitorcore.CheckResult, error) {
		calls++
		if calls == 1 {
			return nil, ipchecker.ErrProxyUnavailable
		}
		return &monitorcore.CheckResult{IPInfo: &types.IPInfo{IP: "203.0.113.11"}, Proxy: &detector.ProxyConfig{}}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := &monitorRun{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	m.current = run
	m.checkOnce(run, &config.Config{})
	m.checkOnce(run, &config.Config{})
	if strings.Contains(strings.Join(logs, "\n"), "Proxy connection restored") {
		t.Fatal("direct success reported proxy recovery")
	}
}

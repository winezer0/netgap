package monitoring

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"netgap/internal/config"
	"netgap/internal/ipchecker"
	monitorcore "netgap/internal/monitor"
)

// A bounded accelerated stress test, not a claim of hours of desktop uptime.
func TestStabilityRepeatedRestartWithConcurrentObservers(t *testing.T) {
	entered := make(chan struct{}, 1)
	var activeChecks, completed, removals atomic.Int32
	m := NewManager(nil, func(ctx context.Context, _ *config.Config) (*monitorcore.CheckResult, error) {
		activeChecks.Add(1)
		defer activeChecks.Add(-1)
		entered <- struct{}{}
		<-ctx.Done()
		completed.Add(1)
		return localRecoveryResult(), nil // Simulate a late successful result.
	})
	m.removeRoute = func() (string, error) { removals.Add(1); return "", nil }
	readerStop, readerDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-readerStop:
				return
			default:
				_ = m.Health()
				_ = m.Running()
				m.SetRemoveRouteOnMatch(false)
				runtime.Gosched()
			}
		}
	}()
	t.Cleanup(func() {
		m.Stop()
		close(readerStop)
		awaitRunSignal(t, readerDone)
	})
	cfg := &config.Config{Interval: 60}
	if err := m.Start(cfg); err != nil {
		t.Fatal(err)
	}
	old := currentRun(m)
	awaitRunSignal(t, entered)
	for cycle := range 200 {
		id := m.Health().RunID
		if !m.Stop() {
			t.Fatal("stop failed", cycle)
		}
		if err := m.Start(cfg); err != nil {
			t.Fatal(err)
		}
		next := currentRun(m)
		awaitRunSignal(t, entered)
		awaitRunSignal(t, old.done)
		health := m.Health()
		if !m.Running() || currentRun(m) != next || health.RunID <= id || !health.Running {
			t.Fatal("old completion corrupted restarted monitor", cycle, health)
		}
		old = next
	}
	if !m.Stop() {
		t.Fatal("last stop failed")
	}
	awaitRunSignal(t, old.done)
	if activeChecks.Load() != 0 || completed.Load() != 201 || removals.Load() != 0 {
		t.Fatal("checks leaked or late results removed routes", activeChecks.Load(), completed.Load(), removals.Load())
	}
}

func TestStabilityFaultStormRecoversWithoutRouteOperations(t *testing.T) {
	queries, removals := 0, 0
	m := NewManager(nil, func(context.Context, *config.Config) (*monitorcore.CheckResult, error) {
		queries++
		switch queries % 4 {
		case 1:
			return nil, errors.New("simulated query failure")
		case 2:
			return nil, context.DeadlineExceeded
		case 3:
			return nil, ipchecker.ErrProxyUnavailable
		default:
			return localRecoveryResult(), nil
		}
	})
	m.removeRoute = func() (string, error) { removals++; return "", nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := &monitorRun{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	run.health.Start(1)
	run.health.SetDisconnect("failed", monitorcore.ErrRoutePermission, time.Time{})
	m.current = run
	m.active.Store(run)
	m.healthRun.Store(run)
	for cycle := range 10000 {
		if m.checkOnce(run, &config.Config{Interval: 1}) {
			t.Fatal("fault or recovery stopped checks", cycle)
		}
		h := m.Health()
		if h.ConsecutiveFailures != uint64(queries%4) || h.DisconnectFailure == "" {
			t.Fatal("query recovery corrupted health", cycle, h)
		}
	}
	h := m.Health()
	if queries != 10000 || removals != 0 || h.Phase != "healthy" || h.LastSuccessAt == 0 || h.QueryFailure != "" {
		t.Fatal(queries, removals, h)
	}
}

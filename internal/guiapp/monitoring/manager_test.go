package monitoring

import (
	"context"
	"sync"
	"testing"
	"time"

	"netgap/internal/config"
	"netgap/internal/ipchecker/types"
	monitorcore "netgap/internal/monitor"
)

// TestManagerStartStop 验证监测管理器的启动与停止。
func TestManagerStartStop(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	logs := make([]string, 0, 4)
	manager := NewManager(func(level, message string) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, level+":"+message)
	})
	manager.check = func(ctx context.Context, cfg *config.Config) (*monitorcore.CheckResult, error) {
		return &monitorcore.CheckResult{
			FetcherName: "mock",
			IPInfo:      &types.IPInfo{IP: "1.2.3.4", Region: "湖南", City: "长沙"},
			IsLocal:     false,
		}, nil
	}
	manager.removeRoute = func() (string, error) {
		return "mock removed", nil
	}
	manager.tickDuration = func(seconds int) time.Duration {
		return 5 * time.Millisecond
	}

	if err := manager.Start(&config.Config{Regions: []string{"湖南"}, Interval: 1}); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if !manager.Running() {
		t.Fatal("expected manager running")
	}
	if err := manager.Start(&config.Config{Regions: []string{"湖南"}, Interval: 1}); err == nil {
		t.Fatal("expected duplicate start to fail")
	}
	if !manager.Stop() {
		t.Fatal("expected stop to return true")
	}
	time.Sleep(15 * time.Millisecond)
	if manager.Stop() {
		t.Fatal("expected second stop to return false")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(logs) == 0 {
		t.Fatal("expected logs to be written")
	}
}

func TestManagerStopShouldNotLogAfterStop(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	logCount := 0
	manager := NewManager(func(level, message string) {
		mu.Lock()
		defer mu.Unlock()
		logCount++
	})
	manager.check = func(ctx context.Context, cfg *config.Config) (*monitorcore.CheckResult, error) {
		return &monitorcore.CheckResult{
			FetcherName: "mock",
			IPInfo:      &types.IPInfo{IP: "1.2.3.4", Region: "湖南", City: "长沙"},
			IsLocal:     false,
		}, nil
	}
	manager.tickDuration = func(seconds int) time.Duration {
		return 5 * time.Millisecond
	}

	if err := manager.Start(&config.Config{Regions: []string{"湖南"}, Interval: 1}); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if !manager.Stop() {
		t.Fatal("expected stop to return true")
	}

	mu.Lock()
	before := logCount
	mu.Unlock()

	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	after := logCount
	mu.Unlock()

	if after > before+1 {
		t.Fatalf("expected no extra logs after stop, before=%d after=%d", before, after)
	}
}

// Force cancellation after result validation but before result publication.
// Stop must not leave a deferred stop message and a stale result to publish.
func TestManagerStopDuringResultPublication(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var logs []string
	manager := NewManager(func(level, message string) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, message)
	}, func(context.Context, *config.Config) (*monitorcore.CheckResult, error) {
		return &monitorcore.CheckResult{
			FetcherName: "mock",
			IPInfo:      &types.IPInfo{IP: "203.0.113.1"},
		}, nil
	})
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		manager.Stop()
	})
	manager.SetHealthListener(func(health monitorcore.Health) {
		if health.Running && health.LastSuccessAt != 0 {
			close(entered)
			<-release
		}
	})
	if err := manager.Start(&config.Config{Interval: 60}); err != nil {
		t.Fatal(err)
	}
	run := currentRun(manager)
	awaitRunSignal(t, entered)
	if !manager.Stop() {
		t.Fatal("stop failed")
	}
	mu.Lock()
	before := len(logs)
	mu.Unlock()
	releaseOnce.Do(func() { close(release) })
	awaitRunSignal(t, run.done)
	mu.Lock()
	defer mu.Unlock()
	if len(logs) != before {
		t.Fatalf("stopped run published more logs: %v", logs[before:])
	}
	stopped := 0
	for _, message := range logs {
		if message == "Monitoring stopped" {
			stopped++
		}
	}
	if stopped != 1 {
		t.Fatalf("expected one stop message, got %d: %v", stopped, logs)
	}
}

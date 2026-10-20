//go:build darwin && cgo && acceptance

package macnetwork

import (
	"context"
	"testing"
	"time"
)

func TestNativeNetworkWatcherRecoversAfterInvalidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if _, err := NetworkState(ctx); err != nil {
		t.Fatalf("initial native network snapshot: %v", err)
	}
	restartNetworkWatcher()

	for {
		if _, err := NetworkState(ctx); err == nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("native network watcher did not recover: %v", ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

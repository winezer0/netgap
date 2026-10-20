package monitor

import (
	"context"
	"testing"

	"netgap/internal/config"
)

func TestCheckerDiscardsResultAfterNetworkOrProxyChange(t *testing.T) {
	c := newRecoveryChecker()
	calls := 0
	c.state = func(context.Context) (string, error) {
		calls++
		if calls == 1 {
			return "old", nil
		}
		return "new", nil
	}
	_, err := c.CheckProxyStatusResultContext(context.Background(), &config.Config{Regions: []string{"test-local"}})
	if err == nil {
		t.Fatal("accepted stale IP result")
	}
}
func TestCheckerRevalidatesImmediatelyBeforeDisconnect(t *testing.T) {
	c := newRecoveryChecker()
	calls := 0
	c.state = func(context.Context) (string, error) {
		calls++
		if calls <= 2 {
			return "old", nil
		}
		return "new", nil
	}
	removed := false
	c.removeRoute = func() error { removed = true; return nil }
	_, err, _ := c.CheckProxyStatus(&config.Config{Regions: []string{"test-local"}, Shutdown: true})
	if err == nil || removed {
		t.Fatal("deleted route for stale result")
	}
}

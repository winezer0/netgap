package monitor

import (
	"context"
	"testing"
	"time"
)

func TestHealthQueryRecoveryPreservesRouteFailure(t *testing.T) {
	var h HealthTracker
	h.Start(1)
	h.SetDisconnect("pending", ErrRoutePermission, time.Now().Add(time.Second))
	h.BeginCheck(1)
	h.CompleteCheck(context.DeadlineExceeded)
	failed := h.Snapshot()
	if failed.ConsecutiveFailures != 1 || failed.LastAttemptAt == 0 || failed.QueryFailure == "" || failed.LastSuccessAt != 0 {
		t.Fatal(failed)
	}
	h.BeginCheck(1)
	h.CompleteCheck(nil)
	recovered := h.Snapshot()
	if recovered.QueryFailure != "" || recovered.ConsecutiveFailures != 0 || recovered.LastSuccessAt == 0 || recovered.DisconnectFailure != failed.DisconnectFailure || recovered.NextRetryAt == 0 {
		t.Fatal(recovered)
	}
	oldRun := recovered.RunID
	h.Stop()
	h.Start(1)
	if next := h.Snapshot(); next.RunID == oldRun || next.LastSuccessAt != 0 || next.DisconnectFailure != "" {
		t.Fatal(next)
	}
}

func TestHealthManualCompletionCannotOverwriteNewAction(t *testing.T) {
	var h HealthTracker
	h.Start(1)
	old := h.RouteRevision()
	h.SetDisconnect("running", nil, time.Time{})
	if h.RecordManualRouteResult(old, "restored", nil) {
		t.Fatal("stale manual result replaced automatic operation")
	}
	current := h.RouteRevision()
	h.BeginCheck(1)
	h.CompleteCheck(nil)
	if !h.RecordManualRouteResult(current, "restored", nil) {
		t.Fatal("query incorrectly invalidated manual result")
	}
	if got := h.Snapshot(); got.DisconnectState != "restored" {
		t.Fatal(got)
	}
}

package monitor

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"netgap/internal/iproute"
)

// DisconnectAttemptTimeout bounds one automatic action, including lock waits.
const DisconnectAttemptTimeout = 15 * time.Second

// Health separates successful exit checks from route protection progress.
// Timestamps are Unix milliseconds; zero means no such observation this run.
type Health struct {
	RunID               uint64 `json:"runId"`
	Revision            uint64 `json:"revision"`
	Running             bool   `json:"running"`
	Phase               string `json:"phase"`
	IntervalSeconds     int    `json:"intervalSeconds"`
	StartedAt           int64  `json:"startedAt"`
	LastAttemptAt       int64  `json:"lastAttemptAt"`
	LastCompletedAt     int64  `json:"lastCompletedAt"`
	LastSuccessAt       int64  `json:"lastSuccessAt"`
	ConsecutiveFailures uint64 `json:"consecutiveFailures"`
	QueryFailure        string `json:"queryFailure"`
	ObservationOnly     string `json:"observationOnly"`
	DisconnectState     string `json:"disconnectState"`
	DisconnectFailure   string `json:"disconnectFailure"`
	NextRetryAt         int64  `json:"nextRetryAt"`
}

type HealthTracker struct {
	mu            sync.RWMutex
	data          Health
	routeRevision uint64
}

var healthRunSequence atomic.Uint64

func (h *HealthTracker) Start(interval int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data = Health{RunID: healthRunSequence.Add(1), Revision: 1, Running: true, Phase: "waiting", IntervalSeconds: interval, StartedAt: time.Now().UnixMilli(), DisconnectState: "idle"}
	h.routeRevision++
}

func (h *HealthTracker) BeginCheck(interval int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.data.StartedAt == 0 {
		h.data.RunID = healthRunSequence.Add(1)
		h.data.StartedAt = time.Now().UnixMilli()
	}
	h.data.Revision++
	h.data.Running = true
	h.data.Phase = "checking"
	h.data.IntervalSeconds = interval
	h.data.LastAttemptAt = time.Now().UnixMilli()
}

func (h *HealthTracker) CompleteCheck(err error) {
	h.CompleteObservation(err, "")
}

func (h *HealthTracker) CompleteObservation(err error, reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data.LastCompletedAt = time.Now().UnixMilli()
	h.data.Revision++
	h.data.ObservationOnly = reason
	if err != nil {
		h.data.Phase = "degraded"
		h.data.QueryFailure = FailureKind(err)
		h.data.ConsecutiveFailures++
		return
	}
	h.data.Phase = "healthy"
	if reason != "" {
		h.data.Phase = "observation_only"
	}
	h.data.LastSuccessAt = h.data.LastCompletedAt
	h.data.QueryFailure = ""
	h.data.ConsecutiveFailures = 0
}

func (h *HealthTracker) SetDisconnect(state string, err error, next time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.setDisconnect(state, err, next)
}

func (h *HealthTracker) setDisconnect(state string, err error, next time.Time) {
	h.routeRevision++
	h.data.DisconnectState = state
	h.data.Revision++
	h.data.DisconnectFailure = ""
	h.data.NextRetryAt = 0
	if err != nil {
		var pending *iproute.PendingDisconnectError
		if errors.As(err, &pending) {
			err = pending.Err
		}
		h.data.DisconnectFailure = FailureKind(err)
	}
	if !next.IsZero() {
		h.data.NextRetryAt = next.UnixMilli()
	}
}

func (h *HealthTracker) RouteRevision() uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.routeRevision
}

// Manual results may update the display only if no newer automatic action
// started in the meantime. Query progress does not interfere with this check.
func (h *HealthTracker) RecordManualRouteResult(expected uint64, state string, err error) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.routeRevision != expected {
		return false
	}
	if err != nil {
		state = "failed"
	}
	h.setDisconnect(state, err, time.Time{})
	return true
}

func (h *HealthTracker) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.data.Running, h.data.Phase = false, "stopped"
	h.data.Revision++
}

func (h *HealthTracker) Snapshot() Health {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := h.data
	if result.Phase == "" {
		result.Phase = "stopped"
	}
	if result.DisconnectState == "" {
		result.DisconnectState = "idle"
	}
	return result
}

func (h Health) String() string {
	stamp := func(value int64) string {
		if value == 0 {
			return "never"
		}
		return time.UnixMilli(value).Format(time.RFC3339)
	}
	return fmt.Sprintf("phase=%s last_attempt=%s last_success=%s consecutive_failures=%d query_error=%s disconnect=%s route_error=%s next_retry=%s observation_only=%s",
		h.Phase, stamp(h.LastAttemptAt), stamp(h.LastSuccessAt), h.ConsecutiveFailures,
		h.QueryFailure, h.DisconnectState, h.DisconnectFailure, stamp(h.NextRetryAt), h.ObservationOnly)
}

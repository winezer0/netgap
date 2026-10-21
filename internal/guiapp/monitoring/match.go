package monitoring

import (
	"time"

	"netgap/internal/monitor"
)

// MatchEvent is a validated observation, independent of log wording.
type MatchEvent struct {
	RunID      uint64
	ObservedAt time.Time
	Matched    bool
	IP         string
	Location   string
	Path       string
	Protection string
}

func (m *Manager) SetMatchListener(listener func(MatchEvent)) {
	if listener == nil {
		m.matchListener.Store(nil)
		return
	}
	m.matchListener.Store(&listener)
}

func (m *Manager) notifyMatch(run *monitorRun, result *monitor.CheckResult) {
	listener := m.matchListener.Load()
	if listener == nil || !m.isCurrent(run) {
		return
	}
	protection := "alerts_only"
	switch {
	case result.ObservationOnly != "":
		protection = "pac_observation"
	case m.removeRouteOnMatch.Load() && run.permissionLimited:
		protection = "permission"
	case m.removeRouteOnMatch.Load():
		protection = "pending"
	}
	(*listener)(MatchEvent{RunID: run.health.Snapshot().RunID, ObservedAt: time.Now(),
		Matched: result.IsLocal, IP: result.IPInfo.IP, Location: result.LocationTag(),
		Path: result.PathTag(), Protection: protection})
}

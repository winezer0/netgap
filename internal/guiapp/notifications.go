package guiapp

import (
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"netgap/internal/guiapp/monitoring"
	"netgap/internal/monitor"
)

const matchReminderInterval = time.Minute
const matchChangeInterval = 3 * time.Second

func (s *Service) cancelAlertTimerLocked() {
	s.alertTimerRevision++
	if s.alertTimer != nil {
		s.alertTimer.Stop()
		s.alertTimer = nil
	}
	s.pendingAlert = nil
	s.nextDelivery = nil
}

func (s *Service) observeAlertHealth(health monitor.Health) {
	s.alertMu.Lock()
	if health.RunID < s.alertRunID {
		s.alertMu.Unlock()
		return
	}
	if health.RunID != s.alertRunID {
		s.cancelAlertTimerLocked()
		s.alertRunID = health.RunID
		s.alertKey, s.lastNotice = "", time.Time{}
		if s.lastAlert != nil {
			s.lastAlert.Active = false
		}
	}
	if !health.Running {
		s.cancelAlertTimerLocked()
		if s.lastAlert != nil {
			s.lastAlert.Active = false
		}
	}
	if s.lastAlert != nil && s.lastAlert.RunID == health.RunID {
		switch health.DisconnectState {
		case "completed", "failed", "pending", "running":
			s.lastAlert.Protection = health.DisconnectState
		case "suspended":
			if health.DisconnectFailure == "permission" {
				s.lastAlert.Protection = "permission"
			}
		}
	}
	s.alertMu.Unlock()
	s.emitAlertState()
}

func (s *Service) observeMatch(event monitoring.MatchEvent) {
	if s.monitor == nil || s.closing.Load() {
		return
	}
	health := s.monitor.Health()
	if !health.Running || health.RunID != event.RunID {
		return
	}
	settings := s.Settings()
	s.alertMu.Lock()
	if event.RunID < s.alertRunID {
		s.alertMu.Unlock()
		return
	}
	if event.RunID != s.alertRunID {
		s.cancelAlertTimerLocked()
		s.alertRunID = event.RunID
		s.alertKey, s.lastNotice = "", time.Time{}
	}
	if !event.Matched {
		s.cancelAlertTimerLocked()
		s.alertKey, s.lastNotice = "", time.Time{}
		if s.lastAlert != nil {
			s.lastAlert.Active = false
		}
		s.alertMu.Unlock()
		s.emitAlertState()
		return
	}
	key := event.IP + "\x00" + event.Location + "\x00" + event.Path
	changed := key != s.alertKey
	timestamp := event.ObservedAt.UnixMilli()
	if changed || s.lastAlert == nil {
		s.alertSequence++
	} else {
		timestamp = s.lastAlert.Timestamp
	}
	alert := Alert{Type: "error", EventID: s.alertSequence, RunID: event.RunID,
		Timestamp: timestamp, ObservedAt: event.ObservedAt.UnixMilli(), IP: event.IP, Location: event.Location,
		Path: event.Path, Protection: event.Protection, Active: true}
	s.alertKey, s.lastAlert = key, &alert
	if !settings.NotificationPopup {
		s.cancelAlertTimerLocked()
		s.alertMu.Unlock()
		s.emitAlertState()
		return
	}
	due := s.lastNotice.Add(matchReminderInterval)
	if changed {
		due = s.lastNotice.Add(matchChangeInterval)
	}
	// Always replace the queued observation with the newest one. The fixed
	// timer deadline must not slide forward on every monitoring tick.
	s.pendingAlert = &alert
	if s.lastNotice.IsZero() || !time.Now().Before(due) {
		s.cancelAlertTimerLocked()
		s.lastNotice, s.alertUnread = time.Now(), true
		s.alertMu.Unlock()
		s.emitAlertState()
		go s.deliverAlert(alert)
		return
	}
	if changed && s.alertTimer != nil {
		s.alertTimerRevision++
		s.alertTimer.Stop()
		s.alertTimer = nil
	}
	if s.alertTimer == nil {
		runID := event.RunID
		s.alertTimerRevision++
		revision := s.alertTimerRevision
		s.alertTimer = time.AfterFunc(time.Until(due), func() { s.deliverPendingAlert(runID, revision) })
	}
	s.alertMu.Unlock()
	s.emitAlertState()
}

func (s *Service) deliverPendingAlert(runID, revision uint64) {
	s.alertMu.Lock()
	if s.alertRunID != runID || revision != s.alertTimerRevision || s.pendingAlert == nil {
		s.alertMu.Unlock()
		return
	}
	alert := *s.pendingAlert
	s.alertTimer, s.pendingAlert = nil, nil
	s.lastNotice, s.alertUnread = time.Now(), true
	s.alertMu.Unlock()
	s.deliverAlert(alert)
}

func (s *Service) alertCanDeliver(alert Alert) bool {
	if s.ctx == nil || s.ctx.Err() != nil || s.closing.Load() || s.monitor == nil || !s.Settings().NotificationPopup {
		return false
	}
	health := s.monitor.Health()
	if health.RunID != alert.RunID || (!health.Running && health.DisconnectState != "completed") {
		return false
	}
	s.alertMu.Lock()
	defer s.alertMu.Unlock()
	return s.lastAlert != nil && s.lastAlert.EventID == alert.EventID &&
		(s.lastAlert.Active || health.DisconnectState == "completed")
}

func (s *Service) deliverAlert(alert Alert) {
	if !s.alertCanDeliver(alert) {
		return
	}
	if !s.notificationBusy.CompareAndSwap(false, true) {
		s.alertMu.Lock()
		if s.nextDelivery == nil || s.nextDelivery.EventID < alert.EventID {
			queued := alert
			s.nextDelivery = &queued
		}
		s.alertMu.Unlock()
		return
	}
	defer func() {
		s.alertMu.Lock()
		next := s.nextDelivery
		s.nextDelivery = nil
		s.notificationBusy.Store(false)
		s.alertMu.Unlock()
		if next != nil {
			go s.deliverAlert(*next)
		}
	}()
	s.alertMu.Lock()
	if s.lastAlert == nil || s.lastAlert.EventID != alert.EventID {
		s.alertMu.Unlock()
		return
	}
	alert.Protection = s.lastAlert.Protection
	s.alertMu.Unlock()
	alert.Message = s.formatMatchAlert(alert)
	// The popup setting always selects desktop notifications. Window focus,
	// minimisation and tray visibility must not suppress or replace delivery.
	if !s.alertCanDeliver(alert) {
		return
	}
	if err := notifyDesktop(notificationTitle, alert.Message); err != nil {
		s.alertMu.Lock()
		logError := time.Since(s.lastNoticeError) >= matchReminderInterval
		if logError {
			s.lastNoticeError = time.Now()
		}
		s.alertMu.Unlock()
		if logError {
			s.writeLog("WARN", "Failed to show desktop notification; latest alert retained: "+err.Error())
		}
		s.emitAlertState()
		return
	}
	s.LogUIError(alert.Message)
	s.markAlertDelivered(alert.EventID)
}

func (s *Service) markAlertDelivered(eventID uint64) {
	s.alertMu.Lock()
	if s.lastAlert != nil && s.lastAlert.EventID == eventID {
		s.alertUnread = false
	}
	s.alertMu.Unlock()
	s.emitAlertState()
}

func (s *Service) alertSnapshot() (*Alert, bool, uint64) {
	s.alertMu.Lock()
	defer s.alertMu.Unlock()
	s.alertStateRevision++
	if s.lastAlert == nil {
		return nil, s.alertUnread, s.alertStateRevision
	}
	alert := *s.lastAlert
	return &alert, s.alertUnread, s.alertStateRevision
}

func (s *Service) emitAlertState() {
	if s.ctx == nil || s.closing.Load() {
		return
	}
	alert, unread, revision := s.alertSnapshot()
	runtime.EventsEmit(s.ctx, eventAlertStateName, map[string]any{"latestAlert": alert, "alertUnread": unread, "alertRevision": revision})
}

// SetWindowForeground records WebView focus/visibility changes. Focus changes
// do not deliver, consume or replace desktop notifications.
func (s *Service) SetWindowForeground(foreground bool, revision uint64) {
	s.alertMu.Lock()
	if revision <= s.windowReportRevision {
		s.alertMu.Unlock()
		return
	}
	s.windowReportRevision = revision
	s.windowForeground.Store(foreground)
	s.alertMu.Unlock()
}

// ShowLastAlert is an explicit user action, including when automatic popups
// are disabled. Preserve the original occurrence time and mark it as history.
func (s *Service) ShowLastAlert() {
	if s.ctx == nil || s.ctx.Err() != nil || s.closing.Load() || !s.lastAlertDialogOpen.CompareAndSwap(false, true) {
		return
	}
	defer s.lastAlertDialogOpen.Store(false)
	s.ShowWindow()
	options, eventID := s.lastAlertDialogOptions()
	// A native dialog stays visible until dismissed and does not depend on
	// WebView event subscription timing or the automatic popup setting.
	if _, err := runtime.MessageDialog(s.ctx, options); err != nil {
		s.writeLog("WARN", "Failed to show latest alert; record retained: "+err.Error())
		return
	}
	if eventID != 0 {
		s.markAlertDelivered(eventID)
	}
}

func (s *Service) lastAlertDialogOptions() (runtime.MessageDialogOptions, uint64) {
	texts := textsFor(s.Settings().Language)
	options := runtime.MessageDialogOptions{
		Type: runtime.InfoDialog, Title: texts.LatestAlertTitle, Message: texts.NoRecentAlert,
	}
	alert, _, _ := s.alertSnapshot()
	if alert == nil {
		return options, 0
	}
	alert.Historical = true
	options.Message = formatMatchAlertWithTexts(*alert, texts)
	return options, alert.EventID
}

func (s *Service) formatMatchAlert(alert Alert) string {
	return formatMatchAlertWithTexts(alert, textsFor(s.Settings().Language))
}

func formatMatchAlertWithTexts(alert Alert, texts guiTexts) string {
	status := texts.Protection[alert.Protection]
	if status == "" {
		status = texts.UnknownProtection
	}
	prefix := texts.MatchedRegion
	if alert.Historical {
		prefix = texts.HistoricalAlert
		if alert.Active {
			status += texts.StillMatched
		} else {
			status += texts.NoActiveMatch
		}
	}
	return fmt.Sprintf("%s · %s · IP %s · %s · %s · %s", prefix,
		time.UnixMilli(alert.Timestamp).Format("2006-01-02 15:04:05"), alert.IP, alert.Location, alert.Path, status)
}

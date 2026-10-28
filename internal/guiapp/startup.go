package guiapp

import (
	"time"

	"netgap/internal/guiapp/autostart"
	"netgap/internal/guiapp/configstore"
)

func (s *Service) GetStartupStatus() (*autostart.Status, error) {
	return autostart.GetStatus()
}

func (s *Service) SetStartupEnabled(enabled bool) (*autostart.Status, error) {
	status, err := autostart.SetEnabled(enabled)
	if err != nil {
		s.writeLog("ERROR", "Failed to update login startup: "+err.Error())
		return nil, err
	}
	s.writeLog("INFO", "Login startup registration updated")
	return status, nil
}

// ShouldStartHidden keeps Wails' initial visibility consistent with launch preferences.
func (s *Service) ShouldStartHidden() bool {
	s.launchMu.Lock()
	defer s.launchMu.Unlock()
	return s.restartMonitoring == nil && s.Settings().StartMinimized
}

// MarkFrontendReady and MarkTrayReady can arrive in either order. A tray callback
// during construction must not consume the one-time launch action before Startup.
func (s *Service) MarkFrontendReady() {
	s.launchMu.Lock()
	if s.frontendReady || s.closing.Load() {
		s.launchMu.Unlock()
		return
	}
	s.frontendReady = true
	if !s.trayReady {
		s.launchTimer = time.AfterFunc(5*time.Second, func() {
			s.launchMu.Lock()
			s.trayWaitExpired = true
			s.launchMu.Unlock()
			s.ApplyLaunchPreferences()
		})
	}
	s.launchMu.Unlock()
	go s.ApplyLaunchPreferences()
}

func (s *Service) MarkTrayReady() {
	s.launchMu.Lock()
	s.trayReady = true
	if s.launchTimer != nil {
		s.launchTimer.Stop()
	}
	s.launchMu.Unlock()
	go s.ApplyLaunchPreferences()
}

// ApplyLaunchPreferences waits for a runtime context, a loaded frontend, and a
// usable tray (or the visible-window fallback), then applies preferences once.
func (s *Service) ApplyLaunchPreferences() {
	s.launchMu.Lock()
	defer s.launchMu.Unlock()
	if s.ctx == nil || !s.frontendReady || (!s.trayReady && !s.trayWaitExpired) || s.closing.Load() {
		return
	}
	s.launchOnce.Do(func() {
		settings := s.Settings()
		if s.restartMonitoring != nil {
			settings.AutoStartMonitoring = *s.restartMonitoring
			settings.StartMinimized = false
		}
		if !s.trayReady {
			settings.StartMinimized = false
			s.writeLog("WARN", "Tray was not ready within 5 seconds; showing the main window")
			s.ShowWindow()
		}
		applyLaunchPreferences(settings, func() error {
			if s.monitor.Running() {
				return nil
			}
			_, err := s.StartMonitoring()
			return err
		}, s.HideToTray, func() {
			if !s.closing.Load() {
				s.ShowWindow()
			}
		})
	})
}

func applyLaunchPreferences(settings *configstore.Settings, start func() error, hide, show func()) {
	if settings.StartMinimized {
		hide()
	}
	if settings.AutoStartMonitoring {
		if err := start(); err != nil {
			show()
		}
	}
}

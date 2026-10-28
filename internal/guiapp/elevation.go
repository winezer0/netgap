package guiapp

import (
	"errors"

	"netgap/internal/guiapp/elevation"
)

func (s *Service) GetElevationStatus() *elevation.Status { return elevation.GetStatus() }

func (s *Service) RestartAsAdmin() (*elevation.RestartResult, error) {
	if s.ctx == nil || s.closing.Load() {
		return nil, errors.New("application is not ready for restart")
	}
	if !s.adminRestartBusy.CompareAndSwap(false, true) {
		return nil, errors.New("administrator restart is already in progress")
	}
	if err := elevation.Restart(s.monitor.Running); err != nil {
		s.adminRestartBusy.Store(false)
		if errors.Is(err, elevation.ErrCancelled) {
			s.writeLog("INFO", "Administrator authorization canceled; current application remains running")
			return &elevation.RestartResult{Cancelled: true}, nil
		}
		s.writeLog("ERROR", "Administrator restart failed: "+err.Error())
		return nil, err
	}
	s.writeLog("INFO", "Administrator process launched; closing current application for restart")
	s.QuitApp()
	return &elevation.RestartResult{}, nil
}

// ConfigureElevatedRestart only overrides this launch, without changing saved preferences.
func (s *Service) ConfigureElevatedRestart(resumeMonitoring bool) {
	s.launchMu.Lock()
	defer s.launchMu.Unlock()
	s.restartMonitoring = &resumeMonitoring
}

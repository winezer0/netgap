package guiapp

import (
	"context"
	"errors"
	"fmt"
	"netgap/internal/guiapp/updater"
	"time"
)

func (s *Service) CheckForUpdates(currentVersion string) (map[string]any, error) {
	manager, err := s.getUpdateManager()
	if err != nil {
		s.recordUpdateCheckCompletion()
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.operationContext(), 30*time.Second)
	defer cancel()
	result, checkErr := manager.Check(ctx, currentVersion)
	checkedAt := s.recordUpdateCheckCompletion()
	if checkErr != nil {
		return nil, checkErr
	}
	return map[string]any{
		"available":      result.Available,
		"currentVersion": result.CurrentVersion,
		"latestVersion":  result.LatestVersion,
		"releaseUrl":     result.ReleaseURL,
		"lastCheckedAt":  checkedAt.Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) recordUpdateCheckCompletion() time.Time {
	checkedAt := time.Now().UTC()
	if err := s.recordUpdateCheckAt(checkedAt); err != nil {
		s.writeLog("WARN", "Failed to persist last update check time: "+err.Error())
	}
	return checkedAt
}

func (s *Service) recordUpdateCheckAt(checkedAt time.Time) error {
	s.mu.Lock()
	settings := cloneSettings(s.settings)
	settings.LastUpdateCheckAt = checkedAt.UTC().Format(time.RFC3339Nano)
	s.settings = settings
	var err error
	if s.store != nil {
		err = s.store.Save(settings)
	}
	s.mu.Unlock()

	s.emitState()
	return err
}

func (s *Service) DownloadUpdate() (map[string]any, error) {
	manager, err := s.getUpdateManager()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.operationContext(), 10*time.Minute)
	defer cancel()
	result, err := manager.Download(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"available":     result.Available,
		"downloaded":    result.Downloaded,
		"latestVersion": result.LatestVersion,
		"releaseUrl":    result.ReleaseURL,
	}, nil
}

func (s *Service) GetPendingUpdate(currentVersion string) map[string]any {
	manager, err := s.getUpdateManager()
	if err != nil {
		return map[string]any{"available": false, "error": err.Error()}
	}
	result := manager.Pending(currentVersion)
	if result == nil {
		return map[string]any{"available": false}
	}
	return map[string]any{
		"available":     result.Available,
		"downloaded":    result.Downloaded,
		"latestVersion": result.LatestVersion,
		"releaseUrl":    result.ReleaseURL,
	}
}

func (s *Service) InstallUpdate() error {
	manager, err := s.getUpdateManager()
	if err != nil {
		return err
	}
	if err := manager.Install(); err != nil {
		return err
	}
	s.QuitApp()
	return nil
}

func (s *Service) getUpdateManager() (*updater.Manager, error) {
	if s.updateManager != nil {
		return s.updateManager, nil
	}
	if s.updateManagerErr != nil {
		return nil, fmt.Errorf("initialize updater: %w", s.updateManagerErr)
	}
	return nil, errors.New("updater is unavailable")
}

package guiapp

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"netgap/internal/guiapp/configstore"
)

func TestLaunchPreferences(t *testing.T) {
	for _, test := range []struct {
		name                  string
		auto, minimized, fail bool
		want                  []string
	}{
		{"default", false, false, false, nil},
		{"monitor", true, false, false, []string{"start"}},
		{"tray", false, true, false, []string{"hide"}},
		{"both", true, true, false, []string{"hide", "start"}},
		{"failure", true, true, true, []string{"hide", "start", "show"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls []string
			applyLaunchPreferences(&configstore.Settings{AutoStartMonitoring: test.auto, StartMinimized: test.minimized}, func() error {
				calls = append(calls, "start")
				if test.fail {
					return errors.New("failed")
				}
				return nil
			}, func() { calls = append(calls, "hide") }, func() { calls = append(calls, "show") })
			if !reflect.DeepEqual(calls, test.want) {
				t.Fatalf("calls = %v, want %v", calls, test.want)
			}
		})
	}
}

func TestLaunchPreferencesSkipClosing(t *testing.T) {
	s := &Service{}
	s.closing.Store(true)
	s.ApplyLaunchPreferences()
	s.ApplyLaunchPreferences()
}

func TestLaunchPreferencesOnlyOnce(t *testing.T) {
	for _, early := range []*Service{
		{},
		{ctx: context.Background()},
		{ctx: context.Background(), frontendReady: true},
		{ctx: context.Background(), trayReady: true},
	} {
		early.ApplyLaunchPreferences()
		available := false
		early.launchOnce.Do(func() { available = true })
		if !available {
			t.Fatal("an early callback consumed the launch action before runtime/frontend/tray readiness")
		}
	}
	s := &Service{settings: configstore.DefaultSettings(), ctx: context.Background(), frontendReady: true, trayReady: true}
	s.ApplyLaunchPreferences()
	// A second tray-ready callback must not apply subsequently changed preferences.
	// A nil monitor would panic if this attempted to start monitoring again.
	s.settings.AutoStartMonitoring = true
	s.ApplyLaunchPreferences()
}

func TestSaveLaunchPreferencesFailureKeepsSettings(t *testing.T) {
	s := &Service{
		settings: configstore.DefaultSettings(),
		store:    configstore.NewStoreWithPath(appName, t.TempDir()),
	}
	next := s.Settings()
	next.RemoveRouteOnMatch = false
	next.AutoStartMonitoring = true
	next.StartMinimized = true
	if _, err := s.SaveSettings(next); err == nil {
		t.Fatal("expected directory write failure")
	}
	if got := s.Settings(); got.AutoStartMonitoring || got.StartMinimized {
		t.Fatal("failed save changed launch preferences")
	}
}

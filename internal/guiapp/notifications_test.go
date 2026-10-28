package guiapp

import (
	"strings"
	"testing"
	"time"

	"netgap/internal/guiapp/configstore"
)

func TestLatestAlertDialogExplainsEmptyHistory(t *testing.T) {
	for _, language := range []string{"zh", "en"} {
		t.Run(language, func(t *testing.T) {
			settings := configstore.DefaultSettings()
			settings.Language = language
			s := &Service{settings: settings}
			options, eventID := s.lastAlertDialogOptions()
			if eventID != 0 || options.Title == "" || options.Message == "" {
				t.Fatalf("missing empty-history feedback: %+v, eventID=%d", options, eventID)
			}
			want := "运行日志"
			if language == "en" {
				want = "runtime logs"
			}
			if !strings.Contains(options.Message, want) {
				t.Fatalf("missing explanation of regular checks: %s", options.Message)
			}
		})
	}
}

func TestLatestAlertDialogCanBeReopenedWithPopupsDisabled(t *testing.T) {
	settings := configstore.DefaultSettings()
	settings.Language, settings.NotificationPopup = "en", false
	alert := Alert{EventID: 42, Timestamp: time.Date(2026, 10, 28, 1, 32, 56, 0, time.Local).UnixMilli(),
		IP: "185.47.253.235", Location: "Country: Peru | Province: Lima Province | City: Lima | ISP: Unknown",
		Path: "http://127.0.0.1:10808", Protection: "alerts_only", Active: false}
	s := &Service{settings: settings, lastAlert: &alert, alertUnread: true}
	for range 2 {
		options, eventID := s.lastAlertDialogOptions()
		if eventID != 42 {
			t.Fatalf("lost original alert: %d", eventID)
		}
		for _, text := range []string{"Previous alert", "2026-10-28 01:32:56", alert.IP, alert.Location, alert.Path, "no active match observation"} {
			if !strings.Contains(options.Message, text) {
				t.Errorf("missing %q from %s", text, options.Message)
			}
		}
	}
	if alert.Historical || !s.alertUnread {
		t.Fatal("preparing a dialog mutated the saved alert or marked it read before display")
	}
}

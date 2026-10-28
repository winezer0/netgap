package guiapp

import (
	"reflect"
	"strings"
	"testing"
)

func TestGUITranslationsAreComplete(t *testing.T) {
	var check func(*testing.T, reflect.Value)
	check = func(t *testing.T, value reflect.Value) {
		t.Helper()
		for i := range value.NumField() {
			field := value.Field(i)
			switch field.Kind() {
			case reflect.Struct:
				check(t, field)
			case reflect.String:
				if strings.TrimSpace(field.String()) == "" {
					t.Errorf("empty translation: %s", value.Type().Field(i).Name)
				}
			}
		}
	}
	for _, language := range []string{"zh", "en"} {
		t.Run(language, func(t *testing.T) {
			texts := textsFor(language)
			check(t, reflect.ValueOf(texts))
			for _, state := range []string{"alerts_only", "permission", "pac_observation", "pending", "running", "completed", "failed"} {
				if texts.Protection[state] == "" {
					t.Errorf("missing protection translation: %s", state)
				}
			}
		})
	}
	for _, language := range []string{"", "fr"} {
		if !reflect.DeepEqual(textsFor(language), textsFor("zh")) {
			t.Fatalf("unsupported language %q did not fall back to Chinese", language)
		}
	}
}

func TestAlertDialogFollowsLanguageChanges(t *testing.T) {
	s := &Service{}
	for _, tc := range []struct{ language, title, emptyText, history string }{
		{"en", "Latest alert", "No region-match alerts yet", "Previous alert"},
		{"zh", "最近提醒", "暂无地区命中提醒", "历史提醒"},
	} {
		settings := s.Settings()
		settings.Language = tc.language
		s.settings = settings
		s.lastAlert = nil
		options, _ := s.lastAlertDialogOptions()
		if options.Title != tc.title || !strings.Contains(options.Message, tc.emptyText) {
			t.Fatalf("incorrect empty dialog for %s: %+v", tc.language, options)
		}
		s.lastAlert = &Alert{EventID: 1, Protection: "unknown-future-state"}
		options, _ = s.lastAlertDialogOptions()
		if options.Title != tc.title || !strings.Contains(options.Message, tc.history) ||
			!strings.Contains(options.Message, textsFor(tc.language).UnknownProtection) {
			t.Fatalf("incorrect historical dialog for %s: %+v", tc.language, options)
		}
	}
}

package guiapp

import (
	"path/filepath"
	"testing"

	"netgap/internal/guiapp/configstore"
	"netgap/internal/guiapp/logbuffer"
	"netgap/internal/guiapp/monitoring"
)

type fakeTrayItem struct{ title, tooltip string }

func (item *fakeTrayItem) SetTitle(value string)   { item.title = value }
func (item *fakeTrayItem) SetTooltip(value string) { item.tooltip = value }

func TestSavedLanguageRefreshesAllTrayItems(t *testing.T) {
	settings := configstore.DefaultSettings()
	settings.RemoveRouteOnMatch = false
	s := &Service{
		settings: settings,
		store:    configstore.NewStoreWithPath(appName, filepath.Join(t.TempDir(), "settings.yaml")),
		logs:     logbuffer.New(10), monitor: monitoring.NewManager(nil, nil),
	}
	items := make([]*fakeTrayItem, 7)
	for i := range items {
		items[i] = &fakeTrayItem{}
	}
	tooltip := ""
	s.tray = &TrayController{service: s,
		items:      trayMenuItems{items[0], items[1], items[2], items[3], items[4], items[5], items[6]},
		setTooltip: func(value string) { tooltip = value },
	}
	for _, tc := range []struct {
		language string
		titles   []string
	}{
		{"en", []string{"Open main window", "View latest alert", "Remove default route", "Restore default route", "Start exit IP monitoring", "Stop exit IP monitoring", "Quit application"}},
		{"zh", []string{"打开主界面", "查看最近提醒", "移除默认路由", "恢复默认路由", "开始出口 IP 检测", "停止出口 IP 检测", "退出程序"}},
	} {
		next := s.Settings()
		next.Language = tc.language
		if _, err := s.SaveSettings(next); err != nil {
			t.Fatal(err)
		}
		for i, item := range items {
			if item.title != tc.titles[i] || item.tooltip != tc.titles[i] {
				t.Errorf("menu %d not translated to %s: %+v", i, tc.language, item)
			}
		}
		if tooltip != textsFor(tc.language).Tray.Tooltip {
			t.Errorf("tray tooltip not refreshed: %s", tooltip)
		}
	}
}

func TestTrayControllerStartShouldBeIdempotent(t *testing.T) {
	t.Parallel()

	calls := 0
	controller := &TrayController{
		start: func() {
			calls++
		},
	}

	controller.Start()
	controller.Start()
	controller.Start()

	if calls != 1 {
		t.Fatalf("expected start called once, got=%d", calls)
	}
}

func TestTrayControllerStopShouldBeIdempotent(t *testing.T) {
	t.Parallel()

	calls := 0
	controller := &TrayController{
		end: func() {
			calls++
		},
	}

	controller.Stop()
	controller.Stop()

	if calls != 1 {
		t.Fatalf("expected stop called once, got=%d", calls)
	}
}

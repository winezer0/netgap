package guiapp

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// TrayController connects the system tray menu to GUI service actions.
type TrayController struct {
	service    *Service
	icon       []byte
	tray       *application.SystemTray
	menu       *application.Menu
	items      trayMenuItems
	mu         sync.Mutex
	startOnce  sync.Once
	stopOnce   sync.Once
	start      func()
	end        func()
	setTooltip func(string)
}

type trayMenuItem interface {
	SetTitle(string)
	SetTooltip(string)
}

type trayMenuItems struct {
	openWindow, latestAlert, removeRoute, restoreRoute, startMonitoring, stopMonitoring, quit trayMenuItem
}

type wailsTrayMenuItem struct{ item *application.MenuItem }

func (i wailsTrayMenuItem) SetTitle(title string) { i.item.SetLabel(title) }
func (wailsTrayMenuItem) SetTooltip(string)       {}

func NewTrayController(service *Service, icon []byte) *TrayController {
	controller := &TrayController{service: service, icon: icon}
	controller.setTooltip = func(value string) {
		if controller.tray != nil {
			controller.tray.SetTooltip(value)
		}
	}
	service.mu.Lock()
	service.tray = controller
	service.mu.Unlock()
	return controller
}

func (t *TrayController) SetSystemTray(tray *application.SystemTray) {
	t.mu.Lock()
	t.tray = tray
	t.mu.Unlock()
}

func (t *TrayController) Start() {
	t.startOnce.Do(func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.tray == nil {
			if t.start != nil {
				t.start()
			}
			return
		}
		t.rebuildMenuLocked()
		t.tray.SetIcon(t.icon)
		t.tray.SetTooltip(textsFor(t.service.Settings().Language).Tray.Tooltip)
		t.tray.SetMenu(t.menuLocked())
		t.tray.Run()
		t.service.MarkTrayReady()
	})
}

func (t *TrayController) Stop() {
	t.stopOnce.Do(func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.end != nil {
			t.end()
		}
		if t.tray != nil {
			t.tray.Hide()
		}
		t.tray = nil
		t.items = trayMenuItems{}
	})
}

func (t *TrayController) RefreshLanguage() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.items.openWindow == nil {
		return
	}
	texts := textsFor(t.service.Settings().Language).Tray
	for _, entry := range []struct {
		item  trayMenuItem
		title string
	}{
		{t.items.openWindow, texts.OpenWindow}, {t.items.latestAlert, texts.LatestAlert},
		{t.items.removeRoute, texts.RemoveRoute}, {t.items.restoreRoute, texts.RestoreRoute},
		{t.items.startMonitoring, texts.StartMonitoring}, {t.items.stopMonitoring, texts.StopMonitoring}, {t.items.quit, texts.Quit},
	} {
		if entry.item != nil {
			entry.item.SetTitle(entry.title)
			entry.item.SetTooltip(entry.title)
		}
	}
	if t.setTooltip != nil {
		t.setTooltip(texts.Tooltip)
	}
}

func (t *TrayController) rebuildMenuLocked() {
	texts := textsFor(t.service.Settings().Language).Tray
	menu := application.NewMenu()
	t.menu = menu
	t.items.openWindow = wailsTrayMenuItem{menu.Add(texts.OpenWindow).OnClick(func(*application.Context) { t.service.ShowWindow() })}
	t.items.latestAlert = wailsTrayMenuItem{menu.Add(texts.LatestAlert).OnClick(func(*application.Context) { go t.service.ShowLastAlert() })}
	menu.AddSeparator()
	t.items.removeRoute = wailsTrayMenuItem{menu.Add(texts.RemoveRoute).OnClick(func(*application.Context) {
		_, _ = t.service.RemoveDefaultRoute()
		t.service.EmitState()
	})}
	t.items.restoreRoute = wailsTrayMenuItem{menu.Add(texts.RestoreRoute).OnClick(func(*application.Context) {
		_, _ = t.service.RestoreDefaultRoute()
		t.service.EmitState()
	})}
	menu.AddSeparator()
	t.items.startMonitoring = wailsTrayMenuItem{menu.Add(texts.StartMonitoring).OnClick(func(*application.Context) { _, _ = t.service.StartMonitoring() })}
	t.items.stopMonitoring = wailsTrayMenuItem{menu.Add(texts.StopMonitoring).OnClick(func(*application.Context) { _, _ = t.service.StopMonitoring() })}
	menu.AddSeparator()
	t.items.quit = wailsTrayMenuItem{menu.Add(texts.Quit).OnClick(func(*application.Context) { t.service.QuitApp() })}
}

func (t *TrayController) menuLocked() *application.Menu {
	// Menus are rebuilt on language changes and cached by the tray controller.
	return t.menu
}

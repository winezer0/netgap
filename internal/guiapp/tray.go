package guiapp

import (
	"sync"

	"github.com/cardinalby/go-systray"
)

type trayMenuItem interface {
	SetTitle(string)
	SetTooltip(string)
}

type trayMenuItems struct {
	openWindow, latestAlert, removeRoute, restoreRoute, startMonitoring, stopMonitoring, quit trayMenuItem
}

// TrayController 表示系统托盘控制器。
type TrayController struct {
	service    *Service
	icon       []byte
	start      func()
	end        func()
	startOnce  sync.Once
	stopOnce   sync.Once
	menuMu     sync.Mutex
	items      trayMenuItems
	setTooltip func(string)
}

// NewTrayController 创建系统托盘控制器。
func NewTrayController(service *Service, icon []byte) *TrayController {
	controller := &TrayController{
		service:    service,
		icon:       icon,
		setTooltip: systray.SetTooltip,
	}
	controller.start, controller.end = systray.RunWithExternalLoop(
		controller.onReady,
		controller.onExit,
	)
	service.mu.Lock()
	service.tray = controller
	service.mu.Unlock()
	return controller
}

// RefreshLanguage reads current settings while serializing native menu updates,
// so delayed save completions cannot restore an older language.
func (t *TrayController) RefreshLanguage() {
	t.menuMu.Lock()
	defer t.menuMu.Unlock()
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
		{t.items.startMonitoring, texts.StartMonitoring}, {t.items.stopMonitoring, texts.StopMonitoring},
		{t.items.quit, texts.Quit},
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

// Start 启动托盘事件循环。
func (t *TrayController) Start() {
	t.startOnce.Do(func() {
		if t.start != nil {
			t.start()
		}
	})
}

// Stop 停止托盘事件循环。
func (t *TrayController) Stop() {
	t.stopOnce.Do(func() {
		if t.end != nil {
			t.end()
		}
	})
}

// onReady 初始化托盘菜单。
func (t *TrayController) onReady() {
	if len(t.icon) > 0 {
		systray.SetIcon(t.icon)
	}
	systray.SetTitle(appName)
	texts := textsFor(t.service.Settings().Language).Tray
	systray.SetTooltip(texts.Tooltip)

	openItem := systray.AddMenuItem(texts.OpenWindow, texts.OpenWindow)
	lastAlertItem := systray.AddMenuItem(texts.LatestAlert, texts.LatestAlert)
	systray.AddSeparator()
	removeItem := systray.AddMenuItem(texts.RemoveRoute, texts.RemoveRoute)
	restoreItem := systray.AddMenuItem(texts.RestoreRoute, texts.RestoreRoute)
	systray.AddSeparator()
	startItem := systray.AddMenuItem(texts.StartMonitoring, texts.StartMonitoring)
	stopItem := systray.AddMenuItem(texts.StopMonitoring, texts.StopMonitoring)
	systray.AddSeparator()
	quitItem := systray.AddMenuItem(texts.Quit, texts.Quit)
	t.menuMu.Lock()
	t.items = trayMenuItems{openItem, lastAlertItem, removeItem, restoreItem, startItem, stopItem, quitItem}
	t.menuMu.Unlock()
	t.RefreshLanguage()

	go func() {
		for {
			select {
			case <-openItem.ClickedCh:
				t.service.ShowWindow()
			case <-lastAlertItem.ClickedCh:
				go t.service.ShowLastAlert()
			case <-removeItem.ClickedCh:
				_, _ = t.service.RemoveDefaultRoute()
				t.service.emitState()
			case <-restoreItem.ClickedCh:
				_, _ = t.service.RestoreDefaultRoute()
				t.service.emitState()
			case <-startItem.ClickedCh:
				_, _ = t.service.StartMonitoring()
			case <-stopItem.ClickedCh:
				_, _ = t.service.StopMonitoring()
			case <-quitItem.ClickedCh:
				t.service.QuitApp()
				return
			}
		}
	}()
}

// onExit 处理托盘退出回调。
func (t *TrayController) onExit() {
	t.menuMu.Lock()
	t.items = trayMenuItems{}
	t.menuMu.Unlock()
	t.service.writeLog("INFO", "Tray controller exited")
}

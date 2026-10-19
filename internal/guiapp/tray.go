package guiapp

import (
	"sync"

	"github.com/cardinalby/go-systray"
)

// TrayController 表示系统托盘控制器。
type TrayController struct {
	service *Service
	icon    []byte
	start   func()
	end     func()
	startOnce sync.Once
	stopOnce  sync.Once
}

// NewTrayController 创建系统托盘控制器。
func NewTrayController(service *Service, icon []byte) *TrayController {
	controller := &TrayController{
		service: service,
		icon:    icon,
	}
	controller.start, controller.end = systray.RunWithExternalLoop(
		controller.onReady,
		controller.onExit,
	)
	return controller
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
	systray.SetTooltip(appName)

	openItem := systray.AddMenuItem("打开主界面", "Open main window")
	systray.AddSeparator()
	removeItem := systray.AddMenuItem("移除默认路由", "Remove default route")
	restoreItem := systray.AddMenuItem("恢复默认路由", "Restore default route")
	systray.AddSeparator()
	startItem := systray.AddMenuItem("开始出口IP检测", "Start monitoring")
	stopItem := systray.AddMenuItem("停止出口IP检测", "Stop monitoring")
	systray.AddSeparator()
	quitItem := systray.AddMenuItem("退出程序", "Quit application")

	go func() {
		for {
			select {
			case <-openItem.ClickedCh:
				t.service.ShowWindow()
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
	t.service.writeLog("INFO", "Tray controller exited")
}

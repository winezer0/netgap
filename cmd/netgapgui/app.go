package main

import (
	"context"

	"netgap/internal/guiapp"
)

// App 表示绑定到 Wails 前端的桥接对象。
type App struct {
	service *guiapp.Service
	tray    *guiapp.TrayController
}

// NewApp 创建 Wails 桥接对象。
func NewApp(service *guiapp.Service, icon []byte) *App {
	return &App{
		service: service,
		tray:    guiapp.NewTrayController(service, icon),
	}
}

// Startup 处理应用启动生命周期。
func (a *App) Startup(ctx context.Context) {
	a.service.Startup(ctx)
}

// DomReady 在前端可用后推送初始状态。
func (a *App) DomReady(ctx context.Context) {
	a.service.EmitState()
	a.service.EmitLogs()
	a.tray.Start()
}

// Shutdown 处理应用关闭生命周期。
func (a *App) Shutdown(ctx context.Context) {
	a.tray.Stop()
	a.service.Shutdown()
}

// BeforeClose 允许窗口正常关闭。
func (a *App) BeforeClose(ctx context.Context) bool {
	return false
}

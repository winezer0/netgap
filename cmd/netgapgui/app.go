package main

import (
	"context"
	"github.com/wailsapp/wails/v3/pkg/application"
	"netgap/internal/guiapp"
	"sync"
)

type App struct {
	service           *guiapp.Service
	tray              *guiapp.TrayController
	runtime           *application.App
	window            *application.WebviewWindow
	frontendReadyOnce sync.Once
}

func NewApp(service *guiapp.Service, icon []byte) *App {
	return &App{service: service, tray: guiapp.NewTrayController(service, icon)}
}

func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.service.Startup(ctx)
	return nil
}

func (a *App) FrontendReady() {
	a.frontendReadyOnce.Do(func() {
		a.service.EmitState()
		a.service.EmitLogs()
		a.tray.SetSystemTray(a.runtime.SystemTray.New())
		a.tray.Start()
		a.service.MarkFrontendReady()
	})
}

func (a *App) Shutdown() { a.tray.Stop(); a.service.Shutdown() }

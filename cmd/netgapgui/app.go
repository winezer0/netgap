package main

import (
	"context"
	"github.com/wailsapp/wails/v3/pkg/application"
	"netgap/internal/guiapp"
	"runtime"
)

type App struct {
	service *guiapp.Service
	tray    *guiapp.TrayController
	runtime *application.App
	window  *application.WebviewWindow
}

func NewApp(service *guiapp.Service, icon []byte) *App {
	return &App{service: service, tray: guiapp.NewTrayController(service, icon)}
}

func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.service.Startup(ctx)
	return nil
}

func (a *App) FrontendReady() {
	a.service.EmitState()
	a.service.EmitLogs()
	if runtime.GOOS != "darwin" {
		a.tray.Start()
	}
	a.service.MarkFrontendReady()
}

func (a *App) Shutdown() { a.tray.Stop(); a.service.Shutdown() }

package main

import (
	"fmt"
	"os"
	"runtime"
	"slices"

	"github.com/wailsapp/wails/v3/pkg/application"
	"netgap/internal/guiapp"
	"netgap/internal/guiapp/elevation"
)

func main() {
	intent, err := elevation.Prepare(os.Args[1:])
	if err != nil {
		elevation.ShowLaunchError(err)
		return
	}
	service, err := newService()
	if err != nil {
		fmt.Printf("failed to create gui service: %v\n", err)
		if intent != nil {
			elevation.ShowLaunchError(err)
		}
		return
	}
	if intent != nil {
		service.ConfigureElevatedRestart(intent.ResumeMonitoring)
	}
	appIcon := appIconPNG
	if runtime.GOOS == "windows" {
		appIcon = windowsTrayIcon
	}
	app := NewApp(service, appIcon)
	var singleInstance *application.SingleInstanceOptions
	if runtime.GOOS == "windows" {
		singleInstance = &application.SingleInstanceOptions{
			UniqueID: "netgapgui-current-user-instance",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				if !slices.Contains(data.Args, "--autostart") {
					service.ShowWindow()
				}
			},
		}
	}
	app.runtime = application.New(application.Options{
		Name:           "netgapgui",
		Icon:           appIcon,
		SingleInstance: singleInstance,
		Services:       []application.Service{application.NewService(app)},
		Assets:         application.AssetOptions{Handler: application.AssetFileServerFS(frontendDist)},
		OnShutdown:     app.Shutdown,
	})
	app.window = app.runtime.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "main", Title: "netgapgui", Width: 600, Height: 800,
		MinWidth: 600, MinHeight: 800, MaxWidth: 800, MaxHeight: 1200,
		Frameless: true, Hidden: service.ShouldStartHidden(),
		BackgroundColour: application.RGBA{Red: 18, Green: 22, Blue: 28, Alpha: 255},
		Linux:            application.LinuxWindow{Icon: appIconPNG},
	})
	service.SetRuntime(app.runtime, app.window)
	if err := app.runtime.Run(); err != nil {
		fmt.Printf("error: %v\n", err)
	}
}

func newService() (*guiapp.Service, error) { return guiapp.NewService() }

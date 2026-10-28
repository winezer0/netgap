package main

import (
	"fmt"
	"os"
	"runtime"
	"slices"

	"netgap/internal/guiapp"
	"netgap/internal/guiapp/elevation"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// main 启动基于 Wails v2 的图形界面程序。
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
	app := NewApp(service, trayIcon)
	var instanceLock *options.SingleInstanceLock
	if runtime.GOOS == "windows" {
		instanceLock = &options.SingleInstanceLock{
			UniqueId: "netgapgui-current-user-instance",
			OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
				if !slices.Contains(data.Args, "--autostart") {
					service.ShowWindow()
				}
			},
		}
	}

	err = wails.Run(&options.App{
		Title:              "netgapgui",
		SingleInstanceLock: instanceLock,
		Width:              600,
		Height:             800,
		MinWidth:           600,
		MinHeight:          800,
		MaxWidth:           800,
		MaxHeight:          1200,
		DisableResize:      false,
		Frameless:          true,
		StartHidden:        service.ShouldStartHidden(),
		HideWindowOnClose:  false,
		AssetServer: &assetserver.Options{
			Assets: frontendDist,
		},
		BackgroundColour: options.NewRGBA(18, 22, 28, 255),
		OnStartup:        app.Startup,
		OnDomReady:       app.DomReady,
		OnShutdown:       app.Shutdown,
		OnBeforeClose:    app.BeforeClose,
		Bind:             []interface{}{app},
		Windows: &windows.Options{
			DisableFramelessWindowDecorations: false,
			DisableWindowIcon:                 true,
		},
	})
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
}

// newService 创建 GUI 服务对象。
func newService() (*guiapp.Service, error) {
	return guiapp.NewService()
}

package main

import (
	"fmt"

	"netgap/internal/guiapp"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// main 启动基于 Wails v2 的图形界面程序。
func main() {
	service, err := newService()
	if err != nil {
		fmt.Printf("failed to create gui service: %v\n", err)
		return
	}
	app := NewApp(service, trayIcon)

	err = wails.Run(&options.App{
		Title:             "netgapgui",
		Width:             600,
		Height:            700,
		MinWidth:          600,
		MinHeight:         700,
		MaxWidth:          1200,
		MaxHeight:         1200,
		DisableResize:     false,
		Frameless:         true,
		StartHidden:       false,
		HideWindowOnClose: false,
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

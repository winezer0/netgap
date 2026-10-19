package main

import "embed"

// frontendDist 存放前端打包产物。
//
//go:embed all:frontend/dist
var frontendDist embed.FS

// trayIcon stores the application icon used by the system tray.
//
//go:embed build/icon.ico
var trayIcon []byte

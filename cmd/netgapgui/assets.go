package main

import "embed"

// frontendDist 存放前端打包产物。
//
//go:embed all:frontend/dist
var frontendDist embed.FS

// appIconPNG stores the cross-platform application icon and Linux/macOS tray icon.
//
//go:embed build/appicon.png
var appIconPNG []byte

// windowsTrayIcon stores the Windows system tray icon.
//
//go:embed build/icon.ico
var windowsTrayIcon []byte

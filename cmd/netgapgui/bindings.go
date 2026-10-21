package main

import (
	"netgap/internal/guiapp"
	"netgap/internal/guiapp/configstore"
	"netgap/internal/iproute"
)

// SetWindowForeground reports WebView focus/visibility for notification routing.
func (a *App) SetWindowForeground(foreground bool, revision uint64) {
	a.service.SetWindowForeground(foreground, revision)
}

// GetInitialState 返回当前界面初始状态。
func (a *App) GetInitialState() *guiapp.ViewState {
	return a.service.State()
}

// SaveSettings 保存当前界面配置。
func (a *App) SaveSettings(input configstore.Settings) (*guiapp.ViewState, error) {
	return a.service.SaveSettings(&input)
}

// AddRegion 添加区域关键字。
func (a *App) AddRegion(keyword string) (*guiapp.ViewState, error) {
	return a.service.AddRegion(keyword)
}

// RemoveRegion 移除指定区域。
func (a *App) RemoveRegion(region string) (*guiapp.ViewState, error) {
	return a.service.RemoveRegion(region)
}

// StartMonitoring 启动出口 IP 监测。
func (a *App) StartMonitoring() (*guiapp.ViewState, error) {
	return a.service.StartMonitoring()
}

// StopMonitoring 停止出口 IP 监测。
func (a *App) StopMonitoring() (*guiapp.ViewState, error) {
	return a.service.StopMonitoring()
}

// RemoveDefaultRoute 移除默认路由。
func (a *App) RemoveDefaultRoute() (*guiapp.RouteActionResult, error) {
	return a.service.RemoveDefaultRoute()
}

// RestoreDefaultRoute 恢复默认路由。
func (a *App) RestoreDefaultRoute() (*guiapp.RouteActionResult, error) {
	return a.service.RestoreDefaultRoute()
}

func (a *App) GetDefaultRouteInfo() (*iproute.RouteInfo, error) {
	return a.service.GetDefaultRouteInfo()
}

func (a *App) ClearLogs() {
	a.service.ClearLogs()
}

func (a *App) LogUIError(message string) {
	a.service.LogUIError(message)
}

func (a *App) GetProxyConfigInfo() (*guiapp.ProxyConfigInfo, error) {
	return a.service.GetProxyConfigInfo()
}

// ShowWindow 显示主窗口。
func (a *App) ShowWindow() {
	a.service.ShowWindow()
}

// MinimiseWindow 最小化到任务栏。
func (a *App) MinimiseWindow() {
	a.service.MinimiseWindow()
}

// HideToTray 最小化到托盘。
func (a *App) HideToTray() {
	a.service.HideToTray()
}

// QuitApp 退出 GUI 程序。
func (a *App) QuitApp() {
	a.service.QuitApp()
}

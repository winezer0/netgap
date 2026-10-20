package guiapp

import "netgap/internal/monitor"

// ViewState 表示前端初始化与刷新时使用的界面状态。
type ViewState struct {
	Language           string         `json:"language"`
	Interval           int            `json:"interval"`
	KeywordInput       string         `json:"keywordInput"`
	Regions            []string       `json:"regions"`
	APIEnabled         []string       `json:"apiEnabled"`
	CustomAPIURL       string         `json:"customApiUrl"`
	CloseAction        string         `json:"closeAction"`
	RemoveRouteOnMatch bool           `json:"removeRouteOnMatch"`
	Logs               []string       `json:"logs"`
	ConfigPath         string         `json:"configPath"`
	IsAdmin            bool           `json:"isAdmin"`
	Running            bool           `json:"running"`
	StatusText         string         `json:"statusText"`
	PermissionText     string         `json:"permissionText"`
	Health             monitor.Health `json:"health"`
}

// RouteActionResult 表示路由相关动作结果。
type RouteActionResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

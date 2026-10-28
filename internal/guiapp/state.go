package guiapp

import "netgap/internal/monitor"

// ViewState 表示前端初始化与刷新时使用的界面状态。
type ViewState struct {
	AutoStartMonitoring bool           `json:"autoStartMonitoring"`
	StartMinimized      bool           `json:"startMinimized"`
	Language            string         `json:"language"`
	Interval            int            `json:"interval"`
	KeywordInput        string         `json:"keywordInput"`
	Regions             []string       `json:"regions"`
	APIEnabled          []string       `json:"apiEnabled"`
	CustomAPIURL        string         `json:"customApiUrl"`
	CloseAction         string         `json:"closeAction"`
	RemoveRouteOnMatch  bool           `json:"removeRouteOnMatch"`
	NotificationPopup   bool           `json:"notificationPopup"`
	Logs                []string       `json:"logs"`
	ErrorCount          uint64         `json:"errorCount"`
	ConfigPath          string         `json:"configPath"`
	IsAdmin             bool           `json:"isAdmin"`
	Running             bool           `json:"running"`
	StatusText          string         `json:"statusText"`
	PermissionText      string         `json:"permissionText"`
	Health              monitor.Health `json:"health"`
	LatestAlert         *Alert         `json:"latestAlert"`
	AlertUnread         bool           `json:"alertUnread"`
	AlertRevision       uint64         `json:"alertRevision"`
}

// RouteActionResult 表示路由相关动作结果。
type RouteActionResult struct {
	Success bool   `json:"success"`
	Noop    bool   `json:"noop"`
	Message string `json:"message"`
}

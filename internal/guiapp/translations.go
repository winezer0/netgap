package guiapp

type trayTexts struct {
	Tooltip, OpenWindow, LatestAlert, RemoveRoute, RestoreRoute, StartMonitoring, StopMonitoring, Quit string
}

type guiTexts struct {
	Tray                            trayTexts
	LatestAlertTitle, NoRecentAlert string
	MatchedRegion, HistoricalAlert  string
	StillMatched, NoActiveMatch     string
	UnknownProtection               string
	Protection                      map[string]string
}

var guiTranslations = map[string]guiTexts{
	"zh": {
		Tray: trayTexts{
			Tooltip: "NetGap — 出口 IP 监控", OpenWindow: "打开主界面", LatestAlert: "查看最近提醒",
			RemoveRoute: "移除默认路由", RestoreRoute: "恢复默认路由",
			StartMonitoring: "开始出口 IP 检测", StopMonitoring: "停止出口 IP 检测", Quit: "退出程序",
		},
		LatestAlertTitle: "最近提醒", NoRecentAlert: "暂无地区命中提醒。普通检测结果请查看运行日志。",
		MatchedRegion: "命中监测地区", HistoricalAlert: "历史提醒",
		StillMatched: "；最近检测仍命中", NoActiveMatch: "；当前无有效命中记录", UnknownProtection: "保护状态未知",
		Protection: map[string]string{
			"alerts_only": "仅告警", "permission": "权限不足，仅告警",
			"pac_observation": "未应用 PAC/WPAD，系统路由出口，仅告警", "pending": "自动断网待处理",
			"running": "正在自动断网", "completed": "断网完成，恢复记录已保存", "failed": "自动断网失败",
		},
	},
	"en": {
		Tray: trayTexts{
			Tooltip: "NetGap — Exit IP monitoring", OpenWindow: "Open main window", LatestAlert: "View latest alert",
			RemoveRoute: "Remove default route", RestoreRoute: "Restore default route",
			StartMonitoring: "Start exit IP monitoring", StopMonitoring: "Stop exit IP monitoring", Quit: "Quit application",
		},
		LatestAlertTitle: "Latest alert", NoRecentAlert: "No region-match alerts yet. See the runtime logs for regular check results.",
		MatchedRegion: "Monitored region matched", HistoricalAlert: "Previous alert",
		StillMatched: "; still matched at last check", NoActiveMatch: "; no active match observation", UnknownProtection: "unknown protection status",
		Protection: map[string]string{
			"alerts_only": "alerts only", "permission": "insufficient permission; alerts only",
			"pac_observation": "PAC/WPAD not applied; system-route exit; alerts only", "pending": "automatic disconnection pending",
			"running": "automatic disconnection in progress", "completed": "disconnected; recovery records saved", "failed": "automatic disconnection failed",
		},
	},
}

func textsFor(language string) guiTexts {
	if texts, ok := guiTranslations[language]; ok {
		return texts
	}
	return guiTranslations["zh"]
}

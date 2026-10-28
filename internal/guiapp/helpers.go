package guiapp

import (
	"strings"

	"netgap/internal/guiapp/configstore"
)

// cloneSettings 复制一份配置对象，避免并发共享底层切片。
func cloneSettings(settings *configstore.Settings) *configstore.Settings {
	if settings == nil {
		return configstore.DefaultSettings()
	}
	regions := append([]string(nil), settings.Regions...)
	apiEnabled := append([]string(nil), settings.APIEnabled...)
	return &configstore.Settings{
		AutoStartMonitoring: settings.AutoStartMonitoring,
		StartMinimized:      settings.StartMinimized,
		Language:            settings.Language,
		Interval:            settings.Interval,
		KeywordInput:        settings.KeywordInput,
		Regions:             regions,
		APIEnabled:          apiEnabled,
		CustomAPIURL:        settings.CustomAPIURL,
		CloseAction:         settings.CloseAction,
		RemoveRouteOnMatch:  settings.RemoveRouteOnMatch,
		NotificationPopup:   settings.NotificationPopup,
	}
}

// filterRegion 返回移除指定区域后的列表。
func filterRegion(regions []string, region string) []string {
	target := strings.TrimSpace(region)
	result := make([]string, 0, len(regions))
	for _, item := range regions {
		if strings.TrimSpace(item) == target {
			continue
		}
		result = append(result, item)
	}
	return result
}

// runningText 返回运行状态文本。
func runningText(running bool) string {
	if running {
		return "running"
	}
	return "stopped"
}

// permissionText 返回权限状态文本。
func permissionText(admin bool) string {
	if admin {
		return "admin"
	}
	return "user"
}

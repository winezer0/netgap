package guiapp

import (
	"fmt"
	"strings"

	"netgap/internal/config"
	"netgap/internal/iproute"
)

// StartMonitoring 启动出口 IP 监测任务。
func (s *Service) StartMonitoring() (*ViewState, error) {
	if info, err := s.GetProxyConfigInfo(); err != nil {
		s.writeLog("ERROR", "Proxy config check failed: "+err.Error())
	} else {
		s.writeLog("INFO", formatProxyConfigLog(info))
	}

	settings := s.Settings()
	cfg := &config.Config{
		Regions:      append([]string(nil), settings.Regions...),
		Interval:     settings.Interval,
		APIUse:       append([]string(nil), settings.APIEnabled...),
		Shutdown:     false,
		CustomAPIURL: settings.CustomAPIURL,
	}
	if err := s.monitor.Start(cfg); err != nil {
		return nil, err
	}
	s.writeLog("INFO", fmt.Sprintf("Monitoring started with %d regions, %d APIs", len(settings.Regions), len(settings.APIEnabled)))
	s.emitState()
	return s.State(), nil
}

// StopMonitoring 停止出口 IP 监测任务。
func (s *Service) StopMonitoring() (*ViewState, error) {
	if !s.monitor.Stop() {
		s.writeLog("WARN", "Monitoring is not running")
		s.emitState()
		return s.State(), nil
	}
	s.writeLog("INFO", "Monitoring stopped by user")
	s.emitState()
	return s.State(), nil
}

// RemoveDefaultRoute 保存并移除默认路由。
func (s *Service) RemoveDefaultRoute() (*RouteActionResult, error) {
	if !isAdmin() {
		return nil, fmt.Errorf("admin permission required")
	}
	route, err := iproute.GetDefaultRoute()
	if err != nil {
		return nil, err
	}
	if err := iproute.SaveRouteInfo(route); err != nil {
		return nil, err
	}
	if err := iproute.DeleteDefaultRoute(); err != nil {
		return nil, err
	}
	message := fmt.Sprintf("Default route removed, gateway=%s", route.Gateway)
	s.writeLog("WARN", message)
	return &RouteActionResult{Success: true, Message: message}, nil
}

// RestoreDefaultRoute 恢复之前保存的默认路由。
func (s *Service) RestoreDefaultRoute() (*RouteActionResult, error) {
	if !isAdmin() {
		return nil, fmt.Errorf("admin permission required")
	}
	route, err := iproute.LoadRouteInfo()
	if err != nil {
		return nil, err
	}
	if err := iproute.AddDefaultRoute(route.Gateway); err != nil {
		return nil, err
	}
	message := fmt.Sprintf("Default route restored, gateway=%s", route.Gateway)
	s.writeLog("INFO", message)
	return &RouteActionResult{Success: true, Message: message}, nil
}

func (s *Service) GetDefaultRouteInfo() (*iproute.RouteInfo, error) {
	return iproute.GetDefaultRoute()
}

func (s *Service) ClearLogs() {
	s.logs.Clear()
	s.emitLogs()
}

func formatProxyConfigLog(info *ProxyConfigInfo) string {
	if info == nil {
		return "Proxy config: unknown"
	}
	source := strings.TrimSpace(info.Source)
	if source == "" {
		source = "unknown"
	}
	if !info.Enabled {
		return fmt.Sprintf("Proxy config: enabled=false source=%s", source)
	}
	addr := strings.TrimSpace(info.Address)
	port := strings.TrimSpace(info.Port)
	if addr == "" {
		return fmt.Sprintf("Proxy config: enabled=true source=%s", source)
	}
	if port == "" {
		return fmt.Sprintf("Proxy config: enabled=true source=%s address=%s", source, addr)
	}
	return fmt.Sprintf("Proxy config: enabled=true source=%s address=%s port=%s", source, addr, port)
}

// IsAdmin 返回当前是否拥有管理员权限。
func (s *Service) IsAdmin() bool {
	return isAdmin()
}

// isAdmin 检查当前权限状态。
func isAdmin() bool {
	return iproute.CheckAdminPermission()
}

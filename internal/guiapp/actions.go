package guiapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"netgap/internal/config"
	"netgap/internal/detector"
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
		Regions:            append([]string(nil), settings.Regions...),
		Interval:           settings.Interval,
		APIUse:             append([]string(nil), settings.APIEnabled...),
		Shutdown:           false,
		CustomAPIURL:       settings.CustomAPIURL,
		RemoveRouteOnMatch: settings.RemoveRouteOnMatch,
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
	ctx, cancel := context.WithTimeout(s.operationContext(), time.Minute)
	defer cancel()
	if err := iproute.ProtectionPreflight(ctx, false); err != nil {
		return nil, err
	}
	if !iproute.CheckAdminPermissionContext(ctx) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("admin permission required")
	}
	record := s.monitor.ObserveManualRouteAction("completed")
	err := iproute.DeleteAllDefaultRoutes(ctx)
	record(err)
	if err != nil {
		return nil, err
	}
	message := "All IPv4/IPv6 default routes removed; recovery snapshot: " + iproute.RecoveryPath()
	s.writeLog("WARN", message)
	return &RouteActionResult{Success: true, Message: message}, nil
}

// RestoreDefaultRoute 恢复之前保存的默认路由。
func (s *Service) RestoreDefaultRoute() (*RouteActionResult, error) {
	ctx, cancel := context.WithTimeout(s.operationContext(), time.Minute)
	defer cancel()
	if err := iproute.ProtectionPreflight(ctx, true); err != nil {
		return nil, err
	}
	if !iproute.CheckAdminPermissionContext(ctx) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("admin permission required")
	}
	record := s.monitor.ObserveManualRouteAction("restored")
	err := iproute.RestoreAllDefaultRoutes(ctx)
	record(err)
	if err != nil {
		return nil, err
	}
	message := "All saved default routes restored"
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
	if info != nil && info.ObservationOnly != "" {
		return detector.ObservationWarning
	}
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
		if info.Type != "" {
			return fmt.Sprintf("Proxy config: enabled=true source=%s mode=%s", source, info.Type)
		}
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

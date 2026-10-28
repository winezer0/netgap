package guiapp

import (
	"path/filepath"

	logging "github.com/winezer0/zaplogs"
	"netgap/internal/monitor"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Health updates bypass State(), which also performs permission checks.
func (s *Service) emitHealth(health monitor.Health) {
	s.observeAlertHealth(health)
	if s.ctx != nil {
		runtime.EventsEmit(s.ctx, eventHealthName, health)
	}
}

// emitState 向前端推送当前状态。
func (s *Service) emitState() {
	if s.ctx == nil {
		return
	}
	runtime.EventsEmit(s.ctx, eventStateName, s.State())
}

// EmitState 主动向前端同步当前状态。
func (s *Service) EmitState() {
	s.emitState()
}

// emitLogs 向前端推送当前日志列表。
func (s *Service) emitLogs() {
	if s.ctx == nil {
		return
	}
	lines, errorCount := s.logs.Snapshot()
	runtime.EventsEmit(s.ctx, eventLogName, lines, errorCount)
}

// EmitLogs 主动向前端同步当前日志。
func (s *Service) EmitLogs() {
	s.emitLogs()
}

// writeLog 记录一条 GUI 日志并广播到前端。
func (s *Service) writeLog(level, message string) {
	switch level {
	case "ERROR":
		logging.Errorf("%s", message)
	case "WARN":
		logging.Warnf("%s", message)
	case "DEBUG":
		logging.Debugf("%s", message)
	default:
		logging.Infof("%s", message)
	}
	s.logs.Add(level, message)
	s.emitLogs()
}

// LogUIError persists the full message displayed by an error notification.
func (s *Service) LogUIError(message string) {
	s.writeLog("ERROR", "UI: "+message)
}

// initLogger 初始化后端日志输出。
func (s *Service) initLogger() {
	logPath := filepath.Join(filepath.Dir(s.store.Path()), appName+".log")
	logCfg := logging.NewConfig("info", logPath, "off")
	logCfg.FileLevel = "info"
	logCfg.MaxBackups = 10
	if err := logging.InitDefaultLogger(logCfg); err != nil {
		s.logs.Add("ERROR", "Failed to initialize logger: "+err.Error())
		return
	}
}

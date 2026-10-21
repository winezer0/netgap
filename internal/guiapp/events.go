package guiapp

import (
	"path/filepath"
	"strings"
	"time"

	logging "github.com/winezer0/zaplogs"
	"netgap/internal/monitor"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Health updates bypass State(), which also performs permission checks.
func (s *Service) emitHealth(health monitor.Health) {
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
	runtime.EventsEmit(s.ctx, eventLogName, s.logs.List())
}

// EmitLogs 主动向前端同步当前日志。
func (s *Service) EmitLogs() {
	s.emitLogs()
}

// writeLog 记录一条 GUI 日志并广播到前端。
func (s *Service) writeLog(level, message string) {
	s.logs.Add(level, message)
	s.emitLogs()

	if level == "WARN" && strings.Contains(message, "Local exit detected") {
		s.emitLocalExitAlert(message)
	}
}

func (s *Service) emitLocalExitAlert(message string) {
	s.alertMu.Lock()
	defer s.alertMu.Unlock()
	if time.Since(s.lastLocalExit) < 3*time.Second {
		return
	}
	s.lastLocalExit = time.Now()
	s.lastAlert = message
	if s.ctx == nil {
		return
	}
	settings := s.Settings()
	if settings.NotificationPopup && s.windowHidden.Load() {
		go func() {
			if err := notifyDesktop(notificationTitle, message); err != nil {
				s.logs.Add("WARN", "Failed to show desktop notification: "+err.Error())
				s.emitLogs()
			}
		}()
	} else if settings.NotificationPopup {
		runtime.EventsEmit(s.ctx, eventAlertName, &Alert{
			Type:      "error",
			Message:   message,
			Timestamp: time.Now().Unix(),
		})
	}
}

// ShowLastAlert 显示主窗口并重放最近一次匹配提醒。
func (s *Service) ShowLastAlert() {
	s.alertMu.Lock()
	message := s.lastAlert
	s.alertMu.Unlock()
	s.ShowWindow()
	if message == "" || s.ctx == nil {
		return
	}
	runtime.EventsEmit(s.ctx, eventAlertName, &Alert{
		Type:      "error",
		Message:   message,
		Timestamp: time.Now().Unix(),
	})
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

package guiapp

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	slogs "github.com/winezer0/slogs"

	"netgap/internal/guiapp/configstore"
	"netgap/internal/guiapp/logbuffer"
	"netgap/internal/guiapp/monitoring"
	"netgap/internal/ipchecker/localdb"
	"netgap/internal/iproute"
	monitorcore "netgap/internal/monitor"
)

const (
	appName             = "netgapgui"
	eventStateName      = "netgapgui:state"
	eventLogName        = "netgapgui:logs"
	eventAlertName      = "netgapgui:alert"
	eventHealthName     = "netgapgui:health"
	eventAlertStateName = "netgapgui:alert-state"
)

// Service 表示 GUI 应用服务。
type Service struct {
	ctx               context.Context
	store             *configstore.Store
	logs              *logbuffer.Buffer
	monitor           *monitoring.Manager
	settings          *configstore.Settings
	resolver          *localdb.Resolver
	tray              *TrayController
	mu                sync.RWMutex
	launchOnce        sync.Once
	launchMu          sync.Mutex
	frontendReady     bool
	trayReady         bool
	trayWaitExpired   bool
	launchTimer       *time.Timer
	restartMonitoring *bool
	adminRestartBusy  atomic.Bool

	alertMu              sync.Mutex
	lastAlert            *Alert
	lastNotice           time.Time
	alertKey             string
	alertRunID           uint64
	alertSequence        uint64
	alertStateRevision   uint64
	windowReportRevision uint64
	alertTimer           *time.Timer
	alertTimerRevision   uint64
	pendingAlert         *Alert
	nextDelivery         *Alert
	alertUnread          bool
	lastNoticeError      time.Time
	windowHidden         atomic.Bool
	windowForeground     atomic.Bool
	closing              atomic.Bool
	notificationBusy     atomic.Bool
	lastAlertDialogOpen  atomic.Bool
}

// NewService 创建 GUI 服务对象。
func NewService() (*Service, error) {
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	store := configstore.NewStoreWithPath(appName, filepath.Join(dir, appName+".yaml"))
	iproute.SetCacheDir(dir)

	settings, err := store.Load()
	if err != nil {
		return nil, err
	}
	service := &Service{
		store:    store,
		logs:     logbuffer.New(500),
		settings: settings,
	}
	// Initialize file logging before the resolver emits startup messages.
	service.initLogger()

	resolver, err := localdb.NewResolver()
	if err != nil {
		return nil, err
	}
	service.resolver = resolver

	checker := monitorcore.NewChecker(resolver)
	service.monitor = monitoring.NewManager(service.writeLog, checker.CheckProxyStatusResultContext)
	service.monitor.SetHealthListener(service.emitHealth)
	service.monitor.SetMatchListener(service.observeMatch)
	return service, nil
}

// Startup 记录 Wails 上下文并初始化日志。
func (s *Service) Startup(ctx context.Context) {
	s.launchMu.Lock()
	s.ctx = ctx
	s.windowHidden.Store(s.restartMonitoring == nil && s.Settings().StartMinimized)
	s.writeLog("INFO", "GUI application started")
	s.launchMu.Unlock()
	go s.ApplyLaunchPreferences()
}

// Shutdown 停止后台任务并释放日志器。
func (s *Service) Shutdown() {
	s.closing.Store(true)
	s.alertMu.Lock()
	s.cancelAlertTimerLocked()
	s.alertMu.Unlock()
	s.launchMu.Lock()
	if s.launchTimer != nil {
		s.launchTimer.Stop()
	}
	s.monitor.Stop()
	s.launchMu.Unlock()
	if s.resolver != nil {
		if err := s.resolver.Close(); err != nil {
			s.writeLog("ERROR", "Failed to close local IP databases: "+err.Error())
		}
	}
	s.writeLog("INFO", "GUI application stopped")
	_ = slogs.CloseAll()
}

// State 返回当前前端视图状态。
func (s *Service) State() *ViewState {
	s.mu.RLock()
	settings := configstore.NormalizeSettings(s.settings)
	s.mu.RUnlock()

	running := s.monitor.Running()
	latestAlert, alertUnread, alertRevision := s.alertSnapshot()
	lines, errorCount := s.logs.Snapshot()
	return &ViewState{
		AutoStartMonitoring: settings.AutoStartMonitoring,
		StartMinimized:      settings.StartMinimized,
		Language:            settings.Language,
		Interval:            settings.Interval,
		KeywordInput:        settings.KeywordInput,
		Regions:             append([]string(nil), settings.Regions...),
		APIEnabled:          append([]string(nil), settings.APIEnabled...),
		CustomAPIURL:        settings.CustomAPIURL,
		CloseAction:         settings.CloseAction,
		RemoveRouteOnMatch:  settings.RemoveRouteOnMatch,
		NotificationPopup:   settings.NotificationPopup,
		Logs:                lines,
		ErrorCount:          errorCount,
		ConfigPath:          s.store.Path(),
		IsAdmin:             isAdmin(),
		Running:             running,
		StatusText:          runningText(running),
		PermissionText:      permissionText(isAdmin()),
		Health:              s.monitor.Health(),
		LatestAlert:         latestAlert,
		AlertUnread:         alertUnread,
		AlertRevision:       alertRevision,
	}
}

// SaveSettings 保存界面配置。
func (s *Service) SaveSettings(input *configstore.Settings) (*ViewState, error) {
	normalized := configstore.NormalizeSettings(input)
	if normalized.RemoveRouteOnMatch {
		ctx, cancel := context.WithTimeout(s.operationContext(), 15*time.Second)
		defer cancel()
		admin := iproute.CheckAdminPermissionContext(ctx)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if admin {
			if err := iproute.ProtectionPreflight(ctx, false); err != nil {
				return nil, err
			}
		}
	}
	s.mu.Lock()
	if err := s.store.Save(normalized); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.settings = normalized
	current := s.settings
	s.mu.Unlock()

	s.mu.RLock()
	tray := s.tray
	s.mu.RUnlock()
	if tray != nil {
		tray.RefreshLanguage()
	}
	if s.monitor != nil {
		s.monitor.SetRemoveRouteOnMatch(current.RemoveRouteOnMatch)
	}
	s.writeLog("INFO", "Configuration saved")
	s.emitState()
	return s.State(), nil
}

// AddRegion 添加区域关键字并自动保存。
func (s *Service) AddRegion(keyword string) (*ViewState, error) {
	s.mu.Lock()
	settings := cloneSettings(s.settings)
	settings.KeywordInput = strings.TrimSpace(keyword)
	if settings.KeywordInput != "" {
		settings.Regions = append(settings.Regions, settings.KeywordInput)
	}
	s.mu.Unlock()
	return s.SaveSettings(settings)
}

// RemoveRegion 移除指定区域并自动保存。
func (s *Service) RemoveRegion(region string) (*ViewState, error) {
	s.mu.Lock()
	settings := cloneSettings(s.settings)
	settings.Regions = filterRegion(settings.Regions, region)
	s.mu.Unlock()
	return s.SaveSettings(settings)
}

// Settings 返回当前配置快照。
func (s *Service) Settings() *configstore.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSettings(s.settings)
}

// ConfigPath 返回当前配置文件路径。
func (s *Service) ConfigPath() string {
	return s.store.Path()
}

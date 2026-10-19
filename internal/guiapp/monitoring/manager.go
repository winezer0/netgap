package monitoring

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"netgap/internal/config"
	"netgap/internal/iproute"
	monitorcore "netgap/internal/monitor"
)

// CheckFunc 表示单次监测函数。
type CheckFunc func(cfg *config.Config) (*monitorcore.CheckResult, error)

// LoggerFunc 表示日志输出函数。
type LoggerFunc func(level, message string)

type RemoveRouteFunc func() (string, error)

// Manager 表示可启动和停止的监测管理器。
type Manager struct {
	mu           sync.RWMutex
	running      bool
	cancel       context.CancelFunc
	check        CheckFunc
	removeRoute  RemoveRouteFunc
	logf         LoggerFunc
	tickDuration func(seconds int) time.Duration
	localHandled bool
}

// NewManager 创建监测管理器。
func NewManager(logf LoggerFunc, checks ...CheckFunc) *Manager {
	var check CheckFunc
	if len(checks) > 0 {
		check = checks[0]
	}
	return &Manager{
		check:       check,
		removeRoute: defaultRemoveRoute,
		logf:        logf,
		tickDuration: func(seconds int) time.Duration {
			return time.Duration(seconds) * time.Second
		},
	}
}

// Start 启动监测循环。
func (m *Manager) Start(cfg *config.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return errors.New("monitor is already running")
	}
	if cfg == nil {
		return errors.New("config is nil")
	}
	if m.check == nil {
		return errors.New("monitor check function is nil")
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.running = true
	m.cancel = cancel
	m.localHandled = false
	go m.run(ctx, cloneConfig(cfg))
	return nil
}

// Stop 停止监测循环。
func (m *Manager) Stop() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return false
	}
	m.running = false
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	return true
}

// Running 返回当前是否正在监测。
func (m *Manager) Running() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

func (m *Manager) run(ctx context.Context, cfg *config.Config) {
	defer m.finishRun()

	m.writeLog("INFO", fmt.Sprintf("Monitoring started, interval=%ds, regions=%v", cfg.Interval, cfg.Regions))
	ticker := time.NewTicker(m.tickDuration(cfg.Interval))
	defer ticker.Stop()

	if m.checkOnce(cfg) {
		m.requestStop()
	}
	for {
		select {
		case <-ctx.Done():
			m.writeLog("INFO", "Monitoring stopped")
			return
		case <-ticker.C:
			select {
			case <-ctx.Done():
				m.writeLog("INFO", "Monitoring stopped")
				return
			default:
			}
			if m.checkOnce(cfg) {
				m.requestStop()
			}
		}
	}
}

func (m *Manager) checkOnce(cfg *config.Config) bool {
	result, err := m.check(cfg)
	fetcherName := ""
	if result != nil {
		fetcherName = result.FetcherName
	}
	if err != nil {
		m.writeLog("ERROR", fmt.Sprintf("Check failed via %s: %v", fetcherName, err))
		return false
	}
	if result == nil || result.IPInfo == nil {
		m.writeLog("WARN", fmt.Sprintf("Exit IP query empty via %s", fetcherName))
		return false
	}

	ip := result.IPInfo.IP
	location := result.LocationTag()
	if !result.IsLocal {
		m.localHandled = false
		m.writeLog("INFO", fmt.Sprintf("Proxy is healthy api[%s] area[%s] ip[%s]", fetcherName, location, ip))
		return false
	}

	m.writeLog("WARN", fmt.Sprintf("Local exit detected api[%s] area[%s] ip[%s]", fetcherName, location, ip))
	if m.localHandled {
		return false
	}
	m.localHandled = true

	message, routeErr := m.removeRoute()
	if routeErr != nil {
		m.writeLog("ERROR", fmt.Sprintf("Remove default route failed: %v", routeErr))
		return false
	}
	if message != "" {
		m.writeLog("WARN", message)
	}
	return true
}

func (m *Manager) writeLog(level, message string) {
	if m.logf != nil {
		m.logf(level, message)
	}
}

func (m *Manager) requestStop() {
	m.mu.RLock()
	cancel := m.cancel
	m.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
}

func (m *Manager) finishRun() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
	m.cancel = nil
	m.localHandled = false
}

func defaultRemoveRoute() (string, error) {
	if !iproute.CheckAdminPermission() {
		return "", errors.New("admin permission required")
	}
	route, err := iproute.GetDefaultRoute()
	if err != nil {
		return "", err
	}
	if err := iproute.SaveRouteInfo(route); err != nil {
		return "", err
	}
	if err := iproute.DeleteDefaultRoute(); err != nil {
		return "", err
	}
	return fmt.Sprintf("Default route removed, gateway=%s", route.Gateway), nil
}

func cloneConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}
	regions := make([]string, len(cfg.Regions))
	copy(regions, cfg.Regions)
	apiUse := make([]string, len(cfg.APIUse))
	copy(apiUse, cfg.APIUse)
	apiBan := make([]string, len(cfg.APIBan))
	copy(apiBan, cfg.APIBan)
	return &config.Config{
		Regions:      regions,
		Interval:     cfg.Interval,
		APIUse:       apiUse,
		APIBan:       apiBan,
		Shutdown:     cfg.Shutdown,
		CustomAPIURL: cfg.CustomAPIURL,
	}
}

package monitoring

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"netgap/internal/config"
	"netgap/internal/ipchecker"
	"netgap/internal/iproute"
	monitorcore "netgap/internal/monitor"
)

// CheckFunc 表示单次监测函数。
type CheckFunc func(ctx context.Context, cfg *config.Config) (*monitorcore.CheckResult, error)

// LoggerFunc 表示日志输出函数。
type LoggerFunc func(level, message string)

type RemoveRouteFunc func() (string, error)

// Each run owns its cancellation and check state, including while it is stopping.
type monitorRun struct {
	ctx              context.Context
	cancel           context.CancelFunc
	done             chan struct{}
	localHandled     bool
	proxyUnavailable bool
}

// Manager 表示可启动和停止的监测管理器。
type Manager struct {
	mu                 sync.RWMutex
	current            *monitorRun
	check              CheckFunc
	removeRoute        RemoveRouteFunc
	logf               LoggerFunc
	tickDuration       func(seconds int) time.Duration
	removeRouteOnMatch atomic.Bool
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

	if m.current != nil && m.current.ctx.Err() == nil {
		return errors.New("monitor is already running")
	}
	if cfg == nil {
		return errors.New("config is nil")
	}
	if cfg.Interval <= 0 {
		return errors.New("monitor interval must be positive")
	}
	if m.check == nil {
		return errors.New("monitor check function is nil")
	}
	m.removeRouteOnMatch.Store(cfg.RemoveRouteOnMatch)

	ctx, cancel := context.WithCancel(context.Background())
	run := &monitorRun{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	m.current = run
	go m.run(run, cloneConfig(cfg))
	return nil
}

// Stop 停止监测循环。
func (m *Manager) Stop() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current == nil || m.current.ctx.Err() != nil {
		return false
	}
	m.current.cancel()
	return true
}

// Running 返回当前是否正在监测。
func (m *Manager) Running() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current != nil && m.current.ctx.Err() == nil
}

// SetRemoveRouteOnMatch updates automatic route removal without restarting monitoring.
func (m *Manager) SetRemoveRouteOnMatch(enabled bool) {
	m.removeRouteOnMatch.Store(enabled)
}

func (m *Manager) run(run *monitorRun, cfg *config.Config) {
	defer m.finishRun(run)
	if !m.isCurrent(run) {
		return
	}

	m.writeLog("INFO", fmt.Sprintf("Monitoring started, interval=%ds, regions=%v", cfg.Interval, cfg.Regions))
	ticker := time.NewTicker(m.tickDuration(cfg.Interval))
	defer ticker.Stop()

	if m.checkOnce(run, cfg) {
		run.cancel()
	}
	for {
		select {
		case <-run.ctx.Done():
			return
		case <-ticker.C:
			select {
			case <-run.ctx.Done():
				return
			default:
			}
			if m.checkOnce(run, cfg) {
				run.cancel()
			}
		}
	}
}

func (m *Manager) checkOnce(run *monitorRun, cfg *config.Config) bool {
	if !m.isCurrent(run) {
		return false
	}
	result, err := m.check(run.ctx, cfg)
	if !m.isCurrent(run) {
		return false
	}
	fetcherName := ""
	if result != nil {
		fetcherName = result.FetcherName
	}
	if err != nil {
		if errors.Is(err, ipchecker.ErrProxyUnavailable) {
			if !run.proxyUnavailable {
				m.writeLog("WARN", fmt.Sprintf("Proxy unavailable; waiting for it to return: %v", err))
				run.proxyUnavailable = true
			}
			return false
		}
		m.writeLog("ERROR", fmt.Sprintf("Check failed via %s: %v", fetcherName, err))
		return false
	}
	if result == nil || result.IPInfo == nil {
		m.writeLog("WARN", fmt.Sprintf("Exit IP query empty via %s", fetcherName))
		return false
	}
	if run.proxyUnavailable {
		if result.Proxy != nil && result.Proxy.Enabled {
			m.writeLog("INFO", "Proxy connection restored")
		} else {
			m.writeLog("INFO", "Exit IP query resumed without an explicit proxy")
		}
		run.proxyUnavailable = false
	}

	ip := result.IPInfo.IP
	location := result.LocationTag()
	if !result.IsLocal {
		run.localHandled = false
		m.writeLog("INFO", fmt.Sprintf("Proxy is healthy api[%s] area[%s] ip[%s]", fetcherName, location, ip))
		return false
	}

	m.writeLog("WARN", fmt.Sprintf("Local exit detected api[%s] area[%s] ip[%s]", fetcherName, location, ip))
	if run.localHandled {
		return false
	}
	if !m.removeRouteOnMatch.Load() {
		return false
	}
	message, routeErr, attempted := m.removeRouteForRun(run)
	if !attempted {
		return false
	}
	if routeErr != nil {
		m.writeLog("ERROR", fmt.Sprintf("Remove default route failed: %v", routeErr))
		return false
	}
	run.localHandled = true
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

func (m *Manager) isCurrent(run *monitorRun) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current == run && run.ctx.Err() == nil
}

// Serialize route changes with Stop so no new action can begin after Stop returns.
func (m *Manager) removeRouteForRun(run *monitorRun) (string, error, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current != run || run.ctx.Err() != nil || !m.removeRouteOnMatch.Load() {
		return "", nil, false
	}
	message, err := m.removeRoute()
	return message, err, true
}

func (m *Manager) finishRun(run *monitorRun) {
	run.cancel()
	m.mu.Lock()
	wasCurrent := m.current == run
	if wasCurrent {
		m.current = nil
	}
	m.mu.Unlock()
	if wasCurrent {
		m.writeLog("INFO", "Monitoring stopped")
	}
	close(run.done)
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
		Regions:            regions,
		Interval:           cfg.Interval,
		APIUse:             apiUse,
		APIBan:             apiBan,
		Shutdown:           cfg.Shutdown,
		CustomAPIURL:       cfg.CustomAPIURL,
		RemoveRouteOnMatch: cfg.RemoveRouteOnMatch,
	}
}

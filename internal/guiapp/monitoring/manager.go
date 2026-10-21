package monitoring

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"netgap/internal/config"
	"netgap/internal/detector"
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
	stopLogged       atomic.Bool
	localHandled     bool
	proxyUnavailable bool
	pending          *iproute.DisconnectTask
	pendingLoaded    bool
	nextRetry        time.Time
	retryDelay       time.Duration
	health           monitorcore.HealthTracker
}

// Manager 表示可启动和停止的监测管理器。
type Manager struct {
	resumeDisconnect   func(context.Context, *iproute.DisconnectTask) error
	mu                 sync.RWMutex
	current            *monitorRun
	active             atomic.Pointer[monitorRun]
	check              CheckFunc
	removeRoute        RemoveRouteFunc
	logf               LoggerFunc
	tickDuration       func(seconds int) time.Duration
	removeRouteOnMatch atomic.Bool
	actionMu           sync.Mutex
	actionCancel       context.CancelFunc
	healthRun          atomic.Pointer[monitorRun]
	healthListener     atomic.Pointer[func(monitorcore.Health)]
}

// NewManager 创建监测管理器。
func NewManager(logf LoggerFunc, checks ...CheckFunc) *Manager {
	var check CheckFunc
	if len(checks) > 0 {
		check = checks[0]
	}
	return &Manager{
		check: check,
		logf:  logf,
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
	if cfg.RemoveRouteOnMatch && m.removeRoute == nil {
		ctx, cancel := context.WithTimeout(context.Background(), monitorcore.DisconnectAttemptTimeout)
		defer cancel()
		if err := iproute.ProtectionPreflight(ctx, false); err != nil {
			return err
		}
	}
	m.removeRouteOnMatch.Store(cfg.RemoveRouteOnMatch)

	ctx, cancel := context.WithCancel(context.Background())
	run := &monitorRun{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	run.health.Start(cfg.Interval)
	m.current = run
	m.active.Store(run)
	m.healthRun.Store(run)
	go m.run(run, cloneConfig(cfg))
	return nil
}

// Stop 停止监测循环。
func (m *Manager) Stop() bool {
	run := m.active.Load()
	if run == nil || run.ctx.Err() != nil {
		return false
	}
	run.cancel()
	run.health.Stop()
	m.notifyHealth(run)
	m.logStopped(run)
	return true
}

// Health remains readable while a route action is waiting on the OS.
func (m *Manager) Health() monitorcore.Health {
	run := m.healthRun.Load()
	if run == nil {
		return monitorcore.Health{Phase: "stopped", DisconnectState: "idle"}
	}
	result := run.health.Snapshot()
	if m.active.Load() != run || run.ctx.Err() != nil {
		result.Running, result.Phase = false, "stopped"
	}
	return result
}

func (m *Manager) SetHealthListener(listener func(monitorcore.Health)) {
	if listener == nil {
		m.healthListener.Store(nil)
		return
	}
	m.healthListener.Store(&listener)
}

// ObserveManualRouteAction returns a completion callback that cannot overwrite
// a newer automatic operation or a replacement monitor run.
func (m *Manager) ObserveManualRouteAction(state string) func(error) {
	run := m.healthRun.Load()
	if run == nil {
		return func(error) {}
	}
	revision := run.health.RouteRevision()
	return func(err error) {
		if m.healthRun.Load() == run && run.health.RecordManualRouteResult(revision, state, err) {
			m.notifyHealth(run)
		}
	}
}

func (m *Manager) notifyHealth(run *monitorRun) {
	if m.healthRun.Load() != run {
		return
	}
	if listener := m.healthListener.Load(); listener != nil {
		(*listener)(m.Health())
	}
}

// Running 返回当前是否正在监测。
func (m *Manager) Running() bool {
	run := m.active.Load()
	return run != nil && run.ctx.Err() == nil
}

// SetRemoveRouteOnMatch updates automatic route removal without restarting monitoring.
func (m *Manager) SetRemoveRouteOnMatch(enabled bool) {
	m.removeRouteOnMatch.Store(enabled)
	if !enabled {
		m.actionMu.Lock()
		if m.actionCancel != nil {
			m.actionCancel()
		}
		m.actionMu.Unlock()
	}
}

func (m *Manager) run(run *monitorRun, cfg *config.Config) {
	defer m.finishRun(run)
	if !m.isCurrent(run) {
		return
	}

	m.writeRunLog(run, "INFO", fmt.Sprintf("Monitoring started, interval=%ds, regions=%v", cfg.Interval, cfg.Regions))
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
	hadPending := false
	if !m.removeRouteOnMatch.Load() {
		if run.pending != nil {
			run.health.SetDisconnect("suspended", nil, time.Time{})
		}
		run.pending, run.pendingLoaded = nil, false
	} else if m.removeRoute == nil {
		if !run.pendingLoaded {
			ctx, cancel := context.WithTimeout(run.ctx, monitorcore.DisconnectAttemptTimeout)
			pending, err := iproute.PendingDefaultRouteDisconnect(ctx)
			cancel()
			if err != nil {
				run.health.SetDisconnect("failed", err, time.Time{})
				m.writeRunLog(run, "ERROR", "Read pending disconnect failed: "+err.Error())
			}
			run.pending, run.pendingLoaded = pending, true
			if pending != nil {
				m.writeRunLog(run, "WARN", "Resuming saved disconnect after checking its route/network scope")
			}
		}
		if run.pending != nil {
			hadPending = true
			if !time.Now().Before(run.nextRetry) && m.performRouteAction(run, nil) {
				return true
			}
		}
	}
	// Always query during backoff or after a failed retry. A fresh result must
	// never widen an existing capability or start another action in this tick.
	if !m.isCurrent(run) {
		return false
	}
	run.health.BeginCheck(cfg.Interval)
	m.notifyHealth(run)
	result, err := m.check(run.ctx, cfg)
	if !m.isCurrent(run) {
		return false
	}
	fetcherName := ""
	if result != nil {
		fetcherName = result.FetcherName
	}
	if err == nil && (result == nil || result.IPInfo == nil) {
		err = errors.New("exit IP query returned no location information")
	}
	previous := run.health.Snapshot()
	reason := ""
	if result != nil {
		reason = result.ObservationOnly
	}
	if err != nil && reason == "" {
		reason = previous.ObservationOnly
	}
	run.health.CompleteObservation(err, reason)
	if reason != "" {
		run.health.SetDisconnect("suspended", nil, time.Time{})
		if previous.ObservationOnly == "" {
			m.writeRunLog(run, "WARN", detector.ObservationWarning)
		}
	} else if err == nil && previous.ObservationOnly != "" {
		m.writeRunLog(run, "INFO", "Normal proxy monitoring resumed")
		if run.pending == nil && previous.DisconnectState == "suspended" {
			run.health.SetDisconnect("idle", nil, time.Time{})
		}
	}
	m.notifyHealth(run)
	if !m.isCurrent(run) {
		return false
	}
	if err != nil {
		if errors.Is(err, ipchecker.ErrProxyUnavailable) {
			if !run.proxyUnavailable {
				m.writeRunLog(run, "WARN", fmt.Sprintf("Proxy unavailable; waiting for it to return: %v", err))
				run.proxyUnavailable = true
			}
			return false
		}
		m.writeRunLog(run, "ERROR", fmt.Sprintf("Check failed via %s [state=%s]: %v", fetcherName, monitorcore.FailureKind(err), err))
		return false
	}
	if run.proxyUnavailable {
		if result.Proxy != nil && result.Proxy.Enabled {
			m.writeRunLog(run, "INFO", "Proxy connection restored")
		} else {
			m.writeRunLog(run, "INFO", "Exit IP query resumed without an explicit proxy")
		}
		run.proxyUnavailable = false
	}

	ip := result.IPInfo.IP
	location := result.LocationTag()
	if !result.IsLocal {
		run.localHandled = false
		m.writeRunLog(run, "INFO", fmt.Sprintf("Exit outside monitored regions api[%s] area[%s] ip[%s] path[%s]", fetcherName, location, ip, result.PathTag()))
		return false
	}

	m.writeRunLog(run, "WARN", fmt.Sprintf("Local exit detected api[%s] area[%s] ip[%s] path[%s]", fetcherName, location, ip, result.PathTag()))
	if result.ObservationOnly != "" {
		return false
	}
	if run.localHandled {
		return false
	}
	if !m.removeRouteOnMatch.Load() {
		return false
	}
	if hadPending {
		return false
	}
	return m.performRouteAction(run, result.ProtectionGuard)
}

func (m *Manager) performRouteAction(run *monitorRun, validate func(context.Context) error) bool {
	run.health.SetDisconnect("running", nil, time.Time{})
	m.notifyHealth(run)
	message, routeErr, attempted := m.removeRouteForRun(run, validate)
	if !attempted {
		if run.pending != nil {
			run.health.SetDisconnect("pending", nil, run.nextRetry)
		} else {
			run.health.SetDisconnect("idle", nil, time.Time{})
		}
		m.notifyHealth(run)
		return false
	}
	if errors.Is(routeErr, detector.ErrObservationOnly) {
		run.health.SetDisconnect("suspended", nil, time.Time{})
		m.notifyHealth(run)
		return false
	}
	if routeErr != nil {
		var pending *iproute.PendingDisconnectError
		if errors.As(routeErr, &pending) {
			run.pending = pending.Task
		}
		if errors.Is(routeErr, detector.ErrStateChanged) || errors.Is(routeErr, iproute.ErrDisconnectObsolete) {
			run.pending = nil
		}
		if run.pending != nil {
			if run.retryDelay == 0 {
				run.retryDelay = time.Second
			} else {
				run.retryDelay *= 2
			}
			if run.retryDelay > 30*time.Second {
				run.retryDelay = 30 * time.Second
			}
			run.nextRetry = time.Now().Add(run.retryDelay)
			run.health.SetDisconnect("pending", routeErr, run.nextRetry)
			m.writeRunLog(run, "WARN", fmt.Sprintf("Disconnect pending; retry after %s; exit checks continue", run.retryDelay))
		} else {
			run.retryDelay, run.nextRetry = 0, time.Time{}
			if errors.Is(routeErr, iproute.ErrDisconnectObsolete) {
				run.health.SetDisconnect("idle", nil, time.Time{})
			} else {
				run.health.SetDisconnect("failed", routeErr, time.Time{})
			}
		}
		m.notifyHealth(run)
		if errors.Is(routeErr, iproute.ErrDisconnectObsolete) {
			m.writeRunLog(run, "INFO", "Saved disconnect is obsolete; exit checks continue")
		} else {
			m.writeRunLog(run, "ERROR", fmt.Sprintf("Remove default route failed: %v", routeErr))
		}
		return false
	}
	run.localHandled = true
	run.pending = nil
	run.retryDelay, run.nextRetry = 0, time.Time{}
	run.health.SetDisconnect("completed", nil, time.Time{})
	m.notifyHealth(run)
	if message != "" {
		m.writeRunLog(run, "WARN", message)
	}
	return true
}

func (m *Manager) writeLog(level, message string) {
	if m.logf != nil {
		m.logf(level, message)
	}
}

// Gate publications without taking the route-action lock: Stop must remain
// responsive even while an OS route operation is still unwinding.
func (m *Manager) writeRunLog(run *monitorRun, level, message string) {
	if m.active.Load() == run && run.ctx.Err() == nil {
		m.writeLog(level, message)
	}
}

func (m *Manager) logStopped(run *monitorRun) {
	if m.active.Load() == run && run.stopLogged.CompareAndSwap(false, true) {
		m.writeLog("INFO", "Monitoring stopped")
	}
}

func (m *Manager) isCurrent(run *monitorRun) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current == run && run.ctx.Err() == nil
}

// Serialize run replacement with route actions. Stop cancels without waiting
// for this lock, so a slow OS command cannot freeze the stop control.
func (m *Manager) removeRouteForRun(run *monitorRun, validate func(context.Context) error) (string, error, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current != run || run.ctx.Err() != nil || !m.removeRouteOnMatch.Load() {
		return "", nil, false
	}
	actionCtx, cancel := context.WithTimeout(run.ctx, monitorcore.DisconnectAttemptTimeout)
	m.actionMu.Lock()
	m.actionCancel = cancel
	if !m.removeRouteOnMatch.Load() {
		cancel()
	}
	m.actionMu.Unlock()
	defer func() {
		cancel()
		m.actionMu.Lock()
		m.actionCancel = nil
		m.actionMu.Unlock()
	}()
	if err := detector.AutomaticProtectionError(actionCtx); err != nil {
		return "", err, true
	}
	if validate != nil {
		if err := validate(actionCtx); err != nil {
			return "", err, true
		}
	}
	remove := m.removeRoute
	if remove == nil {
		remove = func() (string, error) {
			if run.pending != nil && m.resumeDisconnect != nil {
				return "", m.resumeDisconnect(actionCtx, run.pending)
			}
			if !iproute.CheckAdminPermissionContext(actionCtx) {
				if err := actionCtx.Err(); err != nil {
					return "", err
				}
				return "", monitorcore.ErrRoutePermission
			}
			if run.pending != nil {
				if err := iproute.ResumeDefaultRouteDisconnect(actionCtx, run.pending); err != nil {
					return "", err
				}
				return "All IPv4/IPv6 default routes removed; complete recovery snapshot saved", nil
			}
			guard := func(ctx context.Context) error {
				if !m.removeRouteOnMatch.Load() {
					return errors.New("automatic route removal was disabled")
				}
				if validate != nil {
					return validate(ctx)
				}
				return ctx.Err()
			}
			if err := iproute.DeleteAllDefaultRoutesGuarded(actionCtx, guard); err != nil {
				return "", err
			}
			return "All IPv4/IPv6 default routes removed; complete recovery snapshot saved", nil
		}
	}
	message, err := remove()
	return message, err, true
}

func (m *Manager) finishRun(run *monitorRun) {
	run.cancel()
	run.health.Stop()
	m.logStopped(run)
	m.mu.Lock()
	wasCurrent := m.current == run
	if wasCurrent {
		m.notifyHealth(run)
		m.current = nil
		m.active.CompareAndSwap(run, nil)
	}
	m.mu.Unlock()
	close(run.done)
}

func defaultRemoveRoute() (string, error) {
	if !iproute.CheckAdminPermission() {
		return "", monitorcore.ErrRoutePermission
	}
	if err := iproute.DeleteDefaultRoute(); err != nil {
		return "", err
	}
	return "All IPv4/IPv6 default routes removed; complete recovery snapshot saved", nil
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

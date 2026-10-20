package monitor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	logging "github.com/winezer0/zaplogs"

	"netgap/internal/config"
	"netgap/internal/detector"
	"netgap/internal/ipchecker"
	"netgap/internal/ipchecker/types"
	"netgap/internal/iproute"
)

// IPResolver resolves an IP address using local location data.
type IPResolver interface {
	Resolve(ip string) (*types.IPInfo, error)
}

// Checker combines external IP acquisition with local IP location lookup.
type Checker struct {
	resumeDisconnect func(context.Context, *iproute.DisconnectTask) error
	detectProxy      func(context.Context) (*detector.ProxyConfig, string, error)
	state            func(context.Context) (string, error)
	resolver         IPResolver
	lookupIP         func(context.Context, *detector.ProxyConfig, []string, []string, string) (string, error, string)
	removeRoute      func() error
	pending          *iproute.DisconnectTask
	pendingLoaded    bool
	nextRetry        time.Time
	retryDelay       time.Duration
	health           HealthTracker
}

// NewChecker creates a monitor checker with a local IP resolver.
func NewChecker(resolver IPResolver) *Checker {
	return &Checker{detectProxy: detector.DetectProxyWithSource, state: detector.StateFingerprint, resolver: resolver, lookupIP: ipchecker.GetExternalIPContext}
}

// CheckProxyStatusResultContext propagates monitor cancellation to the IP query.
func (c *Checker) CheckProxyStatusResultContext(ctx context.Context, cfg *config.Config) (*CheckResult, error) {
	result := &CheckResult{}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if cfg == nil {
		return result, fmt.Errorf("monitor config is nil")
	}
	if c == nil || c.resolver == nil {
		return result, fmt.Errorf("local IP resolver is unavailable")
	}
	if c.state != nil {
		before, err := c.state(ctx)
		if err != nil {
			return result, err
		}
		result.ValidateCurrent = func(checkCtx context.Context) error {
			after, err := c.state(checkCtx)
			if err != nil {
				return err
			}
			if before != after {
				return fmt.Errorf("%w; discarding stale IP result", detector.ErrStateChanged)
			}
			return nil
		}
	}
	detect := c.detectProxy
	if detect == nil {
		detect = detector.DetectProxyWithSource
	}
	proxy, _, err := detect(ctx)
	if err != nil {
		return result, err
	}
	result.Proxy = proxy

	queryCtx := ipchecker.ObserveRoute(ctx, func(actual *detector.ProxyConfig) { result.Proxy = actual })
	ip, err, fetcherName := c.lookupIP(queryCtx, proxy, cfg.APIUse, cfg.APIBan, cfg.CustomAPIURL)
	result.FetcherName = fetcherName
	if err != nil {
		return result, err
	}
	if ip == "" {
		return result, fmt.Errorf("external IP query returned an empty address")
	}
	ipInfo, err := c.resolver.Resolve(ip)
	if err != nil {
		return result, fmt.Errorf("%w: resolve exit IP %s locally: %w", ErrLocalResolution, ip, err)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if ipInfo == nil {
		return result, fmt.Errorf("%w: empty information for %s", ErrLocalResolution, ip)
	}
	result.IPInfo = ipInfo

	isLocal := false
	for _, region := range cfg.Regions {
		if strings.Contains(ipInfo.Region, region) || strings.Contains(ipInfo.City, region) {
			isLocal = true
			break
		}
	}
	result.IsLocal = isLocal
	if result.ValidateCurrent != nil {
		if err := result.ValidateCurrent(ctx); err != nil {
			return result, err
		}
	}
	return result, nil
}

// StartMonitoring 启动监控
func StartMonitoring(ctx context.Context, cfg *config.Config, checker *Checker) {
	logging.Infof("开始监控代理网络状态，本地区域: %v, 检查间隔: %d秒", cfg.Regions, cfg.Interval)

	ticker := time.NewTicker(time.Duration(cfg.Interval) * time.Second)
	defer ticker.Stop()
	runMonitoring(ctx, cfg, checker, ticker.C)
}

func runMonitoring(ctx context.Context, cfg *config.Config, checker *Checker, ticks <-chan time.Time) {
	checker.health.Start(cfg.Interval)
	defer checker.health.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok {
				return
			}
		}
		if ctx.Err() != nil {
			return
		}
		status, err, fetcherName := checker.CheckProxyStatus(ctx, cfg)
		logging.Infof("Monitor health: %s", checker.Health())
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			logging.Errorf("[接口: %s] 检测或保护操作未完成 [state=%s]: %v", fetcherName, FailureKind(err), err)
			continue
		}

		if !status {
			logging.Warnf("[接口: %s] 代理可能已失效，检测到本地网络出口", fetcherName)

			// 检测到本地网络出口且启用了shutdown功能，路由已删除，退出监控
			if cfg.Shutdown {
				logging.Infof("网络已中断，停止监控")
				return
			}
		} else {
			logging.Infof("[接口: %s] 出口未命中监控地区", fetcherName)
		}
	}
}

// CheckProxyStatus 检查代理状态
// CheckProxyStatus checks the proxy exit location against configured regions.
func (c *Checker) CheckProxyStatus(ctx context.Context, cfg *config.Config) (bool, error, string) {
	if err := ctx.Err(); err != nil {
		return false, err, ""
	}
	if cfg == nil || c == nil {
		return false, errors.New("monitor config/checker unavailable"), ""
	}
	if !cfg.Shutdown {
		if c.pending != nil {
			c.health.SetDisconnect("suspended", nil, time.Time{})
		}
		c.pending, c.pendingLoaded = nil, false
	}
	hadPending := false
	var resumeErr error
	if cfg.Shutdown && c.removeRoute == nil {
		if !c.pendingLoaded {
			actionCtx, cancel := context.WithTimeout(ctx, DisconnectAttemptTimeout)
			pending, err := iproute.PendingDefaultRouteDisconnect(actionCtx)
			cancel()
			if err != nil {
				c.health.SetDisconnect("failed", err, time.Time{})
				logging.Errorf("读取待完成断网任务失败，继续出口检测: %v", err)
			}
			c.pending, c.pendingLoaded = pending, true
		}
		if c.pending != nil {
			hadPending = true
			if !time.Now().Before(c.nextRetry) {
				c.health.SetDisconnect("running", nil, time.Time{})
				actionCtx, cancel := context.WithTimeout(ctx, DisconnectAttemptTimeout)
				resume := c.resumeDisconnect
				if resume == nil {
					resume = iproute.ResumeDefaultRouteDisconnect
				}
				resumeErr = resume(actionCtx, c.pending)
				cancel()
				if resumeErr != nil {
					c.trackDisconnectFailure(resumeErr)
					logging.Warnf("续作未完成，继续出口检测: %v", resumeErr)
				} else {
					c.pending, c.retryDelay, c.nextRetry = nil, 0, time.Time{}
					c.health.SetDisconnect("completed", nil, time.Time{})
					return false, nil, "saved disconnect"
				}
			}
		}
	}
	c.health.BeginCheck(cfg.Interval)
	result, err := c.CheckProxyStatusResultContext(ctx, cfg)
	c.health.CompleteCheck(err)
	fetcherName := ""
	if result != nil {
		fetcherName = result.FetcherName
	}
	if err != nil {
		logging.Errorf("[int: %s] obtain external IP info error: %v", fetcherName, err)
		return false, err, fetcherName
	}

	ipInfo := result.IPInfo
	if ipInfo == nil {
		return false, fmt.Errorf("exit IP information is empty"), fetcherName
	}

	logging.Infof("[int: %s] Current IP: %s, Region: %s|%s|%s, ISN: %s",
		fetcherName, ipInfo.IP, ipInfo.Country, ipInfo.Region, ipInfo.City, ipInfo.Isp)
	logging.Infof("[int: %s] Actual request path: %s", fetcherName, result.PathTag())
	// A fresh observation must not replace or widen the saved operation. Even
	// if it became obsolete this round, wait until the next tick to act again.
	if hadPending {
		if result.IsLocal {
			logging.Warnf("Local exit still detected; saved disconnect is handled separately")
		}
		if c.pending != nil {
			return !result.IsLocal, &iproute.PendingDisconnectError{Task: c.pending, Err: errors.New("saved disconnect waiting for bounded retry; exit query completed")}, fetcherName
		}
		if resumeErr != nil {
			return !result.IsLocal, resumeErr, fetcherName
		}
	}

	if result.IsLocal {
		logging.Warnf("[int: %s] Detected local network, performing network interruption", fetcherName)
		if cfg.Shutdown {
			actionCtx, cancel := context.WithTimeout(ctx, DisconnectAttemptTimeout)
			defer cancel()
			if result.ValidateCurrent != nil {
				if err := result.ValidateCurrent(actionCtx); err != nil {
					c.health.SetDisconnect("failed", err, time.Time{})
					return false, err, fetcherName
				}
			}
			remove := c.removeRoute
			if remove == nil {
				remove = func() error {
					return iproute.DeleteAllDefaultRoutesGuarded(actionCtx, result.ValidateCurrent)
				}
			}
			c.health.SetDisconnect("running", nil, time.Time{})
			if err := remove(); err != nil {
				c.trackDisconnectFailure(err)
				logging.Errorf("delete the default route error: %v", err)
				return false, fmt.Errorf("delete default route: %w", err), fetcherName
			} else {
				c.retryDelay, c.nextRetry = 0, time.Time{}
				c.health.SetDisconnect("completed", nil, time.Time{})
				logging.Infof("delete the default route success...")
			}
		} else {
			logging.Warnf("[int: %s] shutdown function has not been enabled.", fetcherName)
		}
	}

	return !result.IsLocal, nil, fetcherName
}

func (c *Checker) trackDisconnectFailure(err error) {
	var pending *iproute.PendingDisconnectError
	if errors.As(err, &pending) {
		c.pending = pending.Task
	}
	if errors.Is(err, detector.ErrStateChanged) || errors.Is(err, iproute.ErrDisconnectObsolete) {
		c.pending = nil
	}
	if c.pending != nil {
		if c.retryDelay == 0 {
			c.retryDelay = time.Second
		} else {
			c.retryDelay *= 2
		}
		if c.retryDelay > 30*time.Second {
			c.retryDelay = 30 * time.Second
		}
		c.nextRetry = time.Now().Add(c.retryDelay)
		c.health.SetDisconnect("pending", err, c.nextRetry)
		logging.Warnf("断网未完成，将在 %s 后重试剩余路由，出口查询继续", c.retryDelay)
	} else {
		c.retryDelay, c.nextRetry = 0, time.Time{}
		c.health.SetDisconnect("failed", err, time.Time{})
	}
}

func (c *Checker) Health() Health { return c.health.Snapshot() }

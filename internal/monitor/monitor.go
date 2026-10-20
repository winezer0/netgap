package monitor

import (
	"context"
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
	state       func(context.Context) (string, error)
	resolver    IPResolver
	lookupIP    func(context.Context, *detector.ProxyConfig, []string, []string, string) (string, error, string)
	removeRoute func() error
}

// NewChecker creates a monitor checker with a local IP resolver.
func NewChecker(resolver IPResolver) *Checker {
	return &Checker{state: detector.StateFingerprint, resolver: resolver, lookupIP: ipchecker.GetExternalIPContext}
}

// CheckProxyStatusResult obtains the exit IP and resolves its location locally.
func (c *Checker) CheckProxyStatusResult(cfg *config.Config) (*CheckResult, error) {
	return c.CheckProxyStatusResultContext(context.Background(), cfg)
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
	proxy, _, err := detector.DetectProxyWithSource()
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
func StartMonitoring(cfg *config.Config, checker *Checker) {
	logging.Infof("开始监控代理网络状态，本地区域: %v, 检查间隔: %d秒", cfg.Regions, cfg.Interval)

	ticker := time.NewTicker(time.Duration(cfg.Interval) * time.Second)
	defer ticker.Stop()
	runMonitoring(cfg, checker, ticker.C)
}

func runMonitoring(cfg *config.Config, checker *Checker, ticks <-chan time.Time) {
	for range ticks {
		status, err, fetcherName := checker.CheckProxyStatus(cfg)
		if err != nil {
			logging.Errorf("[接口: %s] 检测失败 [state=%s]: %v", fetcherName, FailureKind(err), err)
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
func (c *Checker) CheckProxyStatus(cfg *config.Config) (bool, error, string) {
	result, err := c.CheckProxyStatusResult(cfg)
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

	if result.IsLocal {
		logging.Warnf("[int: %s] Detected local network, performing network interruption", fetcherName)
		if cfg.Shutdown {
			if result.ValidateCurrent != nil {
				if err := result.ValidateCurrent(context.Background()); err != nil {
					return false, err, fetcherName
				}
			}
			remove := c.removeRoute
			if remove == nil {
				remove = func() error {
					return iproute.DeleteAllDefaultRoutesGuarded(context.Background(), result.ValidateCurrent)
				}
			}
			if err := remove(); err != nil {
				logging.Errorf("delete the default route error: %v", err)
				return false, fmt.Errorf("delete default route: %w", err), fetcherName
			} else {
				logging.Infof("delete the default route success...")
			}
		} else {
			logging.Warnf("[int: %s] shutdown function has not been enabled.", fetcherName)
		}
	}

	return !result.IsLocal, nil, fetcherName
}

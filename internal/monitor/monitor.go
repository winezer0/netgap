package monitor

import (
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
	resolver IPResolver
}

// NewChecker creates a monitor checker with a local IP resolver.
func NewChecker(resolver IPResolver) *Checker {
	return &Checker{resolver: resolver}
}

// CheckProxyStatusResult obtains the exit IP and resolves its location locally.
func (c *Checker) CheckProxyStatusResult(cfg *config.Config) (*CheckResult, error) {
	result := &CheckResult{}
	if cfg == nil {
		return result, fmt.Errorf("monitor config is nil")
	}
	if c == nil || c.resolver == nil {
		return result, fmt.Errorf("local IP resolver is unavailable")
	}
	proxy, _, err := detector.DetectProxyWithSource()
	if err != nil {
		return result, err
	}
	result.Proxy = proxy

	ip, err, fetcherName := ipchecker.GetExternalIP(proxy, cfg.APIUse, cfg.APIBan, cfg.CustomAPIURL)
	result.FetcherName = fetcherName
	if err != nil {
		return result, err
	}
	if ip == "" {
		return result, fmt.Errorf("external IP query returned an empty address")
	}
	ipInfo, err := c.resolver.Resolve(ip)
	if err != nil {
		return result, fmt.Errorf("resolve exit IP %s locally: %w", ip, err)
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
	return result, nil
}

// StartMonitoring 启动监控
func StartMonitoring(cfg *config.Config, checker *Checker) {
	logging.Infof("开始监控代理网络状态，本地区域: %v, 检查间隔: %d秒", cfg.Regions, cfg.Interval)

	ticker := time.NewTicker(time.Duration(cfg.Interval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		status, err, fetcherName := checker.CheckProxyStatus(cfg)
		if err != nil {
			logging.Errorf("[接口: %s] 检查代理状态失败: %v", fetcherName, err)
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
			logging.Infof("[接口: %s] 代理工作正常", fetcherName)
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
		logging.Warnf("[int: %s] IP info is empty, Skip this check", fetcherName)
		return true, nil, fetcherName
	}

	logging.Infof("[int: %s] Current IP: %s, Region: %s|%s|%s, ISN: %s",
		fetcherName, ipInfo.IP, ipInfo.Country, ipInfo.Region, ipInfo.City, ipInfo.Isp)

	if result.IsLocal {
		logging.Warnf("[int: %s] Detected local network, performing network interruption", fetcherName)
		if cfg.Shutdown {
			if err := iproute.DeleteDefaultRoute(); err != nil {
				logging.Errorf("delete the default route error: %v", err)
			} else {
				logging.Infof("delete the default route success...")
			}
		} else {
			logging.Warnf("[int: %s] shutdown function has not been enabled.", fetcherName)
		}
	}

	return !result.IsLocal, nil, fetcherName
}

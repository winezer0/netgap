package ipchecker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"sync"
	"time"

	logging "github.com/winezer0/zaplogs"

	"netgap/internal/detector"
	"netgap/internal/ipchecker/fetchers"
	"netgap/internal/ipchecker/types"
)

var (
	ErrProxyUnavailable    = errors.New("proxy listener is unavailable")
	ErrNoAvailableFetchers = errors.New("no IP query API is currently available")
	ErrAPIResponse         = errors.New("invalid IP API response")
	ipFetchers             = make(map[string]types.IPFetcher)
	errorCounts            = make(map[string]int)
	cooldownUntil          = make(map[string]time.Time)
	fetchersMux            sync.RWMutex
	initialized            bool
	initializedMux         sync.Once
	maxErrorCount          = 10
	roundRobinMux          sync.Mutex
	roundRobinStart        int
)

const (
	perFetcherTimeout = 5 * time.Second
	lookupTimeout     = 10 * time.Second
	fetcherCooldown   = 30 * time.Second
)

// 初始化IP查询器
func initFetchers() {
	initializedMux.Do(func() {
		pcjsonFetcher := fetchers.NewPConlineIPJsonFetcher()
		ip2locationFetcher := fetchers.NewIP2LocationFetcher()
		customIPFetcher := fetchers.NewCustomIPFetcher()
		ipconfigFetcher := fetchers.NewIPConfigIOFetcher()
		ipifyFetcher := fetchers.NewIPifyFetcher()
		seeipFetcher := fetchers.NewSeeIPFetcher()
		ipapiFetcher := fetchers.NewIPAPIQueryFetcher()

		RegisterFetcher(pcjsonFetcher)
		RegisterFetcher(ip2locationFetcher)
		RegisterFetcher(customIPFetcher)
		RegisterFetcher(ipconfigFetcher)
		RegisterFetcher(ipifyFetcher)
		RegisterFetcher(seeipFetcher)
		RegisterFetcher(ipapiFetcher)

		fetchersMux.Lock()
		errorCounts[pcjsonFetcher.Name()] = 0
		errorCounts[ip2locationFetcher.Name()] = 0
		errorCounts[customIPFetcher.Name()] = 0
		errorCounts[ipconfigFetcher.Name()] = 0
		errorCounts[ipifyFetcher.Name()] = 0
		errorCounts[seeipFetcher.Name()] = 0
		errorCounts[ipapiFetcher.Name()] = 0
		fetchersMux.Unlock()

		initialized = true
	})
}

// RegisterFetcher 注册IP查询器
func RegisterFetcher(fetcher types.IPFetcher) {
	fetchersMux.Lock()
	defer fetchersMux.Unlock()
	ipFetchers[fetcher.Name()] = fetcher
	errorCounts[fetcher.Name()] = 0
	delete(cooldownUntil, fetcher.Name())
}

// GetExternalIP 获取外部IP信息
func GetExternalIP(proxy *detector.ProxyConfig, apiUse, apiBan []string, customAPIURL string) (string, error, string) {
	return GetExternalIPContext(context.Background(), proxy, apiUse, apiBan, customAPIURL)
}

// GetExternalIPContext keeps queries cancellable and retains failed APIs for recovery.
func GetExternalIPContext(ctx context.Context, proxy *detector.ProxyConfig, apiUse, apiBan []string, customAPIURL string) (string, error, string) {
	if err := ctx.Err(); err != nil {
		return "", err, ""
	}
	initFetchers()

	client := &http.Client{
		Timeout:   perFetcherTimeout,
		Transport: &systemTransport{config: proxy},
	}

	fetchers := getRoundRobinFetchers(apiUse, apiBan)
	if len(fetchers) == 0 {
		return "", ErrNoAvailableFetchers, ""
	}

	logging.Debugf("GetExternalIP: 可用查询器数量: %d", len(fetchers))
	for i, f := range fetchers {
		logging.Debugf("  %d. %s (%s)", i+1, f.Name(), f.Key())
	}

	var lastErr error
	var lastFetcherName string
	lookupCtx, cancelLookup := context.WithTimeout(ctx, lookupTimeout)
	defer cancelLookup()
	for _, fetcher := range fetchers {
		fetcher = configuredFetcher(fetcher, customAPIURL)
		if err := lookupCtx.Err(); err != nil {
			lastErr = err
			break
		}

		fetcherName := fetcher.Name()
		fetcherCtx, cancelFetcher := context.WithTimeout(lookupCtx, perFetcherTimeout)
		ip, err := fetcher.Fetch(fetcherCtx, client)
		cancelFetcher()
		if ctx.Err() != nil {
			return "", ctx.Err(), fetcherName
		}

		if err != nil {
			logging.Debugf("查询器 %s 结果: error=%v", fetcherName, err)
		} else if ip != "" {
			logging.Debugf("查询器 %s 结果: IP=%s", fetcherName, ip)
		} else {
			logging.Debugf("查询器 %s 结果: IP为空", fetcherName)
		}
		var connectionErr *net.OpError
		if proxy != nil && proxy.Enabled && !proxy.UsesURLRules() && errors.As(err, &connectionErr) && connectionErr.Op == "proxyconnect" {
			return "", fmt.Errorf("%w (%s): %w", ErrProxyUnavailable, net.JoinHostPort(proxy.Address, proxy.Port), err), fetcherName
		}

		if err == nil && ip != "" {
			address, parseErr := netip.ParseAddr(ip)
			if parseErr != nil {
				increaseErrorCount(fetcherName)
				lastErr = fmt.Errorf("%w: %w", ErrAPIResponse, parseErr)
				lastFetcherName = fetcherName
				continue
			}
			resetErrorCount(fetcherName)
			return address.Unmap().String(), nil, fetcherName
		}
		if err == nil {
			err = errors.New("API returned an empty IP address")
		}
		if !isNetworkError(err) {
			increaseErrorCount(fetcherName)
			err = fmt.Errorf("%w: %w", ErrAPIResponse, err)
		}
		lastErr = err
		lastFetcherName = fetcherName
		if lookupCtx.Err() != nil {
			lastErr = lookupCtx.Err()
			break
		}
	}

	return "", lastErr, lastFetcherName
}

// Connection, DNS and cancellation failures do not establish an API fault.
func isNetworkError(err error) bool {
	var requestErr *url.Error
	var networkErr net.Error
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.As(err, &requestErr) || errors.As(err, &networkErr)
}

// increaseErrorCount 增加错误计数
func increaseErrorCount(fetcherName string) {
	fetchersMux.Lock()
	defer fetchersMux.Unlock()

	errorCounts[fetcherName]++

	if errorCounts[fetcherName] > maxErrorCount {
		cooldownUntil[fetcherName] = time.Now().Add(fetcherCooldown)
		errorCounts[fetcherName] = 0
	}
}

// resetErrorCount 重置错误计数
func resetErrorCount(fetcherName string) {
	fetchersMux.Lock()
	defer fetchersMux.Unlock()

	errorCounts[fetcherName] = 0
	delete(cooldownUntil, fetcherName)
}

// GetAllFetchers 获取所有已注册的查询器
func GetAllFetchers() []types.IPFetcher {
	initFetchers()

	fetchersMux.RLock()
	defer fetchersMux.RUnlock()

	result := make([]types.IPFetcher, 0, len(ipFetchers))
	for _, fetcher := range ipFetchers {
		result = append(result, fetcher)
	}

	return result
}

// FilterFetchers 根据配置过滤查询器
func FilterFetchers(apiUse, apiBan []string) []types.IPFetcher {
	allFetchers := GetAllFetchers()

	if len(apiUse) == 0 {
		if len(apiBan) == 0 {
			return allFetchers
		}

		banMap := make(map[string]bool)
		for _, ban := range apiBan {
			banMap[ban] = true
		}

		var filtered []types.IPFetcher
		for _, fetcher := range allFetchers {
			key := fetcher.Key()
			banned := false
			for ban := range banMap {
				if len(ban) <= len(key) && key[:len(ban)] == ban {
					banned = true
					break
				}
			}
			if !banned {
				filtered = append(filtered, fetcher)
			}
		}
		return filtered
	}

	useMap := make(map[string]bool)
	for _, use := range apiUse {
		useMap[use] = true
	}

	var filtered []types.IPFetcher
	for _, fetcher := range allFetchers {
		key := fetcher.Key()
		for use := range useMap {
			if len(use) <= len(key) && key[:len(use)] == use {
				filtered = append(filtered, fetcher)
				break
			}
		}
	}

	return filtered
}

// getRandomizedFetchers 获取所有已注册的查询器并随机排序
// getRoundRobinFetchers rotates the starting fetcher while keeping fallback order stable.
func getRoundRobinFetchers(apiUse, apiBan []string) []types.IPFetcher {
	fetchers := FilterFetchers(apiUse, apiBan)
	fetchersMux.RLock()
	available := fetchers[:0]
	for _, fetcher := range fetchers {
		if !time.Now().Before(cooldownUntil[fetcher.Name()]) {
			available = append(available, fetcher)
		}
	}
	fetchersMux.RUnlock()
	fetchers = available
	sort.Slice(fetchers, func(i, j int) bool {
		if fetchers[i].Key() == fetchers[j].Key() {
			return fetchers[i].Name() < fetchers[j].Name()
		}
		return fetchers[i].Key() < fetchers[j].Key()
	})
	if len(fetchers) < 2 {
		return fetchers
	}

	roundRobinMux.Lock()
	start := roundRobinStart % len(fetchers)
	roundRobinStart = (roundRobinStart + 1) % len(fetchers)
	roundRobinMux.Unlock()

	rotated := make([]types.IPFetcher, 0, len(fetchers))
	rotated = append(rotated, fetchers[start:]...)
	rotated = append(rotated, fetchers[:start]...)
	return rotated
}

// Each query owns its custom endpoint, including while an older run is stopping.
func configuredFetcher(fetcher types.IPFetcher, endpoint string) types.IPFetcher {
	if _, ok := fetcher.(*fetchers.CustomIPFetcher); ok {
		custom := &fetchers.CustomIPFetcher{}
		custom.SetEndpoint(endpoint)
		return custom
	}
	return fetcher
}

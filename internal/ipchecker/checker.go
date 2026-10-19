package ipchecker

import (
	"context"
	"errors"
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
	ipFetchers      = make(map[string]types.IPFetcher)
	errorCounts     = make(map[string]int)
	fetchersMux     sync.RWMutex
	initialized     bool
	initializedMux  sync.Once
	maxErrorCount   = 10
	roundRobinMux   sync.Mutex
	roundRobinStart int
)

const (
	perFetcherTimeout = 5 * time.Second
	lookupTimeout     = 10 * time.Second
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
}

// GetExternalIP 获取外部IP信息
func GetExternalIP(proxy *detector.ProxyConfig, apiUse, apiBan []string, customAPIURL string) (string, error, string) {
	initFetchers()

	// 设置自定义 API URL
	setCustomAPIEndpoint(customAPIURL)

	client := &http.Client{
		Timeout: perFetcherTimeout,
	}

	if proxy.Enabled && proxy.Type == "port" {
		proxyURL, err := url.Parse("http://" + proxy.Address + ":" + proxy.Port)
		if err == nil {
			client.Transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
		}
	}

	fetchers := getRoundRobinFetchers(apiUse, apiBan)
	if len(fetchers) == 0 {
		return "", nil, ""
	}

	logging.Debugf("GetExternalIP: 可用查询器数量: %d", len(fetchers))
	for i, f := range fetchers {
		logging.Debugf("  %d. %s (%s)", i+1, f.Name(), f.Key())
	}

	var lastErr error
	var lastFetcherName string
	lookupCtx, cancelLookup := context.WithTimeout(context.Background(), lookupTimeout)
	defer cancelLookup()
	for _, fetcher := range fetchers {
		if err := lookupCtx.Err(); err != nil {
			lastErr = err
			break
		}

		fetcherName := fetcher.Name()
		fetcherCtx, cancelFetcher := context.WithTimeout(lookupCtx, perFetcherTimeout)
		ip, err := fetcher.Fetch(fetcherCtx, client)
		cancelFetcher()

		if err != nil {
			logging.Debugf("查询器 %s 结果: error=%v", fetcherName, err)
		} else if ip != "" {
			logging.Debugf("查询器 %s 结果: IP=%s", fetcherName, ip)
		} else {
			logging.Debugf("查询器 %s 结果: IP为空", fetcherName)
		}

		if err == nil && ip != "" {
			address, parseErr := netip.ParseAddr(ip)
			if parseErr != nil {
				increaseErrorCount(fetcherName)
				lastErr = parseErr
				lastFetcherName = fetcherName
				continue
			}
			resetErrorCount(fetcherName)
			return address.Unmap().String(), nil, fetcherName
		}
		if err == nil {
			err = errors.New("API returned an empty IP address")
		}
		increaseErrorCount(fetcherName)
		lastErr = err
		lastFetcherName = fetcherName
		if lookupCtx.Err() != nil {
			lastErr = lookupCtx.Err()
			break
		}
	}

	return "", lastErr, lastFetcherName
}

// increaseErrorCount 增加错误计数
func increaseErrorCount(fetcherName string) {
	fetchersMux.Lock()
	defer fetchersMux.Unlock()

	errorCounts[fetcherName]++

	if errorCounts[fetcherName] > maxErrorCount {
		delete(ipFetchers, fetcherName)
		delete(errorCounts, fetcherName)
	}
}

// resetErrorCount 重置错误计数
func resetErrorCount(fetcherName string) {
	fetchersMux.Lock()
	defer fetchersMux.Unlock()

	errorCounts[fetcherName] = 0
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

// setCustomAPIEndpoint 设置自定义 API 端点
func setCustomAPIEndpoint(url string) {
	fetchersMux.Lock()
	defer fetchersMux.Unlock()

	if fetcher, ok := ipFetchers["custom-ipapi"]; ok {
		if customFetcher, ok := fetcher.(*fetchers.CustomIPFetcher); ok {
			customFetcher.SetEndpoint(url)
		}
	}
}

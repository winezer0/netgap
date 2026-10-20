package detector

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// ProxyConfig 代理配置结构体
type ProxyConfig struct {
	Type        string // "port"
	Address     string // 代理地址
	Port        string // 代理端口
	Enabled     bool   // 代理是否启用
	Scheme      string
	credentials *url.Userinfo
	rules       *proxyRules
	configErr   error
}

const (
	sourceSystem = "system"
	sourceEnv    = "env"
	sourceNone   = "none"
)

// SystemReadTimeout is the budget for a complete proxy configuration read.
// Child commands share it; synchronous OS calls can only check it on return.
const SystemReadTimeout = 3 * time.Second

func DetectProxyWithSource(ctx context.Context) (*ProxyConfig, string, error) {
	ctx, cancel := context.WithTimeout(ctx, SystemReadTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, sourceSystem, err
	}
	proxy := getProxyFromSystem(ctx)
	if err := ctx.Err(); err != nil {
		return nil, sourceSystem, err
	}
	if proxy != nil && proxy.configErr != nil {
		return nil, sourceSystem, fmt.Errorf("%w: %w", ErrProxyRules, proxy.configErr)
	}
	if proxy != nil && proxy.Enabled {
		return proxy, sourceSystem, nil
	}

	proxyEnv := getProxyFromEnv()
	if proxyEnv != "" {
		// Validate only the endpoint selected for the eventual request URL.
		return &ProxyConfig{Enabled: true, Type: "rules", rules: &proxyRules{environment: true}}, sourceEnv, nil
	}

	// 无代理
	return &ProxyConfig{
		Enabled: false,
	}, sourceNone, nil
}

// getProxyFromEnv 从环境变量获取代理设置
func getProxyFromEnv() string {
	proxyVars := []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"}
	for _, varName := range proxyVars {
		if proxy := os.Getenv(varName); proxy != "" {
			return proxy
		}
		// 检查小写版本
		if proxy := os.Getenv(strings.ToLower(varName)); proxy != "" {
			return proxy
		}
	}
	return ""
}

package detector

import (
	"os"
	"strings"
)

// ProxyConfig 代理配置结构体
type ProxyConfig struct {
	Type    string // "port"
	Address string // 代理地址
	Port    string // 代理端口
	Enabled bool   // 代理是否启用
}

const (
	sourceSystem = "system"
	sourceEnv    = "env"
	sourceNone   = "none"
)

func DetectProxyWithSource() (*ProxyConfig, string, error) {
	proxy := getProxyFromSystem()
	if proxy != nil && proxy.Enabled {
		return proxy, sourceSystem, nil
	}

	proxyEnv := getProxyFromEnv()
	if proxyEnv != "" {
		if parsed := parseProxyURL(proxyEnv); parsed != nil {
			return parsed, sourceEnv, nil
		}
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

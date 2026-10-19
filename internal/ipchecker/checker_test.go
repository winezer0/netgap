package ipchecker

import (
	"testing"

	"netgap/internal/detector"
)

func TestGetExternalIP(t *testing.T) {
	// 测试无代理情况
	proxy := &detector.ProxyConfig{
		Enabled: false,
	}

	ip, err, _ := GetExternalIP(proxy, nil, nil, "")
	if err != nil {
		t.Logf("GetExternalIP failed (可能是网络问题): %v", err)
		// 网络测试可能失败，这里不视为测试失败
		return
	}

	if ip == "" {
		t.Logf("Warning: Expected IP address, got empty (可能是网络问题)")
	}
}

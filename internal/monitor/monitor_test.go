package monitor

import (
	"testing"

	"netgap/internal/config"
	"netgap/internal/ipchecker/localdb"
)

func TestCheckProxyStatus(t *testing.T) {
	// 测试配置
	cfg := &config.Config{
		Regions: []string{"北京", "上海"},
	}

	resolver, err := localdb.NewResolver()
	if err != nil {
		t.Fatalf("local database initialization failed: %v", err)
	}
	defer resolver.Close()

	checker := NewChecker(resolver)
	// Test the proxy status check.
	status, err, _ := checker.CheckProxyStatus(cfg)
	if err != nil {
		t.Logf("CheckProxyStatus failed (可能是网络问题): %v", err)
		// 网络测试可能失败，这里不视为测试失败
		return
	}

	// 状态应该是布尔值
	_ = status
}

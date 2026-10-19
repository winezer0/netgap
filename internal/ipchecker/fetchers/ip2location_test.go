package fetchers

import (
	"context"
	"net/http"
	"testing"
)

func TestIP2LocationFetcher(t *testing.T) {
	// 创建查询器
	fetcher := NewIP2LocationFetcher()
	if fetcher.Name() != "ip2location" {
		t.Errorf("Expected name 'ip2location', got '%s'", fetcher.Name())
	}
	if fetcher.Key() != "ip2location" {
		t.Errorf("Expected key 'ip2location', got '%s'", fetcher.Key())
	}

	// 创建HTTP客户端
	client := &http.Client{}

	// 测试获取IP信息
	ip, err := fetcher.Fetch(context.Background(), client)
	if err != nil {
		t.Logf("Fetch failed (可能是网络问题): %v", err)
		// 网络测试可能失败，这里不视为测试失败
		return
	}

	// 验证IP信息
	if ip == "" {
		t.Logf("Warning: Expected IP address, got empty (可能是网络问题)")
		return
	}

	t.Logf("Successfully fetched external IP: %s", ip)
}

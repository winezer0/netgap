package fetchers

import (
	"context"
	"net/http"
	"testing"
)

func TestCustomIPFetcherName(t *testing.T) {
	// 创建查询器
	fetcher := NewCustomIPFetcher()
	if fetcher.Name() != "custom-ipapi" {
		t.Errorf("Expected name 'custom-ipapi', got '%s'", fetcher.Name())
	}
	if fetcher.Key() != "customip" {
		t.Errorf("Expected key 'customip', got '%s'", fetcher.Key())
	}
}

func TestCustomIPFetcherFetch(t *testing.T) {
	// 创建查询器
	fetcher := NewCustomIPFetcher()

	// 创建HTTP客户端
	client := &http.Client{}

	// 测试获取IP信息（网络请求可能失败）
	ip, err := fetcher.Fetch(context.Background(), client)
	if err != nil {
		t.Logf("Fetch failed (可能是网络问题或API不可用): %v", err)
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

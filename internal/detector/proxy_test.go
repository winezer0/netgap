package detector

import (
	"context"
	"os"
	"testing"
)

func TestDetectProxy(t *testing.T) {
	// 测试无代理情况
	os.Unsetenv("HTTP_PROXY")
	os.Unsetenv("HTTPS_PROXY")
	os.Unsetenv("ALL_PROXY")

	proxy, source, err := DetectProxyWithSource(context.Background())
	if err != nil {
		t.Errorf("DetectProxy failed: %v", err)
	}
	if proxy == nil {
		t.Fatalf("expected proxy not nil")
	}
	if source != sourceSystem && source != sourceEnv && source != sourceNone {
		t.Fatalf("unexpected source: %s", source)
	}
	if !proxy.Enabled && source != sourceNone {
		t.Fatalf("expected source none when proxy disabled, got=%s", source)
	}
}

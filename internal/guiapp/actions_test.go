package guiapp

import (
	"testing"

	"netgap/internal/guiapp/logbuffer"
	"netgap/internal/iproute"
)

func TestClearLogs(t *testing.T) {
	t.Parallel()

	service := &Service{
		logs: logbuffer.New(10),
	}
	service.logs.Add("INFO", "log1")
	service.logs.Add("INFO", "log2")
	if len(service.logs.List()) != 2 {
		t.Fatalf("expected 2 logs")
	}

	service.ClearLogs()
	if len(service.logs.List()) != 0 {
		t.Fatalf("expected logs cleared")
	}
}

func TestGetDefaultRouteInfoConsistency(t *testing.T) {
	t.Parallel()

	route1, err1 := iproute.GetDefaultRoute()
	route2, err2 := (&Service{}).GetDefaultRouteInfo()
	if (err1 != nil) != (err2 != nil) {
		t.Fatalf("unexpected error consistency: err1=%v err2=%v", err1, err2)
	}
	if err1 == nil {
		if route1 == nil || route2 == nil {
			t.Fatalf("expected route not nil")
		}
		if route2.Gateway == "" {
			t.Fatalf("expected gateway not empty")
		}
	}
}

func TestGetProxyConfigInfo(t *testing.T) {
	t.Parallel()

	info, err := (&Service{}).GetProxyConfigInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info == nil {
		t.Fatalf("expected info not nil")
	}
	if info.Source == "" {
		t.Fatalf("expected source not empty")
	}
}

func TestFormatProxyConfigLog(t *testing.T) {
	t.Parallel()

	got := formatProxyConfigLog(nil)
	if got == "" {
		t.Fatalf("expected non-empty")
	}

	got = formatProxyConfigLog(&ProxyConfigInfo{Enabled: false, Source: "env"})
	if got != "Proxy config: enabled=false source=env" {
		t.Fatalf("unexpected: %s", got)
	}

	got = formatProxyConfigLog(&ProxyConfigInfo{Enabled: true, Source: "system", Address: "127.0.0.1", Port: "7890"})
	if got != "Proxy config: enabled=true source=system address=127.0.0.1 port=7890" {
		t.Fatalf("unexpected: %s", got)
	}
}

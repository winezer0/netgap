package monitor

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"netgap/internal/config"
	"netgap/internal/detector"
	"netgap/internal/ipchecker"
	"netgap/internal/ipchecker/types"
)

type diagnosticsResolver struct{}

func (diagnosticsResolver) Resolve(ip string) (*types.IPInfo, error) {
	return &types.IPInfo{IP: ip, Country: "Peru", Region: "Lima Province", City: "Miraflores", Isp: "Example ISP"}, nil
}

func TestDiagnosticsPreserveRawRegionMatching(t *testing.T) {
	for _, tc := range []struct {
		name     string
		keywords []string
		field    string
		keyword  string
	}{
		{name: "province substring", keywords: []string{"Province"}, field: "Province", keyword: "Province"},
		{name: "city substring", keywords: []string{"flores"}, field: "City", keyword: "flores"},
		{name: "first configured keyword wins", keywords: []string{"flores", "Province"}, field: "City", keyword: "flores"},
		{name: "country excluded", keywords: []string{"Peru"}},
		{name: "ISP excluded", keywords: []string{"Example ISP"}},
		{name: "display labels excluded", keywords: []string{"City:", "Country:", "Unknown"}},
		{name: "case sensitivity preserved", keywords: []string{"lima"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checker := &Checker{
				resolver: diagnosticsResolver{},
				detectProxy: func(context.Context) (*detector.ProxyConfig, string, error) {
					return nil, "", nil
				},
				lookupIP: func(context.Context, *detector.ProxyConfig, []string, []string, string) (string, error, string) {
					return "185.47.253.235", nil, "mock"
				},
			}
			result, err := checker.CheckProxyStatusResultContext(context.Background(), &config.Config{Regions: tc.keywords})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsLocal != (tc.field != "") || result.MatchedField != tc.field || result.MatchedKeyword != tc.keyword {
				t.Fatalf("unexpected match: matched=%v field=%q keyword=%q", result.IsLocal, result.MatchedField, result.MatchedKeyword)
			}
		})
	}
}

func TestCheckFailureLogDistinguishesInvalidationFromFailures(t *testing.T) {
	for _, tc := range []struct {
		err   error
		level string
		state string
	}{
		{fmt.Errorf("wrapped: %w", detector.ErrStateChanged), "WARN", "network_changed"},
		{context.Canceled, "INFO", "canceled"},
		{context.DeadlineExceeded, "ERROR", "timeout"},
		{ipchecker.ErrAPIResponse, "ERROR", "api_response"},
		{ErrLocalResolution, "ERROR", "local_resolution"},
	} {
		level, message := CheckFailureLog("mock", tc.err)
		if level != tc.level || !strings.Contains(message, "[state="+tc.state+"]") || !strings.Contains(message, "ipSource[mock]") {
			t.Fatalf("unexpected diagnostic: %s %s", level, message)
		}
		if tc.state == "network_changed" && !strings.Contains(message, "waiting for the next scheduled check") {
			t.Fatalf("missing retry explanation: %s", message)
		}
	}
}

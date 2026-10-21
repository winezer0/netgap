//go:build windows

package iproute

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func TestRouteLifetimeProviderPrecision(t *testing.T) {
	now := time.Date(2026, 10, 21, 13, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, field, ticks string
		elapsed            time.Duration
		seconds            uint64
		wantError          bool
	}{
		{"reported-finite-lifetime", "ValidLifetime", "576000000000", 19*time.Second + 938397*time.Microsecond, 57580, false},
		{"preferred-finite", "PreferredLifetime", "576000000000", 19*time.Second + 938397*time.Microsecond, 57580, false},
		{"less-than-one-second-valid", "ValidLifetime", "10000000", 100 * time.Millisecond, 0, true},
		{"less-than-one-second-preferred", "PreferredLifetime", "10000000", 100 * time.Millisecond, 0, false},
		{"expired", "ValidLifetime", "10000000", 2 * time.Second, 0, true},
		{"infinite", "ValidLifetime", "42949672950000000", time.Hour, 0xffffffff, false},
		{"future-capture", "ValidLifetime", "100000000", -time.Second, 10, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := routeLifetimeParamAt(tc.field, tc.ticks, now.Add(-tc.elapsed), now)
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected error: %v", err)
			}
			if err != nil {
				return
			}
			parts := (*[9]uint32)(unsafe.Pointer(&p.value))
			seconds := uint64(parts[1])*86400 + uint64(parts[2])*3600 + uint64(parts[3])*60 + uint64(parts[4])
			if parts[0] != 0 || parts[5] != 0 || seconds != tc.seconds {
				t.Fatalf("invalid provider interval: %v; want %d whole seconds", parts, tc.seconds)
			}
		})
	}
}

// Cross-check actual generated inputs against this Windows installation's
// system cmdlet contract. No Create/Remove/Invoke operation is performed.
func TestWindowsCreateInputsMatchSystemIndexParameterSet(t *testing.T) {
	file := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "Modules", "NetTCPIP", "MSFT_NetRoute.cdxml")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{}
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	selected := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			if element.Name.Local == "Method" {
				for _, attr := range element.Attr {
					if attr.Name.Local == "CmdletParameterSet" {
						selected = attr.Value == "ByInterfaceIndex"
					}
				}
			}
			if selected && element.Name.Local == "Parameter" {
				for _, attr := range element.Attr {
					if attr.Name.Local == "ParameterName" {
						allowed[attr.Value] = true
					}
				}
			}
		case xml.EndElement:
			if element.Name.Local == "Method" {
				selected = false
			}
		}
	}
	if !allowed["InterfaceIndex"] || allowed["InterfaceAlias"] {
		t.Fatal("unexpected system index parameter set", allowed)
	}
	c, err := openMI()
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()
	for _, store := range []string{"ActiveStore", "PersistentStore"} {
		row := RouteEntry{InterfaceIndex: 5, InterfaceAlias: "WLAN", CompartmentID: 1, Destination: "0.0.0.0/0", NextHop: "192.0.2.1", Metric: 25, Store: store, Protocol: "19", Publish: "0", ValidTicks: "42949672950000000", PreferredTicks: "42949672950000000"}
		params, err := windowsCreateParameters(row, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range params {
			if !allowed[p.name] {
				t.Fatalf("%s: input %s is not in the system parameter set", store, p.name)
			}
		}
		input, err := c.parameters(params)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range params {
			if _, err := miGet(input, p.name, p.typ); err != nil {
				t.Error(err)
			}
		}
		_, policyErr := miGetString(input, "PolicyStore")
		if (policyErr == nil) != (store == "ActiveStore") {
			t.Errorf("incorrect policy-store presence for %s: %v", store, policyErr)
		}
		miCall(input.ft[2], miPtr(input))
	}
}

// Construct and read method inputs locally. Never invoke a provider method.
func TestMIParametersRoundTrip(t *testing.T) {
	c, err := openMI()
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()
	lifetime, err := routeLifetimeParam("ValidLifetime", "42949672950000000", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	params := []miParameter{
		miStringParam("DestinationPrefix", "0.0.0.0/0"),
		miNumberParam("InterfaceIndex", miUint32, 5),
		miNumberParam("RouteMetric", miUint16, 25),
		miNumberParam("Publish", miUint8, 0),
		miNumberParam("PassThru", miBoolean, 0),
		lifetime,
	}
	input, err := c.parameters(params)
	if err != nil {
		t.Fatal(err)
	}
	defer miCall(input.ft[2], miPtr(input))
	for _, p := range params {
		got, err := miGet(input, p.name, p.typ)
		if err != nil {
			t.Fatal(err)
		}
		if p.typ != miString && got != p.value {
			t.Fatalf("%s value changed: %v != %v", p.name, got, p.value)
		}
	}
}

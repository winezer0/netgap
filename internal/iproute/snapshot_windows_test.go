//go:build windows

package iproute

import (
	"context"
	"testing"
	"time"
)

// This exercises enumeration and binding only; it never mutates OS routes.
func TestWindowsSnapshotReadOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	b := windowsRouteBackend{}
	rows, err := b.list(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.InterfaceGUID == "" || row.ValidTicks == "" || row.PreferredTicks == "" {
			t.Fatalf("incomplete route record: %+v", row)
		}
	}
	if err := b.validate(ctx, rows); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsMutationScriptsParseWithoutExecution(t *testing.T) {
	for _, script := range []string{routePSHelpers + routeRemovePS, routePSHelpers + routeRestorePS} {
		_, err := routePowerShell(context.Background(), `
$tokens=$null; $errors=$null
$null=[System.Management.Automation.Language.Parser]::ParseInput($data,[ref]$tokens,[ref]$errors)
if ($errors.Count -ne 0) { throw ($errors -join '; ') }
`, script)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestWindowsRestoreScriptWithMockedCmdlets(t *testing.T) {
	// All mutation helpers are local PowerShell functions here, never OS cmdlets.
	mocks := `
function Assert-Binding($r) {}
function Get-Matching($r) { return @($script:created | Where-Object { $null -ne $_ }) }
function New-NetRoute {
  param($DestinationPrefix,$NextHop,$InterfaceIndex,$RouteMetric,$Protocol,$Publish,$PolicyStore,$ValidLifetime,$PreferredLifetime)
  if ($DestinationPrefix -ne '0.0.0.0/0' -or $NextHop -ne '192.0.2.1' -or $InterfaceIndex -ne 4 -or $RouteMetric -ne 12 -or $PolicyStore -ne 'ActiveStore') { throw 'Lost route parameters' }
  if ($ValidLifetime -ne [TimeSpan]::MaxValue -or $PreferredLifetime -ne [TimeSpan]::MaxValue) { throw 'Lost infinite lifetimes' }
  $script:created=[pscustomobject]@{RouteMetric=$RouteMetric; Protocol=$Protocol; Publish=$Publish}
}
`
	row := fakeRoutes()[0]
	row.Protocol, row.Publish = "NetMgmt", "No"
	row.ValidTicks, row.PreferredTicks = "42949672950000000", "42949672950000000"
	_, err := routePowerShell(context.Background(), mocks+routeRestorePS, map[string]any{"route": row, "savedAt": time.Now().UTC().Format("2006-01-02T15:04:05.0000000Z07:00")})
	if err != nil {
		t.Fatal(err)
	}
}

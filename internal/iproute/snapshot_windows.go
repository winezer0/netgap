//go:build windows

package iproute

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

type windowsRouteBackend struct{}

func lockSnapshotFile(file string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(file + ".lock")
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("another process may be changing routes: %w", err)
	}
	return func() { windows.CloseHandle(handle) }, nil
}

func newRouteBackend() routeBackend { return windowsRouteBackend{} }

// Arguments are JSON/base64 data, never executable interpolation of addresses.
func routePowerShell(ctx context.Context, body string, data any) ([]byte, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	script := `$ErrorActionPreference='Stop'; $ProgressPreference='SilentlyContinue'; [Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); $data=ConvertFrom-Json ([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('` + base64.StdEncoding.EncodeToString(payload) + `'))); ` + routePSHelpers + body
	chars := utf16.Encode([]rune(script))
	encoded := make([]byte, len(chars)*2)
	for i, c := range chars {
		binary.LittleEndian.PutUint16(encoded[i*2:], c)
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("Windows route operation: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

const routePSHelpers = `
function Get-Binding($index) {
  $adapter=Get-NetAdapter -IncludeHidden | Where-Object InterfaceIndex -eq $index | Select-Object -First 1
  if ($null -eq $adapter) { throw "Interface $index no longer exists" }
  $addresses=@(Get-NetIPAddress -InterfaceIndex $index | ForEach-Object { "$($_.IPAddress)/$($_.PrefixLength)" } | Sort-Object)
  $network=@(Get-NetConnectionProfile | Where-Object InterfaceIndex -eq $index | ForEach-Object { "$($_.InstanceID)/$($_.Name)" } | Sort-Object) -join '|'
  return @{guid=$adapter.InterfaceGuid.ToString(); addresses=$addresses; network=$network}
}
function Assert-Binding($r) {
  $binding=Get-Binding $r.interfaceIndex
  if ($binding.guid -ne $r.interfaceGuid -or $binding.network -ne $r.network -or (($binding.addresses -join '|') -ne (@($r.addresses) -join '|'))) { throw "Interface/network identity changed on interface $($r.interfaceIndex)" }
}
function Get-Defaults($store) {
  return @(Get-NetRoute -PolicyStore $store | Where-Object { $_.DestinationPrefix -eq '0.0.0.0/0' -or $_.DestinationPrefix -eq '::/0' })
}
function Get-Matching($r) {
  return @(Get-Defaults $r.store | Where-Object { $_.InterfaceIndex -eq $r.interfaceIndex -and $_.DestinationPrefix -eq $r.destination -and $_.NextHop -eq $r.nextHop })
}
`

func (windowsRouteBackend) list(ctx context.Context) ([]RouteEntry, error) {
	out, err := routePowerShell(ctx, `
$rows=@(foreach($store in @('ActiveStore','PersistentStore')) {
  foreach($r in (Get-Defaults $store)) {
    $b=Get-Binding $r.InterfaceIndex
    [pscustomobject]@{destination=$r.DestinationPrefix; nextHop=$r.NextHop; interfaceIndex=$r.InterfaceIndex; interfaceGuid=$b.guid; addresses=@($b.addresses); network=$b.network; metric=$r.RouteMetric; store=$store; protocol=$r.Protocol.ToString(); publish=$r.Publish.ToString(); validTicks=$r.ValidLifetime.Ticks.ToString(); preferredTicks=$r.PreferredLifetime.Ticks.ToString()}
  }
})
ConvertTo-Json -InputObject @($rows) -Depth 6 -Compress
`, nil)
	if err != nil {
		return nil, err
	}
	var rows []RouteEntry
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("parse Windows route snapshot: %w", err)
	}
	return rows, nil
}

func (windowsRouteBackend) validate(ctx context.Context, rows []RouteEntry) error {
	_, err := routePowerShell(ctx, `foreach($r in @($data)) { Assert-Binding $r }`, rows)
	return err
}

func (windowsRouteBackend) remove(ctx context.Context, row RouteEntry) error {
	_, err := routePowerShell(ctx, routeRemovePS, row)
	return err
}

const routeRemovePS = `
$r=$data; Assert-Binding $r
$matches=Get-Matching $r
foreach($item in $matches) {
  if ($item.RouteMetric -ne $r.metric -or $item.Protocol.ToString() -ne $r.protocol) { throw 'Route changed before removal' }
  $item | Remove-NetRoute -Confirm:$false
}
`

func (windowsRouteBackend) restore(ctx context.Context, row RouteEntry, saved time.Time) error {
	_, err := routePowerShell(ctx, routeRestorePS, map[string]any{"route": row, "savedAt": saved.UTC().Format("2006-01-02T15:04:05.0000000Z07:00")})
	return err
}

const routeRestorePS = `
$r=$data.route; Assert-Binding $r
$matches=Get-Matching $r
if ($matches.Count -gt 0) {
  foreach($item in $matches) { if ($item.RouteMetric -ne $r.metric -or $item.Protocol.ToString() -ne $r.protocol -or $item.Publish.ToString() -ne $r.publish) { throw 'Existing route conflicts with recovery snapshot' } }
  return
}
$elapsed=([DateTime]::UtcNow-([DateTime]::Parse($data.savedAt).ToUniversalTime())).Ticks
function Remaining($ticks) {
  $value=[Int64]::Parse($ticks)
  if ($value -ge [TimeSpan]::MaxValue.Ticks -or $value -ge 42949672950000000) { return [TimeSpan]::MaxValue }
  return [TimeSpan]::FromTicks([Math]::Max([Int64]0, $value-$elapsed))
}
$valid=Remaining $r.validTicks; $preferred=Remaining $r.preferredTicks
if ($valid.Ticks -le 0) { throw 'Saved dynamic route lifetime has expired; reconnect the network to reacquire it' }
New-NetRoute -DestinationPrefix $r.destination -NextHop $r.nextHop -InterfaceIndex $r.interfaceIndex -RouteMetric $r.metric -Protocol $r.protocol -Publish $r.publish -PolicyStore $r.store -ValidLifetime $valid -PreferredLifetime $preferred | Out-Null
$verified=Get-Matching $r
if ($verified.Count -eq 0) { throw 'Route restoration was not confirmed' }
foreach($item in $verified) { if ($item.RouteMetric -ne $r.metric -or $item.Protocol.ToString() -ne $r.protocol -or $item.Publish.ToString() -ne $r.publish) { throw 'Restored route attributes differ from recovery snapshot' } }
Assert-Binding $r
`

//go:build windows

package iproute

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
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

// Data travels on stdin; command length does not depend on route count.
func routePowerShell(ctx context.Context, body string, data any) ([]byte, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	script := `$ErrorActionPreference='Stop'; $ProgressPreference='SilentlyContinue'; [Console]::InputEncoding=[Text.UTF8Encoding]::new($false); [Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); $data=ConvertFrom-Json ([Console]::In.ReadToEnd()); ` + routePSHelpers + body
	chars := utf16.Encode([]rune(script))
	encoded := make([]byte, len(chars)*2)
	for i, c := range chars {
		binary.LittleEndian.PutUint16(encoded[i*2:], c)
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
	cmd.Stdin = strings.NewReader(string(payload))
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
  $adapter=@(Get-NetAdapter -IncludeHidden | Where-Object { $_.InterfaceGuid.ToString() -eq $r.interfaceGuid })
  if ($adapter.Count -ne 1) { throw 'Saved adapter no longer exists uniquely' }
  $r.interfaceIndex=$adapter[0].InterfaceIndex
  $binding=Get-Binding $r.interfaceIndex
  if ($binding.guid -ne $r.interfaceGuid -or $binding.network -ne $r.network) { throw "Interface/network identity changed on interface $($r.interfaceIndex)" }
  $saved=@(Get-RouteNetworks $r.addresses $r.destination $r.nextHop)
  $current=@(Get-RouteNetworks $binding.addresses $r.destination $r.nextHop)
  if ($saved.Count -eq 0 -or @($saved | Where-Object { $current -contains $_ }).Count -eq 0) { throw 'Relevant route subnet changed' }
}
function Get-RouteNetworks($addresses,$destination,$nextHop) {
  $v6=$destination -eq '::/0'
  $hop=[Net.IPAddress]::Parse($nextHop).GetAddressBytes()
  $all=@(); $onLink=@()
  foreach($address in @($addresses)) {
    $parts=$address -split '/'; $ip=[Net.IPAddress]::Parse($parts[0]); $bytes=$ip.GetAddressBytes()
    if (($bytes.Length -eq 16) -ne $v6) { continue }
    $bits=[int]$parts[1]; $network=New-Object byte[] $bytes.Length; $matches=$hop.Length -eq $bytes.Length
    for($i=0;$i -lt $bytes.Length;$i++) {
      $count=[Math]::Min(8,[Math]::Max(0,$bits-8*$i)); $mask=(256-[Math]::Pow(2,8-$count))
      $network[$i]=$bytes[$i] -band [int]$mask
      if ($matches -and (($hop[$i] -band [int]$mask) -ne $network[$i])) { $matches=$false }
    }
    $key=([Net.IPAddress]::new($network)).ToString()+'/'+$bits; $all+=$key
    if ($matches) { $onLink+=$key }
  }
  if ($onLink.Count -gt 0) { return @($onLink | Sort-Object -Unique) }
  return @($all | Sort-Object -Unique)
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

// Journal ActiveStore side effects before persistent creation. This survives a
// crash between restoring the persistent entry and reconciling its active copy.
func (b windowsRouteBackend) prepareRestore(ctx context.Context, snapshot *routeSnapshot) error {
	current, err := b.list(ctx)
	if err != nil {
		return err
	}
	for i := 0; i < len(snapshot.Routes); i++ {
		persistent := snapshot.Routes[i]
		if persistent.Store != "PersistentStore" {
			continue
		}
		exists := false
		for _, row := range current {
			if sameRouteIdentity(row, persistent) {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		active := persistent
		active.Store = "ActiveStore"
		active.CreatedByRecovery, active.ActiveBefore = nil, nil
		var before *RouteEntry
		for _, row := range current {
			if sameRouteIdentity(row, active) {
				copyRow := row
				before = &copyRow
				break
			}
		}
		index := -1
		for j, row := range snapshot.Routes {
			if sameRouteIdentity(row, active) {
				index = j
				break
			}
		}
		if before != nil && (index < 0 || (!sameRoute(*before, snapshot.Routes[index]) && !sameRoute(*before, active))) {
			return errors.New("active route conflicts with persistent recovery; no route overwritten")
		}
		snapshot.Routes[i].ActiveBefore = before
		if index >= 0 {
			if snapshot.Routes[index].CreatedByRecovery == nil {
				snapshot.Routes[index].CreatedByRecovery = &active
			}
		} else {
			cleanup := active
			cleanup.CreatedByRecovery = &active
			cleanup.RemoveAfterRestore = true
			snapshot.Routes = append(snapshot.Routes, cleanup)
		}
	}
	return nil
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
function Same-Attributes($item,$expected) {
  return $item.RouteMetric -eq $expected.metric -and $item.Protocol.ToString() -eq $expected.protocol -and $item.Publish.ToString() -eq $expected.publish
}
if ($r.removeAfterRestore) {
  foreach($item in $matches) {
    if (-not (Same-Attributes $item $r.createdByRecovery)) { throw 'Generated active route changed; refusing cleanup' }
    $item | Remove-NetRoute -Confirm:$false
  }
  if (@(Get-Matching $r).Count -ne 0) { throw 'Generated active route cleanup not confirmed' }
  return
}
if ($matches.Count -gt 0) {
  $allMatch=$true
  foreach($item in $matches) { if (-not (Same-Attributes $item $r)) { $allMatch=$false } }
  if ($allMatch) { return }
  if ($r.store -ne 'ActiveStore' -or $null -eq $r.createdByRecovery) { throw 'Existing route conflicts with recovery snapshot' }
  foreach($item in $matches) { if (-not (Same-Attributes $item $r.createdByRecovery)) { throw 'Generated active route changed; refusing replacement' } }
  foreach($item in $matches) { $item | Remove-NetRoute -Confirm:$false }
}
$elapsed=([DateTime]::UtcNow-([DateTime]::Parse($data.savedAt).ToUniversalTime())).Ticks
function Remaining($ticks) {
  $value=[Int64]::Parse($ticks)
  if ($value -ge [TimeSpan]::MaxValue.Ticks -or $value -ge 42949672950000000) { return [TimeSpan]::MaxValue }
  return [TimeSpan]::FromTicks([Math]::Max([Int64]0, $value-$elapsed))
}
$valid=Remaining $r.validTicks; $preferred=Remaining $r.preferredTicks
if ($valid.Ticks -le 0) { throw 'Saved dynamic route lifetime has expired; reconnect the network to reacquire it' }
$parameters=@{DestinationPrefix=$r.destination; NextHop=$r.nextHop; InterfaceIndex=$r.interfaceIndex; RouteMetric=$r.metric; Protocol=$r.protocol; Publish=$r.publish; ValidLifetime=$valid; PreferredLifetime=$preferred}
if ($r.store -eq 'ActiveStore') { $parameters.PolicyStore='ActiveStore' }
elseif ($r.store -ne 'PersistentStore') { throw 'Unsupported route store' }
if ($r.store -eq 'PersistentStore') {
  $copy=$r.PSObject.Copy(); $copy.store='ActiveStore'
  $active=@(Get-Matching $copy)
  foreach($item in $active) {
    if (-not (Same-Attributes $item $r) -and ($null -eq $r.activeBefore -or -not (Same-Attributes $item $r.activeBefore))) { throw 'Active route changed before persistent creation' }
  }
}
# Omitting PolicyStore creates persistent and active entries. Never pass
# PersistentStore to New-NetRoute, which does not support that value.
try { New-NetRoute @parameters | Out-Null }
catch {
  # Creation can report an active duplicate after writing persistent state.
  # Accept only the exact desired entry observed below; otherwise keep backup.
  if (@(Get-Matching $r).Count -eq 0) { throw }
}
$verified=Get-Matching $r
if ($verified.Count -eq 0) { throw 'Route restoration was not confirmed' }
foreach($item in $verified) { if ($item.RouteMetric -ne $r.metric -or $item.Protocol.ToString() -ne $r.protocol -or $item.Publish.ToString() -ne $r.publish) { throw 'Restored route attributes differ from recovery snapshot' } }
Assert-Binding $r
`

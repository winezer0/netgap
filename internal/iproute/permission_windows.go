//go:build windows

package iproute

import "golang.org/x/sys/windows"

// Query the actual process token. net session adds process/service latency and
// can fail on an elevated process when the Server service is unavailable.
func checkWindowsAdminPermission() bool { return windows.GetCurrentProcessToken().IsElevated() }

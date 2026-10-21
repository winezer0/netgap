//go:build windows

package iproute

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

func getNativeDefaultRoute() (*RouteInfo, error) { return getWindowsDefaultRoute() }
func addNativeDefaultRoute(gateway string) error { return addWindowsDefaultRoute(gateway) }
func nativeProtectionSupported() error {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return errors.New("Windows native route protection requires amd64 or arm64")
	}
	return windows.NewLazySystemDLL("mi.dll").NewProc("MI_Application_InitializeV1").Find()
}

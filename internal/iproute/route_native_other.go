//go:build !darwin && !windows

package iproute

func getNativeDefaultRoute() (*RouteInfo, error) { return getLinuxDefaultRoute() }
func addNativeDefaultRoute(gateway string) error { return addLinuxDefaultRoute(gateway) }
func nativeProtectionSupported() error           { return nil }

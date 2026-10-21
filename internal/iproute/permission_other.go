//go:build !windows

package iproute

func checkWindowsAdminPermission() bool { return false }

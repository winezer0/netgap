//go:build linux

package detector

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
)

// Existing source/identity checks retained after removal of GIO-specific tests.
func TestLinuxProxySourceBoundaries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	for _, key := range []string{"XDG_CURRENT_DESKTOP", "DISPLAY", "WAYLAND_DISPLAY", "DBUS_SESSION_BUS_ADDRESS", "DESKTOP_SESSION"} {
		t.Setenv(key, "")
	}
	if p := getProxyFromSystem(context.Background()); p != nil {
		t.Fatal("headless process read desktop config")
	}
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	if p := getProxyFromSystem(context.Background()); p == nil || p.configErr == nil {
		t.Fatal("missing GNOME dependency became DIRECT")
	}
	t.Setenv("XDG_CURRENT_DESKTOP", "unknown")
	if p := getProxyFromSystem(context.Background()); p == nil || p.configErr == nil {
		t.Fatal("unknown desktop became DIRECT")
	}
}

func TestLinuxDesktopSessionIdentityMismatch(t *testing.T) {
	t.Setenv("SUDO_UID", "999999999999")
	cmd := exec.Command("unused")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := linuxDesktopCommand(cmd); err == nil {
		t.Fatal("invalid original session accepted")
	}
}

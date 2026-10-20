//go:build linux

package detector

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestGIORoutes(t *testing.T) {
	paths, err := gioRoutes([]string{"http://127.0.0.1:1", "socks://127.0.0.1:2", "direct://"})
	if err != nil || len(paths) != 5 || paths[4].URL != nil || paths[1].URL.Scheme != "socks5" {
		t.Fatal(paths, err)
	}
	for _, values := range [][]string{nil, {"direct://bad"}, {"use-proxy:"}, {"http://host:0"}, {"http://host:80/secret"}, {"file:///tmp/pac"}, {"http://host:80", "bad"}} {
		if _, err := gioRoutes(values); err == nil {
			t.Fatal("invalid result accepted", values)
		}
	}
	paths, err = gioRoutes([]string{"http://127.0.0.1:1"})
	if err != nil || len(paths) != 1 || paths[0].URL == nil {
		t.Fatal("implicit DIRECT", paths, err)
	}
}

func TestGIOHelperProtocolFailureAndCancellation(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "helper")
	for _, output := range []string{"broken", `{"version":1,"error":"missing PAC service"}`, `{"version":1,"backend":"dummy","state":"bad"}`} {
		script := "#!/bin/sh\n/bin/cat <<'END'\n" + output + "\nEND\n"
		if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := callGIOHelper(context.Background(), helper, "--snapshot"); err == nil {
			t.Fatal("invalid helper succeeded")
		}
	}
	output := `{"version":1,"backend":"GProxyResolverGnome","configured":true,"state":"` + strings.Repeat("a", 64) + `","routes":["direct://"]}`
	if err := os.WriteFile(helper, []byte("#!/bin/sh\n/bin/cat <<'END'\n"+output+"\nEND\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if reply, err := callGIOHelper(context.Background(), helper, "--snapshot"); err != nil || !reply.Configured {
		t.Fatal(reply, err)
	}
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nexec /bin/sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := callGIOHelper(ctx, helper, "--snapshot"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

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

func TestGIOHelperUnavailableAndOversizedOutput(t *testing.T) {
	t.Setenv("SUDO_UID", "")
	helper := filepath.Join(t.TempDir(), "helper")
	if reply, err := callGIOHelper(context.Background(), helper, "https://via.example/"); err == nil || reply != nil {
		t.Fatal("missing helper became a usable path", reply, err)
	}
	for name, body := range map[string]string{
		"not-executable": "#!/bin/sh\nexit 0\n",
		"nonzero":        "#!/bin/sh\nprintf 'private-error' >&2\nexit 2\n",
		"oversized":      "#!/bin/sh\nexec /usr/bin/head -c 70000 /dev/zero\n",
	} {
		t.Run(name, func(t *testing.T) {
			mode := os.FileMode(0700)
			if name == "not-executable" {
				mode = 0600
			}
			if err := os.WriteFile(helper, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(helper, mode); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			reply, err := callGIOHelper(ctx, helper, "https://via.example/")
			if err == nil || reply != nil || strings.Contains(err.Error(), "private-error") {
				t.Fatal("helper failure accepted or stderr exposed", reply, err)
			}
			if name == "oversized" && !errors.Is(err, errSettingsOutputLimit) {
				t.Fatal("output limit not preserved", err)
			}
		})
	}
}

// Enabled only by scripts/test-gio-helper.py inside its private desktop fixture.
func TestGIONativeSystemRouting(t *testing.T) {
	if os.Getenv("NETGAP_GIO_NATIVE_TEST") != "1" {
		t.Skip("requires isolated native GIO fixture")
	}
	p, source, err := DetectProxyWithSource(context.Background())
	if err != nil || source != sourceSystem || p == nil || !p.Enabled {
		t.Fatal(source, err)
	}
	paths := gnomePaths(t, p, "https://via.example/")
	if len(paths) < 2 || paths[0] != "http://127.0.0.1:18888" || paths[1] != "socks5://127.0.0.1:18890" {
		t.Fatal(paths)
	}
	paths = gnomePaths(t, p, "https://direct.example/")
	if len(paths) != 1 || paths[0] != "DIRECT" {
		t.Fatal(paths)
	}
	if os.Getenv("GSETTINGS_BACKEND") != "keyfile" || !strings.HasPrefix(filepath.Base(os.Getenv("XDG_CONFIG_HOME")), "netgap-gio-test-") {
		t.Fatal("requires temporary desktop fixture")
	}
	get := exec.Command("gsettings", "get", gnomeProxySchema, "ignore-hosts")
	original, err := get.Output()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := exec.Command("gsettings", "set", gnomeProxySchema, "ignore-hosts", strings.TrimSpace(string(original))).Run(); err != nil {
			t.Error(err)
		}
	}()
	if err := exec.Command("gsettings", "set", gnomeProxySchema, "ignore-hosts", "['changed.example']").Run(); err != nil {
		t.Fatal(err)
	}
	target, _ := url.Parse("https://via.example/")
	if _, err := p.Routes(context.Background(), target); !errors.Is(err, ErrStateChanged) {
		t.Fatal("stale GIO configuration accepted", err)
	}
}

func TestGIOSessionIdentityMismatch(t *testing.T) {
	t.Setenv("SUDO_UID", "999999999999")
	cmd := exec.Command("unused")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := gioSessionCommand(cmd); err == nil {
		t.Fatal("invalid original session accepted")
	}
}

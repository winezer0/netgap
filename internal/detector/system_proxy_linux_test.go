//go:build linux

package detector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func fakeGsettings(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "settings")
	if err := os.WriteFile(file, []byte(gnomeFixture(nil)), 0600); err != nil {
		t.Fatal(err)
	}
	helper := "#!/bin/sh\nset -eu\n" + script + "\n"
	if err := os.WriteFile(filepath.Join(dir, "gsettings"), []byte(helper), 0700); err != nil {
		t.Fatal(err)
	}
	// PATH and fixture locations affect this test process only. The helper
	// cannot call the actual gsettings, ip, route, nmcli or resolvectl commands.
	t.Setenv("PATH", dir)
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	t.Setenv("NETGAP_TEST_SETTINGS", file)
	return file
}

const fixtureGsettings = `
[ "$1" = "list-recursively" ] && [ "$2" = "org.gnome.system.proxy" ]
exec /bin/cat "$NETGAP_TEST_SETTINGS"
`

func TestLinuxGnomeDetectionAndFingerprint(t *testing.T) {
	file := fakeGsettings(t, fixtureGsettings)
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	p, source, err := DetectProxyWithSource(context.Background())
	if err != nil || source != sourceSystem || !p.UsesURLRules() {
		t.Fatalf("source=%s err=%v", source, err)
	}
	if got := gnomePaths(t, p, "https://public.example")[0]; got != "http://192.0.2.20:8102" {
		t.Fatal(got)
	}
	first, err := ProxyFingerprint(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	again, err := ProxyFingerprint(context.Background())
	if err != nil || first != again {
		t.Fatal("unchanged snapshot fingerprint changed", err)
	}
	if err := os.WriteFile(file, []byte(gnomeFixture(map[string]string{"ignore-hosts": "['public.example']"})), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := ProxyFingerprint(context.Background())
	if err != nil || changed == first {
		t.Fatal("bypass change was not fingerprinted", err)
	}
	// A selected manual mode with all endpoints disabled expresses DIRECT;
	// it must not fall through to an unrelated environment proxy.
	if err := os.WriteFile(file, []byte(gnomeFixture(map[string]string{"http.host": "''", "https.host": "''", "socks.host": "''"})), 0600); err != nil {
		t.Fatal(err)
	}
	p, source, err = DetectProxyWithSource(context.Background())
	if err != nil || source != sourceSystem {
		t.Fatalf("source=%s err=%v", source, err)
	}
	if paths := gnomePaths(t, p, "https://public.example"); len(paths) != 1 || paths[0] != "DIRECT" {
		t.Fatal(paths)
	}
}

func TestLinuxGnomeReadFailureAndRecovery(t *testing.T) {
	file := fakeGsettings(t, fixtureGsettings)
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	for _, output := range []string{gnomeFixture(map[string]string{"mode": "'unknown'"}), "truncated"} {
		if err := os.WriteFile(file, []byte(output), 0600); err != nil {
			t.Fatal(err)
		}
		p, source, err := DetectProxyWithSource(context.Background())
		if !errors.Is(err, ErrProxyRules) || source != sourceSystem || p != nil {
			t.Fatal("failed rules fell back to another path", err)
		}
	}
	if err := os.WriteFile(file, []byte(gnomeFixture(nil)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := DetectProxyWithSource(context.Background()); err != nil {
		t.Fatal("did not recover", err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if _, _, err := DetectProxyWithSource(context.Background()); !errors.Is(err, ErrProxyRules) {
		t.Fatal("helper exit failure became DIRECT", err)
	}
}

func TestLinuxGnomeCommandDeadlineAndOutputLimit(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		fakeGsettings(t, "exec /bin/sleep 30")
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		if _, _, err := DetectProxyWithSource(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		if time.Since(start) > 2*time.Second {
			t.Fatal("deadline did not stop helper")
		}
	})
	t.Run("output-limit", func(t *testing.T) {
		fakeGsettings(t, "exec /usr/bin/head -c 70000 /dev/zero")
		if _, _, err := DetectProxyWithSource(context.Background()); !errors.Is(err, ErrProxyRules) || !strings.Contains(err.Error(), "64 KiB") {
			t.Fatal("output limit missing", err)
		}
	})
}

func TestLinuxGnomeCancellationKillsHelperGroup(t *testing.T) {
	file := fakeGsettings(t, `/bin/sleep 30 &
printf '%s' "$!" > "$NETGAP_TEST_SETTINGS"
wait`)
	// Avoid interpreting the initial settings fixture as a PID.
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, _, err := DetectProxyWithSource(ctx); done <- err }()
	pid := 0
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(file)
		pid, _ = strconv.Atoi(string(data))
		if pid > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not release helper pipes")
	}
	if pid <= 0 {
		t.Fatal("helper child did not start")
	}
	// A zombie has already exited and may await WSL init's reap; it is not a
	// surviving process. Never signal arbitrary pre-existing system processes.
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		fields := strings.Fields(string(data))
		if len(fields) > 2 && fields[2] == "Z" {
			return
		}
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("helper descendant survived cancellation")
}

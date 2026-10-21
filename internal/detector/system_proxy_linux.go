//go:build linux

package detector

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

func getProxyFromSystem(ctx context.Context) *ProxyConfig {
	desktop := os.Getenv("XDG_CURRENT_DESKTOP")
	gnome, kde := false, false
	for _, name := range strings.Split(desktop, ":") {
		gnome = gnome || name == "GNOME"
		kde = kde || name == "KDE"
	}
	if !gnome && !kde {
		// Installed GNOME schemas do not identify the current desktop. Only an
		// explicitly headless process may use environment configuration alone.
		if desktop == "" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" && os.Getenv("DESKTOP_SESSION") == "" {
			return nil
		}
		return &ProxyConfig{configErr: errors.New("unidentified desktop proxy source; Linux supports GNOME/KDE fixed proxy settings only")}
	}
	if kde && !gnome {
		p, err := kdeSnapshot(ctx)
		if err != nil {
			return &ProxyConfig{configErr: err}
		}
		return p
	}
	// A recognized GNOME session requires its settings reader. Missing tools
	// and failed reads must not masquerade as an unconfigured system proxy.
	if _, err := exec.LookPath("gsettings"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return &ProxyConfig{configErr: errors.New("GNOME session requires gsettings and desktop proxy schemas")}
		}
		return &ProxyConfig{configErr: fmt.Errorf("locate gsettings: %w", err)}
	}
	// One bounded snapshot avoids separate process startup for every key and
	// preserves all protocol settings for both URL selection and fingerprints.
	output, err := runGsettings(ctx, "list-recursively", gnomeProxySchema)
	if err != nil {
		return &ProxyConfig{configErr: err}
	}
	proxy, err := parseGnomeSnapshot(output)
	if err != nil {
		return &ProxyConfig{configErr: err}
	}
	return proxy
}

// limitedSettingsOutput prevents a broken helper from growing memory without
// bound. Stderr is discarded rather than exposing private configuration data.
var errSettingsOutputLimit = errors.New("gsettings output exceeds 64 KiB")

type limitedSettingsOutput struct {
	data          bytes.Buffer
	limitExceeded bool
}

func (b *limitedSettingsOutput) Write(p []byte) (int, error) {
	if len(p) > 64*1024-b.data.Len() {
		b.limitExceeded = true
		return 0, errSettingsOutputLimit
	}
	return b.data.Write(p)
}

func runGsettings(ctx context.Context, args ...string) (string, error) {
	return runConfiguredSettingsHelper(ctx, "gsettings", args, linuxDesktopCommand)
}

func runSettingsHelper(ctx context.Context, executable string, args ...string) (string, error) {
	return runConfiguredSettingsHelper(ctx, executable, args, nil)
}

func runConfiguredSettingsHelper(ctx context.Context, executable string, args []string, configure func(*exec.Cmd) error) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if configure != nil {
		if err := configure(cmd); err != nil {
			return "", err
		}
	}
	cmd.Cancel = func() error {
		// Kill the helper's process group as well, so a descendant cannot hold
		// the output pipe open after the main process is canceled.
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	cmd.WaitDelay = 250 * time.Millisecond
	var out limitedSettingsOutput
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	// A helper may exit with SIGPIPE after the copy goroutine rejects excess
	// output. Preserve the actual limit failure rather than the exit signal.
	if out.limitExceeded {
		return "", fmt.Errorf("read GNOME proxy snapshot: %w", errSettingsOutputLimit)
	}
	if err != nil {
		return "", fmt.Errorf("read GNOME proxy snapshot: %w", err)
	}
	return out.data.String(), nil
}

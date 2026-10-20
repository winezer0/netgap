//go:build linux

package detector

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type gioReply struct {
	Version    int      `json:"version"`
	Backend    string   `json:"backend"`
	State      string   `json:"state"`
	Configured bool     `json:"configured"`
	Routes     []string `json:"routes"`
	Error      string   `json:"error"`
}

func gioSnapshot(ctx context.Context) (*ProxyConfig, error) {
	reply, err := callGIO(ctx, "--snapshot")
	if err != nil {
		return nil, err
	}
	if !reply.Configured {
		return nil, nil
	}
	return &ProxyConfig{Enabled: true, Type: "rules", rules: &proxyRules{gioState: reply.State}}, nil
}

func callGIO(ctx context.Context, target string) (*gioReply, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	// Never search PATH for an executable that may be launched with route privileges.
	helper := filepath.Join(filepath.Dir(executable), "netgap-gio-proxy")
	return callGIOHelper(ctx, helper, target)
}

func callGIOHelper(ctx context.Context, helper, target string) (*gioReply, error) {
	output, err := runConfiguredSettingsHelper(ctx, helper, []string{target}, gioSessionCommand)
	if err != nil {
		return nil, fmt.Errorf("GIO helper unavailable or failed (install netgap-gio-proxy beside the application): %w", err)
	}
	var reply gioReply
	if err := json.Unmarshal([]byte(output), &reply); err != nil {
		return nil, errors.New("invalid GIO helper response")
	}
	if reply.Error != "" {
		return nil, fmt.Errorf("GIO: %s", reply.Error)
	}
	digest, err := hex.DecodeString(reply.State)
	if reply.Version != 1 || (reply.Backend != "GProxyResolverGnome" && reply.Backend != "GLibproxyResolver") || err != nil || len(digest) != 32 {
		return nil, errors.New("invalid GIO backend or state")
	}
	return &reply, nil
}

func resolveGIOProxy(ctx context.Context, state string, target *url.URL) ([]ProxyRoute, error) {
	if target == nil || target.Hostname() == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return nil, errors.New("invalid GIO target")
	}
	reply, err := callGIO(ctx, target.String())
	if err != nil {
		return nil, err
	}
	if reply.State != state {
		return nil, ErrStateChanged
	}
	return gioRoutes(reply.Routes)
}

// The parent retains route privileges; only the disposable resolver runs as
// the original sudo user. Missing or foreign sessions are explicit errors.
func gioSessionCommand(cmd *exec.Cmd) error {
	uidText := os.Getenv("SUDO_UID")
	if uidText == "" || uidText == strconv.Itoa(os.Geteuid()) {
		return nil
	}
	if os.Geteuid() != 0 {
		return errors.New("desktop session identity does not match execution identity")
	}
	uid, err := strconv.ParseUint(uidText, 10, 32)
	if err != nil || uid == 0 {
		return errors.New("invalid original desktop user")
	}
	account, err := user.LookupId(uidText)
	if err != nil {
		return errors.New("cannot identify original desktop user")
	}
	gid, err := strconv.ParseUint(account.Gid, 10, 32)
	if err != nil {
		return err
	}
	runtimeDir := filepath.Join("/run/user", uidText)
	for _, path := range []string{runtimeDir, filepath.Join(runtimeDir, "bus")} {
		info, err := os.Stat(path)
		if err != nil {
			return errors.New("original desktop session unavailable; use the logged-in user's active session")
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uint32(uid) {
			return errors.New("desktop runtime directory or bus belongs to another user")
		}
	}
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		configDir = filepath.Join(account.HomeDir, ".config")
	} else {
		info, err := os.Stat(configDir)
		if err != nil {
			return errors.New("original desktop configuration directory unavailable")
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uint32(uid) || !filepath.IsAbs(configDir) {
			return errors.New("desktop configuration directory belongs to another identity")
		}
	}
	groups, err := account.GroupIds()
	if err != nil {
		return err
	}
	credential := &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
	for _, group := range groups {
		value, err := strconv.ParseUint(group, 10, 32)
		if err != nil {
			return err
		}
		credential.Groups = append(credential.Groups, uint32(value))
	}
	cmd.SysProcAttr.Credential = credential
	overrides := map[string]string{"HOME": account.HomeDir, "USER": account.Username, "LOGNAME": account.Username, "XDG_RUNTIME_DIR": runtimeDir, "DBUS_SESSION_BUS_ADDRESS": "unix:path=" + filepath.Join(runtimeDir, "bus"), "XDG_CONFIG_HOME": configDir}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if _, replaced := overrides[key]; !replaced {
			cmd.Env = append(cmd.Env, item)
		}
	}
	for key, value := range overrides {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	return nil
}

func gioRoutes(values []string) ([]ProxyRoute, error) {
	if len(values) == 0 || len(values) > 64 {
		return nil, errors.New("invalid GIO candidate count")
	}
	var result []ProxyRoute
	for _, value := range values {
		if value == "direct://" {
			result = append(result, ProxyRoute{})
			continue
		}
		u, err := url.Parse(value)
		if err != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || strings.ContainsAny(value, "\r\n\x00") {
			return nil, errors.New("invalid GIO proxy URI")
		}
		if u.Scheme == "socks" {
			for _, scheme := range []string{"socks5", "socks4a", "socks4"} {
				copyURL := *u
				copyURL.Scheme = scheme
				paths, err := checkedRoute(&copyURL)
				if err != nil {
					return nil, err
				}
				result = append(result, paths...)
			}
		} else {
			paths, err := checkedRoute(u)
			if err != nil {
				return nil, err
			}
			result = append(result, paths...)
		}
	}
	return result, nil
}

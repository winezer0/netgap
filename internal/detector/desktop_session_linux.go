//go:build linux

package detector

import (
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Only the settings reader drops privileges. The parent retains the identity
// required for route protection. An unidentifiable desktop session is an error.
func linuxDesktopCommand(cmd *exec.Cmd) error {
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
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
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

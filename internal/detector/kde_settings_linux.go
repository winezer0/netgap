//go:build linux

package detector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"
)

func kdeSnapshot(ctx context.Context) (*ProxyConfig, error) {
	// Reuse identity/session validation without starting any subprocess.
	command := exec.Command("unused-desktop-settings-reader")
	if err := linuxDesktopCommand(command); err != nil {
		return nil, err
	}
	env := command.Env
	if env == nil {
		env = os.Environ()
	}
	getenv := func(key string) string {
		for _, item := range env {
			if value, ok := strings.CutPrefix(item, key+"="); ok {
				return value
			}
		}
		return ""
	}
	config := getenv("XDG_CONFIG_HOME")
	if config == "" {
		home := getenv("HOME")
		if home == "" || !filepath.IsAbs(home) {
			return nil, errors.New("cannot identify KDE user configuration directory")
		}
		config = filepath.Join(home, ".config")
	}
	dirs := getenv("XDG_CONFIG_DIRS")
	if dirs == "" {
		dirs = "/etc/xdg"
	}
	roots := append([]string{config}, strings.Split(dirs, ":")...)
	if len(roots) > 32 {
		return nil, errors.New("too many KDE configuration roots")
	}
	values := make(map[string]string)
	digest := sha256.New()
	fmt.Fprintf(digest, "%q|%q|%q|", os.Getenv("XDG_CURRENT_DESKTOP"), os.Getenv("SUDO_UID"), getenv("HOME"))
	for i := len(roots) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !filepath.IsAbs(roots[i]) {
			return nil, errors.New("invalid KDE XDG configuration root")
		}
		path := filepath.Join(roots[i], "kioslaverc")
		file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(digest, "%q:missing|", path)
			continue
		}
		if err != nil {
			return nil, errors.New("cannot read KDE proxy configuration")
		}
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			file.Close()
			return nil, errors.New("KDE proxy configuration is not a regular file")
		}
		if command.SysProcAttr != nil && command.SysProcAttr.Credential != nil {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || (stat.Uid != 0 && stat.Uid != command.SysProcAttr.Credential.Uid) {
				file.Close()
				return nil, errors.New("KDE proxy configuration belongs to another identity")
			}
		}
		data, err := io.ReadAll(io.LimitReader(file, 1024*1024+1))
		file.Close()
		if err != nil || len(data) > 1024*1024 {
			return nil, errors.New("KDE proxy configuration unreadable or exceeds 1 MiB")
		}
		fmt.Fprintf(digest, "%q:%x|", path, sha256.Sum256(data))
		if err := mergeKDEProxySettings(values, string(data)); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return parseKDESettings(values, hex.EncodeToString(digest.Sum(nil)))
}

func mergeKDEProxySettings(values map[string]string, data string) error {
	if !utf8.ValidString(data) || strings.ContainsRune(data, 0) {
		return errors.New("invalid KDE configuration encoding")
	}
	inProxy := false
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return errors.New("invalid KDE configuration group")
			}
			if strings.HasPrefix(line, "[Proxy Settings]") && line != "[Proxy Settings]" {
				return errors.New("unsupported KDE proxy group options")
			}
			inProxy = line == "[Proxy Settings]"
			continue
		}
		if !inProxy {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return errors.New("invalid KDE proxy configuration entry")
		}
		// Immutable/deleted/localized/expanded KConfig entries require the full
		// KConfig engine. Reject them rather than silently choosing a wrong path.
		if strings.ContainsAny(key, "[]") {
			return errors.New("unsupported KDE proxy entry options")
		}
		decoded, err := kdeConfigString(strings.TrimSpace(value))
		if err != nil {
			return err
		}
		values[key] = decoded
	}
	return nil
}

func kdeConfigString(value string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' {
			out.WriteByte(value[i])
			continue
		}
		i++
		if i == len(value) {
			return "", errors.New("truncated KDE configuration escape")
		}
		switch value[i] {
		case '\\':
			out.WriteByte('\\')
		case 's':
			out.WriteByte(' ')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'x':
			if i+2 >= len(value) {
				return "", errors.New("truncated KDE hexadecimal escape")
			}
			decoded, err := hex.DecodeString(value[i+1 : i+3])
			if err != nil {
				return "", errors.New("invalid KDE hexadecimal escape")
			}
			out.WriteByte(decoded[0])
			i += 2
		default:
			return "", errors.New("unsupported KDE configuration escape")
		}
	}
	result := out.String()
	if !utf8.ValidString(result) || strings.ContainsRune(result, 0) {
		return "", errors.New("invalid KDE configuration value")
	}
	return result, nil
}

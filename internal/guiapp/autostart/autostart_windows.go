package autostart

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const valueName = "NetGap"

func startupCommand(executable string) string {
	// Windows paths cannot contain quotes. Always quote even paths without spaces.
	return `"` + executable + `" --autostart`
}

func executableCommand() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", err
	}
	return startupCommand(executable), nil
}

func statusAt(path, command string) (*Status, error) {
	status := &Status{Supported: true}
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return nil, err
	}
	defer key.Close()
	value, _, err := key.GetStringValue(valueName)
	if errors.Is(err, registry.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return nil, err
	}
	status.Registered = true
	status.NeedsRepair = !strings.EqualFold(value, command)
	return status, nil
}

func setAt(path, command string, enabled bool) (*Status, error) {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return nil, err
	}
	if enabled {
		err = key.SetStringValue(valueName, command)
	} else {
		err = key.DeleteValue(valueName)
	}
	key.Close()
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return nil, err
	}
	return statusAt(path, command)
}

func GetStatus() (*Status, error) {
	command, err := executableCommand()
	if err != nil {
		return nil, err
	}
	return statusAt(runKey, command)
}

func SetEnabled(enabled bool) (*Status, error) {
	command, err := executableCommand()
	if err != nil {
		return nil, err
	}
	return setAt(runKey, command, enabled)
}

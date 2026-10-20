package guiapp

import (
	"os"
	"path/filepath"
	"runtime"
)

// stateDir keeps macOS runtime data outside the signed application bundle.
func stateDir() (string, error) {
	if runtime.GOOS != "darwin" {
		return appDir(), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, appName), nil
}

func appDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	dir := filepath.Dir(exe)
	if dir == "" {
		return "."
	}
	return dir
}

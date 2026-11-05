package updater

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func appBundlePath(executable string) (string, error) {
	current := filepath.Clean(executable)
	for {
		if strings.EqualFold(filepath.Ext(current), ".app") {
			info, err := os.Stat(current)
			if err != nil {
				return "", err
			}
			if info.IsDir() {
				return current, nil
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("could not locate the running macOS .app bundle")
		}
		current = parent
	}
}

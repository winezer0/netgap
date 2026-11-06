package downloadunpack

import (
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/winezer0/downutils"
)

func normalizeExpectedFilePath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	path = strings.ReplaceAll(path, `\`, "/")
	if strings.ContainsRune(path, '\x00') || strings.HasPrefix(path, "/") || hasWindowsDrivePrefix(path) {
		return "", fmt.Errorf("expected file must be a relative file path")
	}
	local := filepath.FromSlash(path)
	if !filepath.IsLocal(local) {
		return "", fmt.Errorf("expected file must be a relative file path")
	}
	clean := pathpkg.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("expected file must be a relative file path")
	}
	return clean, nil
}

func hasWindowsDrivePrefix(path string) bool {
	if len(path) < 2 || path[1] != ':' {
		return false
	}
	return path[0] >= 'a' && path[0] <= 'z' || path[0] >= 'A' && path[0] <= 'Z'
}

func resolveDownloadPath(request Request) (string, error) {
	if request.Item.FileName == "" {
		return "", nil
	}
	options := request.DownloadOptions
	if options == nil {
		options = downutils.NewDefaultDownOptions()
	}
	path := downutils.GetDownItemFinalPath(request.Item.FileName, request.Item.StorageDir, options.OutputForce)
	return absoluteCleanPath(path)
}

func absoluteCleanPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	original := abs
	current := abs
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if _, lstatErr := os.Lstat(current); lstatErr == nil {
			return "", err
		} else if !errors.Is(lstatErr, os.ErrNotExist) {
			return "", lstatErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return original, nil
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

// pathsOverlap reports whether either path contains the other, including equality.
func pathsOverlap(first, second string) bool {
	first = filepath.Clean(first)
	second = filepath.Clean(second)
	if runtime.GOOS == "windows" {
		first = strings.ToLower(first)
		second = strings.ToLower(second)
	}
	return pathContains(first, second) || pathContains(second, first)
}

func pathContains(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func resolveOutputFile(root, relative string) (string, error) {
	relative, err := normalizeExpectedFilePath(relative)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, filepath.FromSlash(relative))
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("expected file path escapes extraction destination")
	}
	info, err := os.Lstat(target)
	if err != nil {
		return "", fmt.Errorf("expected output file %q: %w", relative, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("expected output %q is not a regular file", relative)
	}
	return filepath.ToSlash(rel), nil
}

func hasKnownArchiveExtension(sourcePath string) bool {
	name := strings.ToLower(filepath.Base(sourcePath))
	for _, extension := range []string{
		".zip", ".tar", ".tar.gz", ".tgz", ".rar", ".7z",
		".br", ".bz2", ".gz", ".gzip", ".lz4", ".lz", ".mz", ".sz", ".s2",
		".xz", ".zz", ".zst",
	} {
		if strings.HasSuffix(name, extension) {
			return true
		}
	}
	return false
}

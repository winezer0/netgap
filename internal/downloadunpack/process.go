package downloadunpack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archives"
	"github.com/winezer0/dezip"
)

func processDownloadedFile(ctx context.Context, sourcePath, destination string, mode Mode, options dezip.Options, expectedFile string) (dezip.Result, string, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return dezip.Result{}, "", false, err
	}

	dest, err := filepath.Abs(destination)
	if err != nil {
		return dezip.Result{}, "", false, fmt.Errorf("resolve destination: %w", err)
	}
	dest = filepath.Clean(dest)
	if _, err := os.Lstat(dest); err == nil {
		return dezip.Result{}, "", false, dezip.ErrDestinationExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return dezip.Result{}, "", false, fmt.Errorf("check destination: %w", err)
	}

	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return dezip.Result{}, "", false, fmt.Errorf("create destination parent: %w", err)
	}
	stage, err := os.MkdirTemp(parent, ".downloadunpack-*")
	if err != nil {
		return dezip.Result{}, "", false, fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	stagedOutput := filepath.Join(stage, "output")

	processed, err := dezip.Unpack(ctx, sourcePath, stagedOutput, options)
	if err != nil {
		if mode == ModeAuto && expectedFile == "" && !hasKnownArchiveExtension(sourcePath) && errors.Is(err, archives.NoMatch) && ctx.Err() == nil {
			return dezip.Result{}, "", true, nil
		}
		return unpublishedResult(processed), "", false, err
	}
	if !formatMatchesMode(processed.Format, mode) {
		return unpublishedResult(processed), "", false, fmt.Errorf("%w: requested %s, detected %s", ErrModeMismatch, mode, processed.Format)
	}

	mainFileRelative := expectedFile
	if mainFileRelative == "" && (mode == ModeGzip || processed.Format == "gz" || processed.Format == "gzip") {
		mainFileRelative = decompressedOutputName(sourcePath, processed.Format, options.OutputName)
	}
	if mainFileRelative != "" {
		if _, err := resolveOutputFile(stagedOutput, mainFileRelative); err != nil {
			return unpublishedResult(processed), "", false, err
		}
	}
	if err := ctx.Err(); err != nil {
		return unpublishedResult(processed), "", false, err
	}
	if _, err := os.Lstat(dest); err == nil {
		return unpublishedResult(processed), "", false, dezip.ErrDestinationExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return unpublishedResult(processed), "", false, fmt.Errorf("check destination: %w", err)
	}
	if err := os.Rename(stagedOutput, dest); err != nil {
		return unpublishedResult(processed), "", false, fmt.Errorf("publish unpacked output: %w", err)
	}
	processed.OutputDir = dest

	mainFile := ""
	if mainFileRelative != "" {
		mainFile = filepath.Join(dest, filepath.FromSlash(mainFileRelative))
	}
	return processed, mainFile, false, nil
}

func unpublishedResult(result dezip.Result) dezip.Result {
	result.OutputDir = ""
	return result
}

func formatMatchesMode(format string, mode Mode) bool {
	extension := strings.ToLower(strings.TrimPrefix(format, "."))
	switch mode {
	case ModeZip:
		return extension == "zip"
	case ModeTar:
		return extension == "tar" || extension == "tgz" || strings.HasPrefix(extension, "tar.")
	case ModeGzip:
		return extension == "gz" || extension == "gzip"
	default:
		return mode == ModeAuto
	}
}

func decompressedOutputName(sourcePath, format, configuredName string) string {
	if configuredName != "" {
		return configuredName
	}
	base := filepath.Base(sourcePath)
	for _, suffix := range []string{"." + format, ".gzip", ".gz"} {
		if strings.HasSuffix(strings.ToLower(base), strings.ToLower(suffix)) && len(base) > len(suffix) {
			return base[:len(base)-len(suffix)]
		}
	}
	return ""
}

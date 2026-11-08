package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/winezer0/dezip"
	"github.com/winezer0/downunpack"
)

const (
	maxUpdateArchiveEntries       = 100_000
	maxUpdateArchiveExpandedBytes = 1 << 30
)

func extractPayload(ctx context.Context, archivePath, destination string) (string, error) {
	assetName := filepath.Base(archivePath)
	mode := downloadunpack.ModeZip
	switch assetName {
	case "netgapgui-windows-amd64.zip", "netgapgui-macos-universal.zip":
	case "netgapgui-linux-amd64.tar.gz":
		mode = downloadunpack.ModeTar
	default:
		return "", fmt.Errorf("unsupported update archive %q", assetName)
	}
	if err := unpackUpdateArchive(ctx, archivePath, destination, mode); err != nil {
		return "", err
	}

	var payload string
	switch runtime.GOOS {
	case "windows":
		payload = filepath.Join(destination, "netgapgui.exe")
	case "linux":
		payload = filepath.Join(destination, "netgapgui")
	case "darwin":
		payload = filepath.Join(destination, "netgapgui.app")
	default:
		return "", fmt.Errorf("automatic updates are not supported on %s", runtime.GOOS)
	}
	info, err := os.Lstat(payload)
	if err != nil {
		return "", fmt.Errorf("update archive does not contain %s: %w", filepath.Base(payload), err)
	}
	if runtime.GOOS == "darwin" && !info.IsDir() {
		return "", errors.New("macOS update archive does not contain an application bundle")
	}
	if runtime.GOOS != "darwin" && !info.Mode().IsRegular() {
		return "", errors.New("update archive payload is not a regular file")
	}
	return payload, nil
}

func unpackUpdateArchive(ctx context.Context, archivePath, destination string, mode downloadunpack.Mode) error {
	_, err := downloadunpack.ProcessFile(ctx, downloadunpack.ProcessRequest{
		SourcePath:  archivePath,
		ExtractAs:   string(mode),
		Destination: destination,
		ExtractOptions: dezip.Options{
			MaxEntries:       maxUpdateArchiveEntries,
			MaxExpandedBytes: maxUpdateArchiveExpandedBytes,
			AllowSymlinks:    true,
		},
	})
	return err
}

// Keep format-specific wrappers for focused archive validation at package scope.
func extractZip(archivePath, destination string) error {
	return unpackUpdateArchive(context.Background(), archivePath, destination, downloadunpack.ModeZip)
}

func extractTarGz(archivePath, destination string) error {
	return unpackUpdateArchive(context.Background(), archivePath, destination, downloadunpack.ModeTar)
}

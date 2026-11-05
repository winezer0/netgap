package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const maxExpandedSize = uint64(1 << 30)

func extractPayload(archivePath, destination string) (string, error) {
	assetName := filepath.Base(archivePath)
	switch assetName {
	case "netgapgui-windows-amd64.zip", "netgapgui-macos-universal.zip":
		if err := extractZip(archivePath, destination); err != nil {
			return "", err
		}
	case "netgapgui-linux-amd64.tar.gz":
		if err := extractTarGz(archivePath, destination); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported update archive %q", assetName)
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

func extractZip(archivePath, destination string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer archive.Close()
	if len(archive.File) > 100000 {
		return errors.New("update archive contains too many files")
	}
	var expandedSize uint64
	for _, entry := range archive.File {
		if entry.UncompressedSize64 > maxExpandedSize-expandedSize {
			return errors.New("expanded update archive exceeds size limit")
		}
		expandedSize += entry.UncompressedSize64
		target, err := safeArchiveTarget(destination, entry.Name)
		if err != nil {
			return err
		}
		if err := ensureNoSymlinkParents(destination, target); err != nil {
			return err
		}
		mode := entry.Mode()
		if mode.IsDir() {
			if err := checkArchiveDestination(target, true); err != nil {
				return err
			}
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if mode&os.ModeSymlink != 0 {
			if err := checkArchiveDestination(target, false); err != nil {
				return err
			}
			if err := extractZipSymlink(destination, target, entry); err != nil {
				return err
			}
			continue
		}
		if !mode.IsRegular() && mode != 0 {
			return fmt.Errorf("unsupported file type in update archive: %s", entry.Name)
		}
		if err := checkArchiveDestination(target, false); err != nil {
			return err
		}
		reader, err := entry.Open()
		if err != nil {
			return err
		}
		permissions := mode.Perm()
		if permissions == 0 {
			permissions = 0o644
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, permissions)
		if err != nil {
			_ = reader.Close()
			return err
		}
		_, copyErr := io.Copy(file, reader)
		readerErr := reader.Close()
		fileErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if readerErr != nil {
			return readerErr
		}
		if fileErr != nil {
			return fileErr
		}
	}
	return nil
}

func extractZipSymlink(root, target string, entry *zip.File) error {
	reader, err := entry.Open()
	if err != nil {
		return err
	}
	linkData, readErr := io.ReadAll(io.LimitReader(reader, 4096))
	closeErr := reader.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	linkTarget := string(linkData)
	if linkTarget == "" || filepath.IsAbs(linkTarget) {
		return fmt.Errorf("unsafe symbolic link in update archive: %s", entry.Name)
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(target), filepath.FromSlash(linkTarget)))
	if !isWithin(root, resolved) {
		return fmt.Errorf("symbolic link escapes update archive: %s", entry.Name)
	}
	return os.Symlink(filepath.FromSlash(linkTarget), target)
}

func extractTarGz(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	var expandedSize uint64
	var entryCount int
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		entryCount++
		if entryCount > 100000 || header.Size < 0 {
			return errors.New("invalid or oversized update archive")
		}
		if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA {
			if uint64(header.Size) > maxExpandedSize-expandedSize {
				return errors.New("expanded update archive exceeds size limit")
			}
			expandedSize += uint64(header.Size)
		}
		target, err := safeArchiveTarget(destination, header.Name)
		if err != nil {
			return err
		}
		if err := ensureNoSymlinkParents(destination, target); err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := checkArchiveDestination(target, true); err != nil {
				return err
			}
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := checkArchiveDestination(target, false); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			permissions := os.FileMode(header.Mode).Perm()
			if permissions == 0 {
				permissions = 0o644
			}
			output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, permissions)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(output, reader)
			closeErr := output.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			if err := checkArchiveDestination(target, false); err != nil {
				return err
			}
			if err := createSafeSymlink(destination, target, header.Linkname); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported file type in update archive: %s", header.Name)
		}
	}
}

func safeArchiveTarget(root, name string) (string, error) {
	if strings.ContainsRune(name, '\x00') {
		return "", errors.New("invalid path in update archive")
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe path in update archive: %s", name)
	}
	target := filepath.Join(root, clean)
	if !isWithin(root, target) {
		return "", fmt.Errorf("unsafe path in update archive: %s", name)
	}
	return target, nil
}

func ensureNoSymlinkParents(root, target string) error {
	relative, err := filepath.Rel(root, filepath.Dir(target))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("update archive path escapes extraction directory")
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive entry traverses a symbolic link: %s", current)
		}
	}
	return nil
}

func checkArchiveDestination(target string, directory bool) error {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if directory && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	return fmt.Errorf("duplicate or conflicting update archive entry: %s", target)
}

func createSafeSymlink(root, target, linkTarget string) error {
	if linkTarget == "" || filepath.IsAbs(filepath.FromSlash(linkTarget)) {
		return fmt.Errorf("unsafe symbolic link target: %s", linkTarget)
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(target), filepath.FromSlash(linkTarget)))
	if !isWithin(root, resolved) {
		return fmt.Errorf("symbolic link escapes update archive: %s", linkTarget)
	}
	return os.Symlink(filepath.FromSlash(linkTarget), target)
}

func isWithin(root, target string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

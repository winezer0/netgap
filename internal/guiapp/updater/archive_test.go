package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractZipRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "unsafe.zip")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(archiveFile)
	entry, err := writer.Create("../escaped")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("outside")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(root, "unpacked")
	if err := extractZip(archivePath, destination); err == nil {
		t.Fatal("expected zip path traversal to be rejected")
	}
	if _, err := os.Stat(filepath.Join(root, "escaped")); !os.IsNotExist(err) {
		t.Fatalf("zip extraction wrote outside the destination: %v", err)
	}
}

func TestExtractTarGzRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "unsafe.tar.gz")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(archiveFile)
	tarWriter := tar.NewWriter(gzipWriter)
	content := []byte("outside")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "../escaped", Mode: 0o600, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(root, "unpacked")
	if err := extractTarGz(archivePath, destination); err == nil {
		t.Fatal("expected tar path traversal to be rejected")
	}
	if _, err := os.Stat(filepath.Join(root, "escaped")); !os.IsNotExist(err) {
		t.Fatalf("tar extraction wrote outside the destination: %v", err)
	}
}

func TestExtractZipCreatesPayload(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "safe.zip")
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.Create("netgapgui.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "unpacked")
	if err := extractZip(archivePath, destination); err != nil {
		t.Fatal(err)
	}
	if payload, err := os.ReadFile(filepath.Join(destination, "netgapgui.exe")); err != nil || string(payload) != "payload" {
		t.Fatalf("unexpected extracted payload %q, error %v", payload, err)
	}
}

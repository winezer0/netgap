package downloadunpack

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/winezer0/downutils"
)

func TestParseModeRejectsExeAlias(t *testing.T) {
	if _, err := parseMode("exe"); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("parseMode(\"exe\") error = %v, want ErrInvalidMode", err)
	}
	for _, value := range []string{"", "skip", "auto", "zip", "tar", "gzip"} {
		if _, err := parseMode(value); err != nil {
			t.Errorf("parseMode(%q): %v", value, err)
		}
	}
}

func TestNormalizeExpectedFilePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "nested relative path", input: "bin/plugin.exe", want: "bin/plugin.exe"},
		{name: "windows separators", input: `bin\plugin.exe`, want: "bin/plugin.exe"},
		{name: "internal parent component", input: "bin/../plugin.exe", want: "plugin.exe"},
		{name: "parent traversal", input: "../outside.exe", wantErr: true},
		{name: "backslash traversal", input: `..\outside.exe`, wantErr: true},
		{name: "absolute path", input: "/outside.exe", wantErr: true},
		{name: "windows drive path", input: `C:\outside.exe`, wantErr: true},
		{name: "windows drive relative path", input: "C:outside.exe", wantErr: true},
		{name: "unc path", input: `\\server\share\outside.exe`, wantErr: true},
		{name: "dot path", input: ".", wantErr: true},
		{name: "nul byte", input: "plugin\x00.exe", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizeExpectedFilePath(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("normalizeExpectedFilePath(%q) error = %v, wantErr %v", test.input, err, test.wantErr)
			}
			if err == nil && got != test.want {
				t.Fatalf("normalizeExpectedFilePath(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestRunRejectsOverlappingDownloadAndExtractionPathsBeforeDownloading(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = writer.Write([]byte("archive"))
	}))
	defer server.Close()

	tests := []struct {
		name        string
		filename    string
		destination func(root, downloadDir, archivePath string) string
	}{
		{
			name:     "same path",
			filename: "plugin.zip",
			destination: func(_, _, archivePath string) string {
				return archivePath
			},
		},
		{
			name:     "destination contains download path",
			filename: "plugin.zip",
			destination: func(_, downloadDir, _ string) string {
				return downloadDir
			},
		},
		{
			name:     "download path contains destination",
			filename: "bundle",
			destination: func(_, _, archivePath string) string {
				return filepath.Join(archivePath, "output")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			downloadDir := filepath.Join(root, "downloads")
			archivePath := filepath.Join(downloadDir, test.filename)
			request := newRequest(server.URL, downloadDir, test.filename, "zip", test.destination(root, downloadDir, archivePath))

			_, err := Run(context.Background(), request)
			if !errors.Is(err, ErrPathConflict) {
				t.Fatalf("Run() error = %v, want ErrPathConflict", err)
			}
			if requests.Load() != 0 {
				t.Fatalf("server received %d requests, want no download", requests.Load())
			}
		})
	}
}

func TestRunRejectsInvalidExpectedFilePathBeforeDownloading(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = writer.Write([]byte("archive"))
	}))
	defer server.Close()

	root := t.TempDir()
	request := newRequest(server.URL, filepath.Join(root, "downloads"), "plugin.zip", "zip", filepath.Join(root, "plugin"))
	request.ExpectedFile = `..\outside.exe`
	if _, err := Run(context.Background(), request); err == nil {
		t.Fatal("Run() succeeded with a traversal ExpectedFile path")
	}
	if requests.Load() != 0 {
		t.Fatalf("server received %d requests, want no download", requests.Load())
	}
}

func TestRunRejectsSymlinkAliasedPathsBeforeDownloading(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = writer.Write([]byte("archive"))
	}))
	defer server.Close()

	root := t.TempDir()
	downloadDir := filepath.Join(root, "downloads")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		t.Fatalf("create download directory: %v", err)
	}
	destinationAlias := filepath.Join(root, "download-alias")
	if err := os.Symlink(downloadDir, destinationAlias); err != nil {
		t.Skipf("directory symlinks are unavailable: %v", err)
	}
	request := newRequest(server.URL, downloadDir, "plugin.zip", "zip", destinationAlias)
	if _, err := Run(context.Background(), request); !errors.Is(err, ErrPathConflict) {
		t.Fatalf("Run() error = %v, want ErrPathConflict", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("server received %d requests, want no download", requests.Load())
	}
}

func TestRunManyRejectsCrossRequestPathOverlapBeforeDownloading(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = writer.Write([]byte("archive"))
	}))
	defer server.Close()

	root := t.TempDir()
	destination := filepath.Join(root, "plugin")
	requestsToRun := []Request{
		newRequest(server.URL, filepath.Join(root, "downloads"), "plugin.zip", "zip", destination),
		newRequest(server.URL, filepath.Join(destination, "nested"), "payload.bin", "skip", ""),
	}
	if _, err := RunMany(context.Background(), requestsToRun); !errors.Is(err, ErrPathConflict) {
		t.Fatalf("RunMany() error = %v, want ErrPathConflict", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("server received %d requests, want no downloads", requests.Load())
	}
}

func TestRunManyRejectsOverlappingExtractionDestinationsBeforeDownloading(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = writer.Write([]byte("archive"))
	}))
	defer server.Close()

	root := t.TempDir()
	destination := filepath.Join(root, "plugins")
	requestsToRun := []Request{
		newRequest(server.URL, filepath.Join(root, "downloads"), "first.zip", "zip", destination),
		newRequest(server.URL, filepath.Join(root, "downloads"), "second.zip", "zip", filepath.Join(destination, "nested")),
	}
	if _, err := RunMany(context.Background(), requestsToRun); !errors.Is(err, ErrPathConflict) {
		t.Fatalf("RunMany() error = %v, want ErrPathConflict", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("server received %d requests, want no downloads", requests.Load())
	}
}

func TestRunDownloadsAndUnpacksArchive(t *testing.T) {
	archive := testZip(t, map[string]string{
		"bin/plugin.exe":   "binary",
		"data/config.json": "{}",
	})
	server := serveBytes(archive)
	defer server.Close()

	root := t.TempDir()
	destination := filepath.Join(root, "plugin")
	request := newRequest(server.URL, root, "plugin.zip", "zip", destination)
	request.ExpectedFile = "bin/plugin.exe"

	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if result.Download.Status != downutils.TaskStatusSuccess || !result.Extracted || result.Skipped {
		t.Fatalf("Run() result = %+v, want downloaded and extracted", result)
	}
	if result.OutputPath != destination {
		t.Fatalf("OutputPath = %q, want %q", result.OutputPath, destination)
	}
	wantMainFile := filepath.Join(destination, "bin", "plugin.exe")
	if result.MainFile != wantMainFile {
		t.Fatalf("MainFile = %q, want %q", result.MainFile, wantMainFile)
	}
	assertContent(t, wantMainFile, "binary")
	assertContent(t, filepath.Join(destination, "data", "config.json"), "{}")
	if _, err := os.Stat(result.ArchivePath); err != nil {
		t.Fatalf("downloaded archive should remain after extraction: %v", err)
	}
}

func TestRunUnpacksTarGzipArchive(t *testing.T) {
	server := serveBytes(testTarGzip(t, map[string]string{"root/bin/plugin": "binary"}))
	defer server.Close()

	root := t.TempDir()
	destination := filepath.Join(root, "plugin")
	request := newRequest(server.URL, root, "plugin.tar.gz", "tar.gz", destination)
	request.ExpectedFile = "root/bin/plugin"

	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if !result.Extracted || result.Mode != ModeTar {
		t.Fatalf("Run() result = %+v, want extracted TAR archive", result)
	}
	assertContent(t, filepath.Join(destination, "root", "bin", "plugin"), "binary")
}

func TestRunUnpacksGzipStreamAndReportsMainFile(t *testing.T) {
	server := serveBytes(testGzip(t, []byte("binary")))
	defer server.Close()

	root := t.TempDir()
	destination := filepath.Join(root, "plugin")
	request := newRequest(server.URL, root, "plugin.bin.gz", "gzip", destination)

	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	wantMainFile := filepath.Join(destination, "plugin.bin")
	if !result.Extracted || result.MainFile != wantMainFile {
		t.Fatalf("Run() result = %+v, want gzip output %q", result, wantMainFile)
	}
	assertContent(t, wantMainFile, "binary")
}

func TestRunRemovesArchiveAfterSuccessfulExtraction(t *testing.T) {
	server := serveBytes(testZip(t, map[string]string{"plugin.exe": "binary"}))
	defer server.Close()

	root := t.TempDir()
	request := newRequest(server.URL, root, "plugin.zip", "zip", filepath.Join(root, "plugin"))
	request.ExpectedFile = "plugin.exe"
	request.RemoveArchiveAfterSuccess = true

	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if !result.ArchiveRemoved {
		t.Fatal("ArchiveRemoved = false, want true")
	}
	if _, err := os.Stat(result.ArchivePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive stat error = %v, want not-exist", err)
	}
	assertContent(t, result.MainFile, "binary")
}

func TestRunSkipKeepsDirectDownloadUnchanged(t *testing.T) {
	server := serveBytes([]byte("binary payload"))
	defer server.Close()

	root := t.TempDir()
	request := newRequest(server.URL, root, "plugin.exe", "skip", "")

	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if !result.Skipped || result.Extracted || result.OutputPath != result.ArchivePath || result.MainFile != result.ArchivePath {
		t.Fatalf("Run() result = %+v, want unchanged direct download", result)
	}
	assertContent(t, result.ArchivePath, "binary payload")
}

func TestRunAutoKeepsOrdinaryFileUnchanged(t *testing.T) {
	server := serveBytes([]byte("binary payload"))
	defer server.Close()

	root := t.TempDir()
	destination := filepath.Join(root, "auto-output")
	request := newRequest(server.URL, root, "plugin.bin", "auto", destination)

	result, err := Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}
	if !result.Skipped || result.Extracted || result.OutputPath != result.ArchivePath {
		t.Fatalf("Run() result = %+v, want ordinary file kept as-is", result)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination stat error = %v, want not-exist", err)
	}
	assertContent(t, result.ArchivePath, "binary payload")
}

func TestRunChecksumFailureDoesNotExtract(t *testing.T) {
	archive := testZip(t, map[string]string{"plugin.exe": "binary"})
	server := serveBytes(archive)
	defer server.Close()

	root := t.TempDir()
	destination := filepath.Join(root, "plugin")
	request := newRequest(server.URL, root, "plugin.zip", "zip", destination)
	wrongChecksum := sha256.Sum256([]byte("not the archive"))
	request.Item.Checksum = fmt.Sprintf("sha256:%x", wrongChecksum)

	result, err := Run(context.Background(), request)
	if !errors.Is(err, ErrDownloadNotCompleted) {
		t.Fatalf("Run() error = %v, want ErrDownloadNotCompleted", err)
	}
	if result.Download.Status != downutils.TaskStatusFailed {
		t.Fatalf("download status = %q, want failed", result.Download.Status)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination stat error = %v, want not-exist", err)
	}
}

func TestRunDoesNotPublishWhenExpectedFileOrModeValidationFails(t *testing.T) {
	tests := []struct {
		name         string
		extractAs    string
		expectedFile string
	}{
		{name: "missing expected file", extractAs: "zip", expectedFile: "missing.exe"},
		{name: "mode mismatch", extractAs: "tar"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			archive := testZip(t, map[string]string{"plugin.exe": "binary"})
			server := serveBytes(archive)
			defer server.Close()

			root := t.TempDir()
			destination := filepath.Join(root, "plugin")
			request := newRequest(server.URL, root, "plugin.zip", test.extractAs, destination)
			request.ExpectedFile = test.expectedFile

			result, err := Run(context.Background(), request)
			if err == nil {
				t.Fatal("Run() succeeded, want extraction validation error")
			}
			if result.Extracted || result.Extraction.OutputDir != "" {
				t.Fatalf("Run() result = %+v, want unpublished output", result)
			}
			if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("destination stat error = %v, want not-exist", err)
			}
			matches, err := filepath.Glob(filepath.Join(root, ".downloadunpack-*"))
			if err != nil {
				t.Fatalf("find staging directories: %v", err)
			}
			if len(matches) != 0 {
				t.Fatalf("staging directories remain: %v", matches)
			}
		})
	}
}

func TestRunSkippedDownloadDoesNotExtract(t *testing.T) {
	var requests atomic.Int32
	archive := testZip(t, map[string]string{"plugin.exe": "new binary"})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = writer.Write(archive)
	}))
	defer server.Close()

	root := t.TempDir()
	downloadDir := filepath.Join(root, "downloads")
	archivePath := filepath.Join(downloadDir, "plugin.zip")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		t.Fatalf("create download directory: %v", err)
	}
	if err := os.WriteFile(archivePath, []byte("existing archive"), 0o600); err != nil {
		t.Fatalf("write existing archive: %v", err)
	}
	destination := filepath.Join(root, "plugin")
	request := newRequest(server.URL, downloadDir, "plugin.zip", "zip", destination)
	request.Item.KeepUpdated = "disable"

	result, err := Run(context.Background(), request)
	if !errors.Is(err, ErrDownloadNotCompleted) {
		t.Fatalf("Run() error = %v, want ErrDownloadNotCompleted", err)
	}
	if !result.DownloadSkipped || result.Download.Status != downutils.TaskStatusSkipped {
		t.Fatalf("download result = %+v, want skipped", result.Download)
	}
	if requests.Load() != 0 {
		t.Fatalf("server received %d requests, want 0", requests.Load())
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination stat error = %v, want not-exist", err)
	}
}

func TestRunManyContinuesAfterItemFailure(t *testing.T) {
	archive := testZip(t, map[string]string{"plugin.exe": "binary"})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/fail" {
			http.Error(writer, "unavailable", http.StatusInternalServerError)
			return
		}
		_, _ = writer.Write(archive)
	}))
	defer server.Close()

	root := t.TempDir()
	requests := []Request{
		newRequest(server.URL+"/fail", filepath.Join(root, "downloads"), "failed.zip", "zip", filepath.Join(root, "failed")),
		newRequest(server.URL+"/ok", filepath.Join(root, "downloads"), "good.zip", "zip", filepath.Join(root, "good")),
	}
	requests[1].ExpectedFile = "plugin.exe"

	outcomes, err := RunMany(context.Background(), requests)
	if err != nil {
		t.Fatalf("RunMany(): %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("RunMany() returned %d outcomes, want 2", len(outcomes))
	}
	if outcomes[0].Err == nil || outcomes[0].Result.Extracted {
		t.Fatalf("first outcome = %+v, want failure without extraction", outcomes[0])
	}
	if outcomes[1].Err != nil || !outcomes[1].Result.Extracted {
		t.Fatalf("second outcome = %+v, want successful extraction", outcomes[1])
	}
	assertContent(t, outcomes[1].Result.MainFile, "binary")
	if _, err := os.Stat(requests[0].Destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed destination stat error = %v, want not-exist", err)
	}
}

func newRequest(url, downloadDir, filename, extractAs, destination string) Request {
	options := downutils.NewDefaultDownOptions()
	options.ShowProgress = false
	options.MaxRetries = 1
	options.ConnectTimeout = 5
	options.IdleTimeout = 5

	return Request{
		Item: downutils.DownItem{
			Module:       "test-plugin",
			FileName:     filename,
			DownloadURLs: []string{url},
			KeepUpdated:  "enable",
			StorageDir:   downloadDir,
		},
		DownloadOptions: options,
		ExtractAs:       extractAs,
		Destination:     destination,
	}
}

func serveBytes(content []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(content)
	}))
}

func testZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range entries {
		member, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip member %q: %v", name, err)
		}
		if _, err := member.Write([]byte(content)); err != nil {
			t.Fatalf("write zip member %q: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buffer.Bytes()
}

func testTarGzip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(compressed)
	for name, content := range entries {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatalf("write tar header %q: %v", name, err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatalf("write tar member %q: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buffer.Bytes()
}

func testGzip(t *testing.T, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(content); err != nil {
		t.Fatalf("write gzip: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buffer.Bytes()
}

func assertContent(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(content) != want {
		t.Fatalf("content of %s = %q, want %q", path, content, want)
	}
}

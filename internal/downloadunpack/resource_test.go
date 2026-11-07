package downloadunpack

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/winezer0/downutils"
)

func TestPrepareSelectsDownloadsExtractsAndResolvesPayload(t *testing.T) {
	archive := makeTestZip(t, map[string]string{
		"netgapgui-v0.1.0/netgapgui.exe": "binary",
		"netgapgui-v0.1.0/runtime.dll":   "component",
	})
	digest := sha256.Sum256(archive)
	assetName := "netgap_windows_amd64_v0.1.0.zip"
	var assetRequests atomic.Int32
	var checksumRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/asset":
			assetRequests.Add(1)
			_, _ = writer.Write(archive)
		case "/SHA256SUMS":
			checksumRequests.Add(1)
			_, _ = io.WriteString(writer, hex.EncodeToString(digest[:]))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	stageParent := t.TempDir()
	options := downutils.NewDefaultDownOptions()
	options.ShowProgress = false
	options.MaxRetries = 1
	result, err := Prepare(context.Background(), ResourceRequest{
		Sources: []Source{{
			Name:     assetName,
			URL:      server.URL + "/asset",
			Checksum: "sha256:" + server.URL + "/SHA256SUMS",
			Size:     int64(len(archive)),
		}},
		SourcePattern:     "netgap_windows_amd64_*.zip",
		ExtractAs:         "auto",
		PayloadPattern:    "netgapgui-*",
		EntrypointPattern: "**/netgapgui.exe",
		StageParent:       stageParent,
		MaxDownloadBytes:  int64(len(archive)),
		DownloadOptions:   options,
		HTTPClient:        server.Client(),
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if !result.Extracted {
		t.Fatal("Prepare() did not extract the archive")
	}
	wantPayload := filepath.Join(result.StageDir, "extracted", "netgapgui-v0.1.0")
	if result.PayloadPath != wantPayload {
		t.Errorf("PayloadPath = %q, want %q", result.PayloadPath, wantPayload)
	}
	wantEntryPoint := filepath.Join(wantPayload, "netgapgui.exe")
	if result.EntryPoint != wantEntryPoint {
		t.Errorf("EntryPoint = %q, want %q", result.EntryPoint, wantEntryPoint)
	}
	if assetRequests.Load() != 1 || checksumRequests.Load() != 1 {
		t.Errorf("requests: asset=%d checksum=%d, want one each", assetRequests.Load(), checksumRequests.Load())
	}
	if data, err := os.ReadFile(result.EntryPoint); err != nil || string(data) != "binary" {
		t.Errorf("read entrypoint = %q, %v; want binary", data, err)
	}
	stageDir := result.StageDir
	if err := result.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, err := os.Stat(stageDir); !os.IsNotExist(err) {
		t.Errorf("stage directory still exists after Cleanup(): %v", err)
	}
}

func TestPrepareSkipKeepsDownloadedFile(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(writer, "standalone executable")
	}))
	defer server.Close()

	result, err := Prepare(context.Background(), ResourceRequest{
		Sources:     []Source{{Name: "netgap-v0.1.0.exe", URL: server.URL + "/asset"}},
		ExtractAs:   "skip",
		StageParent: t.TempDir(),
		HTTPClient:  server.Client(),
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	defer result.Cleanup()
	if result.Extracted {
		t.Error("Prepare() extracted a skipped resource")
	}
	if result.PayloadPath != result.Download.StoragePath {
		t.Errorf("PayloadPath = %q, want downloaded path %q", result.PayloadPath, result.Download.StoragePath)
	}
	if data, err := os.ReadFile(result.PayloadPath); err != nil || string(data) != "standalone executable" {
		t.Errorf("read payload = %q, %v", data, err)
	}
	if requests.Load() != 1 {
		t.Errorf("server received %d requests, want one", requests.Load())
	}
}

func TestPrepareRejectsAmbiguousSourcesBeforeDownloading(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	_, err := Prepare(context.Background(), ResourceRequest{
		Sources: []Source{
			{Name: "netgap_windows_amd64_v1.zip", URL: server.URL + "/v1"},
			{Name: "netgap_windows_amd64_v2.zip", URL: server.URL + "/v2"},
		},
		SourcePattern: "netgap_windows_amd64_*.zip",
	})
	if !errors.Is(err, ErrSourceAmbiguous) {
		t.Fatalf("Prepare() error = %v, want ErrSourceAmbiguous", err)
	}
	if requests.Load() != 0 {
		t.Errorf("server received %d requests, want none", requests.Load())
	}
}

func TestPrepareCleansStageAfterChecksumFailure(t *testing.T) {
	archive := makeTestZip(t, map[string]string{"app.exe": "payload"})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path == "/asset" {
			_, _ = writer.Write(archive)
			return
		}
		_, _ = io.WriteString(writer, strings.Repeat("0", 64))
	}))
	defer server.Close()

	stageParent := t.TempDir()
	options := downutils.NewDefaultDownOptions()
	options.ShowProgress = false
	options.MaxRetries = 1
	_, err := Prepare(context.Background(), ResourceRequest{
		Sources: []Source{{
			Name:     "app.zip",
			URL:      server.URL + "/asset",
			Checksum: "sha256:" + server.URL + "/SHA256SUMS",
		}},
		StageParent:     stageParent,
		DownloadOptions: options,
		HTTPClient:      server.Client(),
	})
	if err == nil {
		t.Fatal("Prepare() succeeded with an incorrect checksum")
	}
	stages, globErr := filepath.Glob(filepath.Join(stageParent, ".downloadunpack-*"))
	if globErr != nil {
		t.Fatalf("scan staging parent: %v", globErr)
	}
	if len(stages) != 0 {
		t.Errorf("staging directories remain after failure: %v", stages)
	}
	if requests.Load() < 2 {
		t.Errorf("server received %d requests, want asset and checksum requests", requests.Load())
	}
}

func TestPrepareRejectsAmbiguousPayloadAndCleansStage(t *testing.T) {
	archive := makeTestZip(t, map[string]string{
		"app-v1.exe": "one",
		"app-v2.exe": "two",
	})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(archive)
	}))
	defer server.Close()

	stageParent := t.TempDir()
	options := downutils.NewDefaultDownOptions()
	options.ShowProgress = false
	options.MaxRetries = 1
	_, err := Prepare(context.Background(), ResourceRequest{
		Sources:         []Source{{Name: "app.zip", URL: server.URL + "/asset"}},
		PayloadPattern:  "app-*.exe",
		StageParent:     stageParent,
		DownloadOptions: options,
		HTTPClient:      server.Client(),
	})
	if !errors.Is(err, ErrPayloadAmbiguous) {
		t.Fatalf("Prepare() error = %v, want ErrPayloadAmbiguous", err)
	}
	stages, globErr := filepath.Glob(filepath.Join(stageParent, ".downloadunpack-*"))
	if globErr != nil {
		t.Fatalf("scan staging parent: %v", globErr)
	}
	if len(stages) != 0 {
		t.Errorf("staging directories remain after failure: %v", stages)
	}
}

func makeTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, contents := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", name, err)
		}
		if _, err := io.WriteString(file, contents); err != nil {
			t.Fatalf("write zip entry %q: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buffer.Bytes()
}

package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type updaterRoundTripper func(*http.Request) (*http.Response, error)

func (f updaterRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  int
	}{
		{name: "numeric components", left: "v1.10.0", right: "v1.9.99", want: 1},
		{name: "prefix is optional", left: "1.2.3", right: "v1.2.3", want: 0},
		{name: "stable follows prerelease", left: "v2.0.0", right: "v2.0.0-rc1", want: 1},
		{name: "prerelease precedes stable", left: "v2.0.0-beta", right: "v2.0.0", want: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := compareVersions(test.left, test.right)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
			}
		})
	}

	if _, err := compareVersions("dev", "v1.0.0"); err == nil {
		t.Fatal("expected invalid versions to be rejected")
	}
}

func TestCheckAndDownloadVerifiesAndPersistsUpdate(t *testing.T) {
	manager, assetName := newTestManager(t, "")
	check, err := manager.Check(context.Background(), "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !check.Available || check.LatestVersion != "v1.1.0" {
		t.Fatalf("unexpected update check result: %+v", check)
	}

	download, err := manager.Download(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !download.Available || !download.Downloaded || download.LatestVersion != "v1.1.0" {
		t.Fatalf("unexpected download result: %+v", download)
	}
	if _, err := os.Stat(manager.pendingPath); err != nil {
		t.Fatalf("pending update metadata was not saved: %v", err)
	}
	if manager.staged == nil {
		t.Fatal("verified update was not staged")
	}
	if _, err := os.Stat(manager.staged.SourcePath); err != nil {
		t.Fatalf("verified update payload was not staged: %v", err)
	}
	if manager.staged.AssetName != assetName {
		t.Fatalf("staged asset = %q, want %q", manager.staged.AssetName, assetName)
	}
	if pending := manager.Pending("v1.0.0"); pending == nil || !pending.Downloaded {
		t.Fatalf("downloaded update is not reported as pending: %+v", pending)
	}
}

func TestDownloadRejectsChecksumMismatch(t *testing.T) {
	manager, _ := newTestManager(t, strings.Repeat("0", 64))
	if _, err := manager.Check(context.Background(), "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Download(context.Background()); err == nil {
		t.Fatal("expected a checksum mismatch to fail the download")
	}
	if manager.staged != nil {
		t.Fatal("checksum mismatch must not create a pending update")
	}
	if _, err := os.Stat(manager.pendingPath); !os.IsNotExist(err) {
		t.Fatalf("checksum mismatch unexpectedly persisted metadata: %v", err)
	}
}

func TestCheckSkipsDevelopmentBuildWithoutNetwork(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: updaterRoundTripper(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, fmt.Errorf("unexpected request")
	})}
	manager := &Manager{apiClient: client}

	for _, version := range []string{"dev", "v1.2.3-4-gabcdef0", "v1.2.3-dirty"} {
		result, err := manager.Check(context.Background(), version)
		if err != nil {
			t.Fatalf("Check(%q): %v", version, err)
		}
		if result.Available {
			t.Fatalf("development version %q unexpectedly reports an update", version)
		}
	}
	if calls != 0 {
		t.Fatalf("development builds triggered %d network requests", calls)
	}
}

func newTestManager(t *testing.T, checksumOverride string) (*Manager, string) {
	t.Helper()
	assetName, err := assetNameForPlatform()
	if err != nil {
		t.Skip(err)
	}
	archive := makeTestArchive(t)
	archiveHash := sha256.Sum256(archive)
	checksum := hex.EncodeToString(archiveHash[:])
	if checksumOverride != "" {
		checksum = checksumOverride
	}
	manifest := []byte(fmt.Sprintf("%s  %s\n", checksum, assetName))
	assetURL := "https://github.com/" + repository + "/releases/download/v1.1.0/" + assetName
	checksumURL := "https://github.com/" + repository + "/releases/download/v1.1.0/SHA256SUMS"
	releaseJSON, err := json.Marshal(release{
		TagName: "v1.1.0",
		HTMLURL: "https://github.com/" + repository + "/releases/tag/v1.1.0",
		Assets: []releaseAsset{
			{Name: assetName, BrowserDownloadURL: assetURL, Size: int64(len(archive))},
			{Name: "SHA256SUMS", BrowserDownloadURL: checksumURL, Size: int64(len(manifest))},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := updaterRoundTripper(func(request *http.Request) (*http.Response, error) {
		var body []byte
		switch request.URL.String() {
		case latestReleaseURL:
			body = releaseJSON
		case checksumURL:
			body = manifest
		case assetURL:
			body = archive
		default:
			return nil, fmt.Errorf("unexpected updater request URL %q", request.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(body)),
			Request:    request,
		}, nil
	})
	root := t.TempDir()
	client := &http.Client{Transport: transport}
	return &Manager{
		root:        root,
		pendingPath: filepath.Join(root, "pending.json"),
		apiClient:   client,
		dlClient:    client,
	}, assetName
}

func makeTestArchive(t *testing.T) []byte {
	t.Helper()
	payload := []byte("test binary payload")
	var archive bytes.Buffer
	switch runtime.GOOS {
	case "windows", "darwin":
		writer := zip.NewWriter(&archive)
		if runtime.GOOS == "darwin" {
			dir := &zip.FileHeader{Name: "netgapgui.app/", Method: zip.Store}
			dir.SetMode(os.ModeDir | 0o755)
			if _, err := writer.CreateHeader(dir); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Create("netgapgui.app/Contents/MacOS/netgapgui"); err != nil {
				t.Fatal(err)
			}
		} else {
			file := &zip.FileHeader{Name: "netgapgui.exe", Method: zip.Deflate}
			file.SetMode(0o755)
			entry, err := writer.CreateHeader(file)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write(payload); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	case "linux":
		gzipWriter := gzip.NewWriter(&archive)
		tarWriter := tar.NewWriter(gzipWriter)
		header := &tar.Header{Name: "netgapgui", Mode: 0o755, Size: int64(len(payload)), Typeflag: tar.TypeReg}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(payload); err != nil {
			t.Fatal(err)
		}
		if err := tarWriter.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gzipWriter.Close(); err != nil {
			t.Fatal(err)
		}
	default:
		t.Skipf("update archive is unsupported on %s", runtime.GOOS)
	}
	return archive.Bytes()
}

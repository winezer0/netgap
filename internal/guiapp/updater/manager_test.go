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

func TestPlatformAssetAndPayloadPatterns(t *testing.T) {
	tests := []struct {
		goos        string
		goarch      string
		asset       string
		payload     string
		unsupported bool
	}{
		{goos: "windows", goarch: "amd64", asset: "netgapgui-windows-amd64*.zip", payload: "netgapgui.exe"},
		{goos: "linux", goarch: "amd64", asset: "netgapgui-linux-amd64*.tar.gz", payload: "netgapgui"},
		{goos: "darwin", goarch: "amd64", asset: "netgapgui-macos-universal*.zip", payload: "netgapgui.app"},
		{goos: "darwin", goarch: "arm64", asset: "netgapgui-macos-universal*.zip", payload: "netgapgui.app"},
		{goos: "windows", goarch: "arm64", unsupported: true},
	}
	for _, test := range tests {
		t.Run(test.goos+"/"+test.goarch, func(t *testing.T) {
			asset, assetErr := assetPatternFor(test.goos, test.goarch)
			payload, payloadErr := payloadPatternFor(test.goos)
			if test.unsupported {
				if assetErr == nil || payloadErr != nil {
					t.Fatalf("unsupported platform errors: asset=%v payload=%v", assetErr, payloadErr)
				}
				return
			}
			if assetErr != nil || payloadErr != nil {
				t.Fatalf("platform pattern errors: asset=%v payload=%v", assetErr, payloadErr)
			}
			if asset != test.asset || payload != test.payload {
				t.Fatalf("patterns = (%q, %q), want (%q, %q)", asset, payload, test.asset, test.payload)
			}
		})
	}
}

func TestLoadPendingRestoresDownloadedUpdateAfterRestart(t *testing.T) {
	root := t.TempDir()
	stageDir := filepath.Join(root, "stage-restart")
	sourcePath := filepath.Join(stageDir, "extracted", "netgapgui.exe")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("staged payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	staged := stagedUpdate{
		Version:    "v1.1.0",
		ReleaseURL: "https://github.com/" + repository + "/releases/tag/v1.1.0",
		AssetName:  "netgapgui-windows-amd64-v1.1.0.zip",
		StageDir:   stageDir,
		SourcePath: sourcePath,
	}
	pendingPath := filepath.Join(root, "pending.json")
	data, err := json.Marshal(staged)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pendingPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	restarted := &Manager{root: root, pendingPath: pendingPath}
	if err := restarted.loadPending(); err != nil {
		t.Fatalf("restore pending update after restart: %v", err)
	}
	pending := restarted.Pending("v1.0.0")
	if pending == nil || !pending.Downloaded || pending.LatestVersion != staged.Version {
		t.Fatalf("restored pending update = %+v", pending)
	}
	if restarted.staged == nil || restarted.staged.SourcePath != sourcePath {
		t.Fatalf("restored payload path = %+v, want %q", restarted.staged, sourcePath)
	}
}

func TestCheckAndDownloadVerifiesAndPersistsUpdate(t *testing.T) {
	manager, assetName, requests := newTestManager(t, "")
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
	assertUpdateDownloadRequests(t, requests, assetName)
}

func TestCheckAndDownloadSelectsVersionedAssetName(t *testing.T) {
	assetName, err := assetNameForPlatform()
	if err != nil {
		t.Skip(err)
	}
	if strings.HasSuffix(assetName, ".tar.gz") {
		assetName = strings.TrimSuffix(assetName, ".tar.gz") + "-v1.1.0.tar.gz"
	} else {
		assetName = strings.TrimSuffix(assetName, ".zip") + "-v1.1.0.zip"
	}
	manager, assetName, requests := newTestManagerWithAssetName(t, "", assetName)
	check, err := manager.Check(context.Background(), "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !check.Available {
		t.Fatalf("versioned release asset was not selected: %+v", check)
	}
	if _, err := manager.Download(context.Background()); err != nil {
		t.Fatalf("download versioned release asset: %v", err)
	}
	if manager.staged == nil || manager.staged.AssetName != assetName {
		t.Fatalf("staged asset = %+v, want %q", manager.staged, assetName)
	}
	assertUpdateDownloadRequests(t, requests, assetName)
}

func TestDownloadRejectsChecksumMismatch(t *testing.T) {
	manager, assetName, requests := newTestManager(t, strings.Repeat("0", 64))
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
	entries, err := os.ReadDir(manager.root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("checksum mismatch left staged files behind: %v", entries)
	}
	assertUpdateDownloadRequests(t, requests, assetName)
}

func TestDownloadCleansStageWhenPendingPersistenceFails(t *testing.T) {
	manager, _, _ := newTestManager(t, "")
	if _, err := manager.Check(context.Background(), "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	manager.pendingPath = filepath.Join(manager.root, "missing", "pending.json")
	if _, err := manager.Download(context.Background()); err == nil {
		t.Fatal("expected pending metadata persistence to fail")
	}
	entries, err := os.ReadDir(manager.root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed persistence left staged files behind: %v", entries)
	}
}

func TestDownloadRejectsNonGitHubURLsBeforeStaging(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Manager)
	}{
		{
			name: "package URL",
			edit: func(manager *Manager) { manager.asset.BrowserDownloadURL = "https://github.example.com/package.zip" },
		},
		{
			name: "checksum URL",
			edit: func(manager *Manager) { manager.checksum.BrowserDownloadURL = "https://example.com/SHA256SUMS" },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, _, _ := newTestManager(t, "")
			if _, err := manager.Check(context.Background(), "v1.0.0"); err != nil {
				t.Fatal(err)
			}
			test.edit(manager)
			if _, err := manager.Download(context.Background()); err == nil {
				t.Fatal("expected an untrusted asset URL to be rejected")
			}
			entries, err := os.ReadDir(manager.root)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("rejected URL created staged files: %v", entries)
			}
		})
	}
}

func TestDownloadRejectsOversizedAssetBeforeStaging(t *testing.T) {
	manager, _, _ := newTestManager(t, "")
	if _, err := manager.Check(context.Background(), "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	manager.asset.Size = maxAssetSize + 1
	if _, err := manager.Download(context.Background()); err == nil {
		t.Fatal("expected an oversized update asset to be rejected")
	}
	entries, err := os.ReadDir(manager.root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("oversized asset created staged files: %v", entries)
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

func newTestManager(t *testing.T, checksumOverride string) (*Manager, string, map[string]int) {
	t.Helper()
	assetName, err := assetNameForPlatform()
	if err != nil {
		t.Skip(err)
	}
	return newTestManagerWithAssetName(t, checksumOverride, assetName)
}

func newTestManagerWithAssetName(t *testing.T, checksumOverride, assetName string) (*Manager, string, map[string]int) {
	t.Helper()
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
	requests := make(map[string]int)
	apiTransport := updaterRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != latestReleaseURL {
			return nil, fmt.Errorf("unexpected GitHub API request URL %q", request.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(releaseJSON)),
			Request:    request,
		}, nil
	})
	downloadTransport := updaterRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests[request.URL.String()]++
		var body []byte
		switch request.URL.String() {
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
	apiClient := &http.Client{Transport: apiTransport}
	downloadClient := &http.Client{Transport: downloadTransport}
	return &Manager{
		root:        root,
		pendingPath: filepath.Join(root, "pending.json"),
		apiClient:   apiClient,
		dlClient:    downloadClient,
	}, assetName, requests
}

func assertUpdateDownloadRequests(t *testing.T, requests map[string]int, assetName string) {
	t.Helper()
	checksumURL := "https://github.com/" + repository + "/releases/download/v1.1.0/SHA256SUMS"
	assetURL := "https://github.com/" + repository + "/releases/download/v1.1.0/" + assetName
	if requests[checksumURL] != 1 || requests[assetURL] != 1 || len(requests) != 2 {
		t.Fatalf("updater download client requests = %#v, want one checksum and one package request", requests)
	}
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

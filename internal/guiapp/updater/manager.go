package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	repository       = "winezer0/netgap"
	latestReleaseURL = "https://api.github.com/repos/" + repository + "/releases/latest"
	releasesPageURL  = "https://github.com/" + repository + "/releases/latest"
	maxAssetSize     = 512 << 20
)

var versionPattern = regexp.MustCompile(`^[vV]?(\d+)\.(\d+)\.(\d+)([-+].*)?$`)
var gitDescribeVersionPattern = regexp.MustCompile(`^[vV]?\d+\.\d+\.\d+-\d+-g[0-9a-fA-F]+(-dirty)?$`)

type CheckResult struct {
	Available      bool
	CurrentVersion string
	LatestVersion  string
	ReleaseURL     string
}

type PendingUpdate struct {
	Available     bool
	Downloaded    bool
	LatestVersion string
	ReleaseURL    string
}

type release struct {
	TagName    string         `json:"tag_name"`
	HTMLURL    string         `json:"html_url"`
	Prerelease bool           `json:"prerelease"`
	Assets     []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type stagedUpdate struct {
	Version    string `json:"version"`
	ReleaseURL string `json:"releaseUrl"`
	AssetName  string `json:"assetName"`
	StageDir   string `json:"stageDir"`
	SourcePath string `json:"sourcePath"`
}

type Manager struct {
	root        string
	pendingPath string
	apiClient   *http.Client
	dlClient    *http.Client
	mu          sync.Mutex
	available   *release
	asset       *releaseAsset
	checksum    *releaseAsset
	staged      *stagedUpdate
}

func NewManager() (*Manager, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(cacheDir, "netgapgui", "updates")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	m := &Manager{
		root:        root,
		pendingPath: filepath.Join(root, "pending.json"),
		apiClient:   &http.Client{Timeout: 20 * time.Second},
		dlClient:    &http.Client{Timeout: 10 * time.Minute},
	}
	if err := m.loadPending(); err != nil {
		_ = os.Remove(m.pendingPath)
	}
	return m, nil
}

func (m *Manager) Check(ctx context.Context, currentVersion string) (*CheckResult, error) {
	currentVersion = strings.TrimSpace(currentVersion)
	result := &CheckResult{CurrentVersion: currentVersion, ReleaseURL: releasesPageURL}
	if !validVersion(currentVersion) || isDevelopmentVersion(currentVersion) {
		return result, nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "netgapgui-update-checker")
	response, err := m.apiClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("check GitHub Releases: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("GitHub Releases returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	var latest release
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&latest); err != nil {
		return nil, fmt.Errorf("decode GitHub release: %w", err)
	}
	result.ReleaseURL = latest.HTMLURL
	result.LatestVersion = latest.TagName
	if latest.Prerelease {
		return nil, errors.New("latest GitHub release is marked as a prerelease")
	}
	newer, err := compareVersions(latest.TagName, currentVersion)
	if err != nil {
		return nil, err
	}
	if newer <= 0 {
		m.clearAvailable()
		return result, nil
	}

	assetName, err := assetNameForPlatform()
	if err != nil {
		return nil, err
	}
	var packageAsset, checksumAsset *releaseAsset
	for i := range latest.Assets {
		asset := &latest.Assets[i]
		switch asset.Name {
		case assetName:
			packageAsset = asset
		case "SHA256SUMS":
			checksumAsset = asset
		}
	}
	if packageAsset == nil {
		return nil, fmt.Errorf("release %s has no update package for %s/%s", latest.TagName, runtime.GOOS, runtime.GOARCH)
	}
	if checksumAsset == nil {
		return nil, fmt.Errorf("release %s does not contain SHA256SUMS", latest.TagName)
	}

	m.mu.Lock()
	m.available = &latest
	m.asset = packageAsset
	m.checksum = checksumAsset
	m.mu.Unlock()
	result.Available = true
	return result, nil
}

func (m *Manager) Download(ctx context.Context) (*PendingUpdate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.staged != nil {
		if m.available == nil {
			return pendingInfo(m.staged), nil
		}
		newer, err := compareVersions(m.available.TagName, m.staged.Version)
		if err != nil || newer <= 0 {
			return pendingInfo(m.staged), nil
		}
	}
	if m.available == nil || m.asset == nil || m.checksum == nil {
		return nil, errors.New("check for updates before downloading")
	}
	if m.asset.Size < 0 || m.asset.Size > maxAssetSize {
		return nil, fmt.Errorf("update package size is invalid: %d", m.asset.Size)
	}

	stageDir, err := os.MkdirTemp(m.root, "stage-")
	if err != nil {
		return nil, err
	}
	keepStage := false
	defer func() {
		if !keepStage {
			_ = os.RemoveAll(stageDir)
		}
	}()

	manifest, err := m.downloadBytes(ctx, m.checksum.BrowserDownloadURL, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("download update checksum: %w", err)
	}
	expectedHash, err := checksumForAsset(manifest, m.asset.Name)
	if err != nil {
		return nil, err
	}
	archivePath := filepath.Join(stageDir, m.asset.Name)
	if err := m.downloadFile(ctx, m.asset.BrowserDownloadURL, archivePath, m.asset.Size, expectedHash); err != nil {
		return nil, fmt.Errorf("download update package: %w", err)
	}
	extractDir := filepath.Join(stageDir, "unpacked")
	if err := os.Mkdir(extractDir, 0o755); err != nil {
		return nil, err
	}
	sourcePath, err := extractPayload(archivePath, extractDir)
	if err != nil {
		return nil, err
	}
	if err := os.Remove(archivePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	staged := &stagedUpdate{
		Version:    m.available.TagName,
		ReleaseURL: m.available.HTMLURL,
		AssetName:  m.asset.Name,
		StageDir:   stageDir,
		SourcePath: sourcePath,
	}
	oldStaged := m.staged
	if err := m.persistPending(staged); err != nil {
		return nil, err
	}
	m.staged = staged
	keepStage = true
	if oldStaged != nil {
		_ = os.RemoveAll(oldStaged.StageDir)
	}
	return pendingInfo(staged), nil
}

func (m *Manager) Pending(currentVersion string) *PendingUpdate {
	if !validVersion(currentVersion) || isDevelopmentVersion(currentVersion) {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.staged == nil {
		return nil
	}
	newer, err := compareVersions(m.staged.Version, currentVersion)
	if err != nil || newer <= 0 {
		m.clearStagedLocked()
		return nil
	}
	if _, err := os.Stat(m.staged.SourcePath); err != nil {
		m.clearStagedLocked()
		return nil
	}
	return pendingInfo(m.staged)
}

func (m *Manager) Install() error {
	m.mu.Lock()
	staged := m.staged
	m.mu.Unlock()
	if staged == nil {
		return errors.New("there is no downloaded update to install")
	}
	if _, err := os.Stat(staged.SourcePath); err != nil {
		return fmt.Errorf("downloaded update is unavailable: %w", err)
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return fmt.Errorf("automatic updates are not supported on %s", runtime.GOOS)
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(executable); resolveErr == nil {
		executable = resolved
	}
	target := executable
	kind := "binary"
	if runtime.GOOS == "darwin" {
		target, err = appBundlePath(executable)
		if err != nil {
			return err
		}
		kind = "app"
	}
	if err := checkWritableDirectory(filepath.Dir(target)); err != nil {
		return fmt.Errorf("application install directory is not writable: %w", err)
	}
	return startPlatformInstaller(staged.SourcePath, target, kind, staged.StageDir, m.pendingPath, os.Getpid())
}

func (m *Manager) clearAvailable() {
	m.mu.Lock()
	m.available = nil
	m.asset = nil
	m.checksum = nil
	m.mu.Unlock()
}

func (m *Manager) clearStagedLocked() {
	if m.staged != nil {
		_ = os.RemoveAll(m.staged.StageDir)
		m.staged = nil
	}
	_ = os.Remove(m.pendingPath)
}

func (m *Manager) loadPending() error {
	data, err := os.ReadFile(m.pendingPath)
	if err != nil {
		return err
	}
	var staged stagedUpdate
	if err := json.Unmarshal(data, &staged); err != nil {
		return err
	}
	if staged.Version == "" || !validVersion(staged.Version) || !withinRoot(m.root, staged.StageDir) || !withinRoot(m.root, staged.SourcePath) {
		return errors.New("invalid pending update metadata")
	}
	if _, err := os.Stat(staged.SourcePath); err != nil {
		return err
	}
	m.staged = &staged
	return nil
}

func (m *Manager) persistPending(staged *stagedUpdate) error {
	data, err := json.Marshal(staged)
	if err != nil {
		return err
	}
	tempPath := m.pendingPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o600); err != nil {
		return err
	}
	backupPath := m.pendingPath + ".backup"
	_ = os.Remove(backupPath)
	if _, err := os.Stat(m.pendingPath); err == nil {
		if err := os.Rename(m.pendingPath, backupPath); err != nil {
			_ = os.Remove(tempPath)
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, m.pendingPath); err != nil {
		_ = os.Remove(tempPath)
		_ = os.Rename(backupPath, m.pendingPath)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func (m *Manager) downloadBytes(ctx context.Context, downloadURL string, limit int64) ([]byte, error) {
	request, err := downloadRequest(ctx, downloadURL)
	if err != nil {
		return nil, err
	}
	response, err := m.dlClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("download exceeds size limit")
	}
	return data, nil
}

func (m *Manager) downloadFile(ctx context.Context, downloadURL, destination string, expectedSize int64, expectedHash string) error {
	request, err := downloadRequest(ctx, downloadURL)
	if err != nil {
		return err
	}
	response, err := m.dlClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %s", response.Status)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	limited := &io.LimitedReader{R: response.Body, N: maxAssetSize + 1}
	written, copyErr := io.Copy(io.MultiWriter(file, hash), limited)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(destination)
		return closeErr
	}
	if written > maxAssetSize {
		_ = os.Remove(destination)
		return errors.New("update package exceeds size limit")
	}
	if expectedSize > 0 && written != expectedSize {
		_ = os.Remove(destination)
		return fmt.Errorf("update package size mismatch: expected %d bytes, received %d", expectedSize, written)
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expectedHash) {
		_ = os.Remove(destination)
		return errors.New("update package checksum does not match SHA256SUMS")
	}
	return nil
}

func downloadRequest(ctx context.Context, rawURL string) (*http.Request, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "https" || parsed.Host != "github.com" {
		return nil, fmt.Errorf("unexpected GitHub release asset URL: %s", rawURL)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "netgapgui-update-checker")
	request.Header.Set("Accept", "application/octet-stream")
	return request, nil
}

func checksumForAsset(manifest []byte, name string) (string, error) {
	for _, line := range strings.Split(string(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		fileName := strings.TrimPrefix(fields[1], "*")
		if filepath.Base(fileName) == name {
			if len(fields[0]) != sha256.Size*2 {
				return "", errors.New("invalid SHA256SUMS entry")
			}
			if _, err := hex.DecodeString(fields[0]); err != nil {
				return "", errors.New("invalid SHA256SUMS hash")
			}
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("SHA256SUMS has no entry for %s", name)
}

func assetNameForPlatform() (string, error) {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "amd64" {
			return "netgapgui-windows-amd64.zip", nil
		}
	case "linux":
		if runtime.GOARCH == "amd64" {
			return "netgapgui-linux-amd64.tar.gz", nil
		}
	case "darwin":
		if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
			return "netgapgui-macos-universal.zip", nil
		}
	}
	return "", fmt.Errorf("automatic updates are not available for %s/%s", runtime.GOOS, runtime.GOARCH)
}

func validVersion(version string) bool {
	return versionPattern.MatchString(strings.TrimSpace(version))
}

func isDevelopmentVersion(version string) bool {
	version = strings.TrimSpace(version)
	return gitDescribeVersionPattern.MatchString(version) || strings.HasSuffix(strings.ToLower(version), "-dirty")
}

func compareVersions(left, right string) (int, error) {
	leftParts := versionPattern.FindStringSubmatch(strings.TrimSpace(left))
	rightParts := versionPattern.FindStringSubmatch(strings.TrimSpace(right))
	if leftParts == nil || rightParts == nil {
		return 0, fmt.Errorf("invalid release version: %q or %q", left, right)
	}
	for i := 1; i <= 3; i++ {
		leftNumber := strings.TrimLeft(leftParts[i], "0")
		rightNumber := strings.TrimLeft(rightParts[i], "0")
		if leftNumber == "" {
			leftNumber = "0"
		}
		if rightNumber == "" {
			rightNumber = "0"
		}
		if len(leftNumber) > len(rightNumber) {
			return 1, nil
		}
		if len(leftNumber) < len(rightNumber) {
			return -1, nil
		}
		if leftNumber > rightNumber {
			return 1, nil
		}
		if leftNumber < rightNumber {
			return -1, nil
		}
	}
	leftSuffix := leftParts[4]
	rightSuffix := rightParts[4]
	leftPrerelease := strings.HasPrefix(leftSuffix, "-")
	rightPrerelease := strings.HasPrefix(rightSuffix, "-")
	if leftPrerelease && !rightPrerelease {
		return -1, nil
	}
	if !leftPrerelease && rightPrerelease {
		return 1, nil
	}
	if leftPrerelease && rightPrerelease {
		if leftSuffix > rightSuffix {
			return 1, nil
		}
		if leftSuffix < rightSuffix {
			return -1, nil
		}
	}
	return 0, nil
}

func pendingInfo(staged *stagedUpdate) *PendingUpdate {
	return &PendingUpdate{
		Available:     true,
		Downloaded:    true,
		LatestVersion: staged.Version,
		ReleaseURL:    staged.ReleaseURL,
	}
}

func withinRoot(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil || rel == "." || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func checkWritableDirectory(directory string) error {
	probe, err := os.CreateTemp(directory, ".netgapgui-update-check-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

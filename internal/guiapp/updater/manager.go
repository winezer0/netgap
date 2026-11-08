package updater

import (
	"context"
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

	"github.com/winezer0/dezip"
	"github.com/winezer0/downunpack"
	"github.com/winezer0/downutils"
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
	root         string
	pendingPath  string
	apiClient    *http.Client
	dlClient     *http.Client
	mu           sync.Mutex
	available    *release
	asset        *releaseAsset
	checksum     *releaseAsset
	assetPattern string
	staged       *stagedUpdate
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

	assetPattern, err := assetPatternForPlatform()
	if err != nil {
		return nil, err
	}
	var packageAsset, checksumAsset *releaseAsset
	sources := make([]downloadunpack.Source, 0, len(latest.Assets))
	for i := range latest.Assets {
		asset := &latest.Assets[i]
		if asset.Name == "SHA256SUMS" {
			if checksumAsset != nil {
				return nil, fmt.Errorf("release %s contains multiple SHA256SUMS assets", latest.TagName)
			}
			checksumAsset = asset
			continue
		}
		sources = append(sources, downloadunpack.Source{Name: asset.Name, URL: asset.BrowserDownloadURL, Size: asset.Size})
	}
	selected, err := downloadunpack.SelectSource(sources, assetPattern)
	if err != nil {
		if errors.Is(err, downloadunpack.ErrSourceNotFound) {
			return nil, fmt.Errorf("release %s has no update package for %s/%s", latest.TagName, runtime.GOOS, runtime.GOARCH)
		}
		return nil, fmt.Errorf("select update package from release %s: %w", latest.TagName, err)
	}
	for i := range latest.Assets {
		asset := &latest.Assets[i]
		if asset.Name == selected.Name && asset.BrowserDownloadURL == selected.URL {
			packageAsset = asset
			break
		}
	}
	if checksumAsset == nil {
		return nil, fmt.Errorf("release %s does not contain SHA256SUMS", latest.TagName)
	}

	m.mu.Lock()
	m.available = &latest
	m.asset = packageAsset
	m.checksum = checksumAsset
	m.assetPattern = assetPattern
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
	if err := validateGitHubDownloadURL(m.checksum.BrowserDownloadURL); err != nil {
		return nil, fmt.Errorf("invalid update checksum URL: %w", err)
	}
	if err := validateGitHubDownloadURL(m.asset.BrowserDownloadURL); err != nil {
		return nil, fmt.Errorf("invalid update package URL: %w", err)
	}
	assetPattern := m.assetPattern
	if assetPattern == "" {
		return nil, errors.New("update package selection is unavailable; check for updates again")
	}
	payloadPattern, err := payloadPatternForPlatform()
	if err != nil {
		return nil, err
	}
	sources := make([]downloadunpack.Source, 0, len(m.available.Assets))
	for i := range m.available.Assets {
		asset := &m.available.Assets[i]
		sources = append(sources, downloadunpack.Source{
			Name: asset.Name,
			URL:  asset.BrowserDownloadURL,
			Size: asset.Size,
		})
	}
	selected, err := downloadunpack.SelectSource(sources, assetPattern)
	if err != nil {
		return nil, fmt.Errorf("select update package: %w", err)
	}
	if selected.Size < 0 || selected.Size > maxAssetSize {
		return nil, fmt.Errorf("update package size is invalid: %d", selected.Size)
	}
	if err := validateGitHubDownloadURL(selected.URL); err != nil {
		return nil, fmt.Errorf("invalid update package URL: %w", err)
	}
	for i := range sources {
		if sources[i].Name == selected.Name && sources[i].URL == selected.URL {
			sources[i].Checksum = "sha256:" + m.checksum.BrowserDownloadURL
		}
	}
	options := downutils.NewDefaultDownOptions()
	options.ShowProgress = false
	options.MaxRetries = 1
	prepared, err := downloadunpack.Prepare(ctx, downloadunpack.ResourceRequest{
		Sources:          sources,
		SourcePattern:    assetPattern,
		ExtractAs:        string(downloadunpack.ModeAuto),
		PayloadPattern:   payloadPattern,
		StageParent:      m.root,
		MaxDownloadBytes: maxAssetSize,
		DownloadOptions:  options,
		HTTPClient:       m.dlClient,
		ExtractOptions: dezip.Options{
			MaxEntries:       maxUpdateArchiveEntries,
			MaxExpandedBytes: maxUpdateArchiveExpandedBytes,
			AllowSymlinks:    true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("prepare update package: %w", err)
	}
	keepStage := false
	defer func() {
		if !keepStage {
			_ = prepared.Cleanup()
		}
	}()
	if prepared.Source.Name != m.asset.Name || prepared.Source.URL != m.asset.BrowserDownloadURL {
		return nil, errors.New("prepared update package differs from the checked release asset")
	}
	if err := os.Remove(prepared.Download.StoragePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	staged := &stagedUpdate{
		Version:    m.available.TagName,
		ReleaseURL: m.available.HTMLURL,
		AssetName:  prepared.Source.Name,
		StageDir:   prepared.StageDir,
		SourcePath: prepared.PayloadPath,
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
	m.assetPattern = ""
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

func validateGitHubDownloadURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" || parsed.Host != "github.com" {
		return fmt.Errorf("unexpected GitHub release asset URL: %s", rawURL)
	}
	return nil
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

func assetPatternForPlatform() (string, error) {
	return assetPatternFor(runtime.GOOS, runtime.GOARCH)
}

func assetPatternFor(goos, goarch string) (string, error) {
	switch goos {
	case "windows":
		if goarch == "amd64" {
			return "netgapgui-windows-amd64*.zip", nil
		}
	case "linux":
		if goarch == "amd64" {
			return "netgapgui-linux-amd64*.tar.gz", nil
		}
	case "darwin":
		if goarch == "amd64" || goarch == "arm64" {
			return "netgapgui-macos-universal*.zip", nil
		}
	}
	return "", fmt.Errorf("automatic updates are not available for %s/%s", goos, goarch)
}

func payloadPatternForPlatform() (string, error) {
	return payloadPatternFor(runtime.GOOS)
}

func payloadPatternFor(goos string) (string, error) {
	switch goos {
	case "windows":
		return "netgapgui.exe", nil
	case "linux":
		return "netgapgui", nil
	case "darwin":
		return "netgapgui.app", nil
	default:
		return "", fmt.Errorf("automatic updates are not available for %s", goos)
	}
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
	rootResolved, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return false
	}
	pathResolved, err := filepath.EvalSymlinks(pathAbs)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootResolved, pathResolved)
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

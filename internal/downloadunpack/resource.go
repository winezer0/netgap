package downloadunpack

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/winezer0/dezip"
	"github.com/winezer0/downutils"
)

var (
	ErrSourceNotFound      = errors.New("download source not found")
	ErrSourceAmbiguous     = errors.New("download source is ambiguous")
	ErrPayloadNotFound     = errors.New("payload not found")
	ErrPayloadAmbiguous    = errors.New("payload is ambiguous")
	ErrEntrypointNotFound  = errors.New("entrypoint not found")
	ErrEntrypointAmbiguous = errors.New("entrypoint is ambiguous")
)

// Source describes one remote resource candidate. Size is optional; when set,
// it is checked by downutils before the downloaded file is accepted.
type Source struct {
	Name     string
	URL      string
	Checksum string
	Size     int64
}

// ResourceRequest describes a source selection and its staged processing.
// Glob patterns use slash-separated paths; ** matches zero or more directories.
type ResourceRequest struct {
	Sources           []Source
	SourcePattern     string
	ExtractAs         string
	PayloadPattern    string
	EntrypointPattern string
	StageParent       string
	MaxDownloadBytes  int64
	DownloadOptions   *downutils.DownOptions
	HTTPClient        *http.Client
	ExtractOptions    dezip.Options
}

// ResourceResult contains paths inside a private staging directory. The caller
// owns the staged files after success and should call Cleanup when finished.
type ResourceResult struct {
	StageDir    string
	PayloadPath string
	EntryPoint  string
	Source      Source
	Download    downutils.TaskInfo
	Extracted   bool
	Extraction  dezip.Result

	cleanupPath string
}

// Cleanup removes the staging directory created by Prepare. It is safe to call
// more than once.
func (result *ResourceResult) Cleanup() error {
	if result == nil || result.cleanupPath == "" {
		return nil
	}
	return os.RemoveAll(result.cleanupPath)
}

// SelectSource matches source names without performing network or filesystem
// operations. It uses the same exact-one rule as Prepare.
func SelectSource(sources []Source, pattern string) (Source, error) {
	if len(sources) == 0 {
		return Source{}, ErrSourceNotFound
	}
	normalized, err := normalizeSourcePattern(pattern)
	if err != nil {
		return Source{}, fmt.Errorf("invalid source pattern: %w", err)
	}
	matches := make([]Source, 0, 2)
	for _, candidate := range sources {
		if normalized == "" {
			matches = append(matches, candidate)
			continue
		}
		matched, err := matchAssetPattern(normalized, candidate.Name)
		if err != nil {
			return Source{}, err
		}
		if matched {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return Source{}, fmt.Errorf("%w: %q", ErrSourceNotFound, pattern)
	}
	if len(matches) > 1 {
		return Source{}, fmt.Errorf("%w: pattern %q matched %d assets", ErrSourceAmbiguous, pattern, len(matches))
	}
	return matches[0], nil
}

// Prepare selects one candidate, downloads and verifies it, optionally
// extracts it, and resolves the requested payload. It never changes an
// installation path. A successful result keeps its staging directory until
// the caller calls Cleanup.
func Prepare(ctx context.Context, request ResourceRequest) (*ResourceResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	source, mode, payloadPattern, entrypointPattern, err := validateResourceRequest(request)
	if err != nil {
		return nil, err
	}

	stageParent := request.StageParent
	if stageParent == "" {
		stageParent = os.TempDir()
	}
	stageParent, err = absoluteCleanPath(stageParent)
	if err != nil {
		return nil, fmt.Errorf("resolve staging parent: %w", err)
	}
	if err := os.MkdirAll(stageParent, 0o755); err != nil {
		return nil, fmt.Errorf("create staging parent: %w", err)
	}
	stageDir, err := os.MkdirTemp(stageParent, ".downloadunpack-*")
	if err != nil {
		return nil, fmt.Errorf("create staging directory: %w", err)
	}
	keepStage := false
	defer func() {
		if !keepStage {
			_ = os.RemoveAll(stageDir)
		}
	}()

	downloadDir := filepath.Join(stageDir, "download")
	if err := os.Mkdir(downloadDir, 0o700); err != nil {
		return nil, fmt.Errorf("create download directory: %w", err)
	}

	options := copyDownloadOptions(request.DownloadOptions)
	client := request.HTTPClient
	ownedClient := false
	if client == nil {
		client, err = downutils.CreateHTTPClient(&downutils.HttpClientConfig{
			ConnectTimeout: options.ConnectTimeout,
			IdleTimeout:    options.IdleTimeout,
			ProxyURL:       options.ProxyURL,
		})
		if err != nil {
			return nil, fmt.Errorf("create download HTTP client: %w", err)
		}
		ownedClient = true
	}
	if ownedClient {
		defer client.CloseIdleConnections()
	}

	maxSize := request.MaxDownloadBytes
	if source.Size > 0 && (maxSize == 0 || source.Size < maxSize) {
		maxSize = source.Size
	}
	checkSize := ""
	if source.Size > 0 {
		checkSize = fmt.Sprintf("%dB", source.Size)
	}
	item := downutils.DownItem{
		Module:       "downloadunpack",
		FileName:     source.Name,
		DownloadURLs: []string{source.URL},
		KeepUpdated:  "enable",
		StorageDir:   downloadDir,
		Enable:       true,
		Checksum:     source.Checksum,
		CheckSize:    checkSize,
		MaxSize:      maxSize,
	}
	downloadResult, err := downutils.ExecuteDownloadsWithClientContext(ctx, options, downutils.DownConfig{
		"downloadunpack": {item},
	}, client)
	if err != nil {
		return nil, fmt.Errorf("download resource: %w", err)
	}
	if downloadResult == nil || len(downloadResult.TaskInfos) != 1 {
		return nil, errors.New("download resource returned no task result")
	}
	task := downloadResult.TaskInfos[0]
	if task.Status != downutils.TaskStatusSuccess {
		message := strings.TrimSpace(task.ErrorMsg)
		if message == "" {
			message = string(task.Status)
		}
		return nil, fmt.Errorf("download resource did not complete successfully: %s", message)
	}
	downloadPath, err := validateDownloadedPath(stageDir, downloadDir, source.Name, task.StoragePath)
	if err != nil {
		return nil, err
	}

	result := &ResourceResult{
		StageDir:    stageDir,
		PayloadPath: downloadPath,
		Source:      source,
		Download:    task,
		cleanupPath: stageDir,
	}
	if mode == ModeSkip {
		keepStage = true
		return result, nil
	}

	extractDir := filepath.Join(stageDir, "extracted")
	extraction, _, skipped, err := processDownloadedFile(
		ctx, downloadPath, extractDir, ModeAuto, request.ExtractOptions, "",
	)
	if err != nil {
		return nil, fmt.Errorf("extract resource: %w", err)
	}
	if skipped {
		if payloadPattern != "" || entrypointPattern != "" {
			return nil, errors.New("payload and entrypoint patterns require an extracted resource")
		}
		keepStage = true
		return result, nil
	}

	result.Extracted = true
	result.Extraction = extraction
	result.PayloadPath, err = resolvePayload(ctx, extraction.OutputDir, payloadPattern)
	if err != nil {
		return nil, err
	}
	if entrypointPattern != "" {
		result.EntryPoint, err = resolveEntrypoint(ctx, result.PayloadPath, entrypointPattern)
		if err != nil {
			return nil, err
		}
	}
	keepStage = true
	return result, nil
}

func validateResourceRequest(request ResourceRequest) (Source, Mode, string, string, error) {
	mode := ModeAuto
	switch strings.ToLower(strings.TrimSpace(request.ExtractAs)) {
	case "", string(ModeAuto):
	case string(ModeSkip):
		mode = ModeSkip
	default:
		return Source{}, "", "", "", fmt.Errorf("%w: %q", ErrInvalidMode, request.ExtractAs)
	}
	if request.MaxDownloadBytes < 0 {
		return Source{}, "", "", "", errors.New("maximum download size must not be negative")
	}

	payloadPattern, err := normalizeGlobPattern(request.PayloadPattern, true)
	if err != nil {
		return Source{}, "", "", "", fmt.Errorf("invalid payload pattern: %w", err)
	}
	entrypointPattern, err := normalizeGlobPattern(request.EntrypointPattern, false)
	if err != nil {
		return Source{}, "", "", "", fmt.Errorf("invalid entrypoint pattern: %w", err)
	}
	if mode == ModeSkip && (payloadPattern != "" || entrypointPattern != "") {
		return Source{}, "", "", "", errors.New("payload and entrypoint patterns cannot be used when extraction is skipped")
	}

	source, err := SelectSource(request.Sources, request.SourcePattern)
	if err != nil {
		return Source{}, "", "", "", err
	}
	if err := validateSource(source, request.MaxDownloadBytes); err != nil {
		return Source{}, "", "", "", err
	}
	return source, mode, payloadPattern, entrypointPattern, nil
}

func validateSource(source Source, maxDownloadBytes int64) error {
	name := source.Name
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\\:`) || strings.ContainsRune(name, '\x00') || hasWindowsDrivePrefix(name) {
		return fmt.Errorf("invalid source file name %q", name)
	}
	if source.Size < 0 {
		return fmt.Errorf("source %q has a negative declared size", name)
	}
	if maxDownloadBytes > 0 && source.Size > maxDownloadBytes {
		return fmt.Errorf("source %q exceeds maximum download size: %d > %d", name, source.Size, maxDownloadBytes)
	}
	parsed, err := url.Parse(source.URL)
	if err != nil || (!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) || parsed.Hostname() == "" {
		return fmt.Errorf("source %q has an invalid HTTP(S) URL", name)
	}
	return nil
}

func copyDownloadOptions(input *downutils.DownOptions) *downutils.DownOptions {
	var options *downutils.DownOptions
	if input == nil {
		options = downutils.NewDefaultDownOptions()
		options.ShowProgress = false
	} else {
		copy := *input
		options = &copy
	}
	// The package owns the staging path; an OutputForce override could escape it.
	options.OutputForce = ""
	return options
}

func validateDownloadedPath(stageDir, downloadDir, name, downloadedPath string) (string, error) {
	if downloadedPath == "" {
		return "", errors.New("downloader returned an empty storage path")
	}
	actual, err := filepath.Abs(downloadedPath)
	if err != nil {
		return "", fmt.Errorf("resolve downloaded path: %w", err)
	}
	expected := filepath.Join(downloadDir, name)
	if !samePath(actual, expected) || !pathContains(stageDir, actual) {
		return "", fmt.Errorf("downloader returned a path outside the staging directory: %q", actual)
	}
	info, err := os.Lstat(actual)
	if err != nil {
		return "", fmt.Errorf("inspect downloaded resource: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("downloaded resource is not a regular file")
	}
	return actual, nil
}

func samePath(first, second string) bool {
	first = filepath.Clean(first)
	second = filepath.Clean(second)
	if runtimePathCaseInsensitive() {
		return strings.EqualFold(first, second)
	}
	return first == second
}

func runtimePathCaseInsensitive() bool {
	return filepath.Separator == '\\'
}

func resolvePayload(ctx context.Context, root, pattern string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve extraction root: %w", err)
	}
	if pattern == "" || pattern == "." {
		return root, nil
	}
	matches, err := findResourceMatches(ctx, root, pattern, false)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("%w: pattern %q", ErrPayloadNotFound, pattern)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("%w: pattern %q matched multiple files or directories", ErrPayloadAmbiguous, pattern)
	}
	return matches[0], nil
}

func resolveEntrypoint(ctx context.Context, payloadPath, pattern string) (string, error) {
	info, err := os.Lstat(payloadPath)
	if err != nil {
		return "", fmt.Errorf("inspect payload: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("payload cannot be a symbolic link")
	}
	if info.Mode().IsRegular() {
		matched, err := matchRelativeGlob(pattern, filepath.Base(payloadPath))
		if err != nil {
			return "", err
		}
		if matched {
			return payloadPath, nil
		}
		return "", fmt.Errorf("%w: pattern %q did not match payload file", ErrEntrypointNotFound, pattern)
	}
	if !info.IsDir() {
		return "", errors.New("payload is neither a regular file nor a directory")
	}
	matches, err := findResourceMatches(ctx, payloadPath, pattern, true)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("%w: pattern %q", ErrEntrypointNotFound, pattern)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("%w: pattern %q matched multiple files", ErrEntrypointAmbiguous, pattern)
	}
	return matches[0], nil
}

func findResourceMatches(ctx context.Context, root, pattern string, filesOnly bool) ([]string, error) {
	matches := make([]string, 0, 2)
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if current == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if filesOnly && !info.Mode().IsRegular() {
			return nil
		}
		if !filesOnly && !info.Mode().IsRegular() && !info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		matched, err := matchRelativeGlob(pattern, filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		if matched {
			matches = append(matches, current)
			if len(matches) > 1 {
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan staged payload: %w", err)
	}
	return matches, nil
}

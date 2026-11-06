package downloadunpack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/winezer0/dezip"
	"github.com/winezer0/downutils"
)

// Run downloads one item and processes it only after downutils reports success.
// An empty ExtractAs or "skip" keeps the downloaded file as-is.
func Run(ctx context.Context, request Request) (Result, error) {
	mode, extractOptions, expectedFile, err := validateRequest(request)
	if err != nil {
		return Result{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	options := request.DownloadOptions
	if options == nil {
		options = downutils.NewDefaultDownOptions()
	} else {
		copy := *options
		options = &copy
	}
	item := request.Item
	item.Enable = true

	download, err := downutils.ExecuteDownloadsContext(ctx, options, downutils.DownConfig{
		"downloadunpack": {item},
	})
	if err != nil {
		return Result{}, fmt.Errorf("download: %w", err)
	}
	if download == nil || len(download.TaskInfos) != 1 {
		return Result{}, errors.New("download returned no task result")
	}

	task := download.TaskInfos[0]
	result := Result{
		Mode:        mode,
		Download:    task,
		ArchivePath: task.StoragePath,
	}
	if task.Status == downutils.TaskStatusSkipped {
		result.DownloadSkipped = true
		if mode == ModeSkip {
			result.Skipped = true
			result.OutputPath = task.StoragePath
			result.MainFile = task.StoragePath
			return result, nil
		}
		return result, fmt.Errorf("%w: existing download was skipped; set KeepUpdated or UpdateForce to require a fresh verified download", ErrDownloadNotCompleted)
	}
	if task.Status != downutils.TaskStatusSuccess {
		message := strings.TrimSpace(task.ErrorMsg)
		if message == "" {
			message = string(task.Status)
		}
		return result, fmt.Errorf("%w: %s", ErrDownloadNotCompleted, message)
	}

	result, err = ProcessFile(ctx, ProcessRequest{
		SourcePath:     task.StoragePath,
		ExtractAs:      string(mode),
		Destination:    request.Destination,
		ExtractOptions: extractOptions,
		ExpectedFile:   expectedFile,
	})
	result.Download = task
	if err != nil {
		return result, fmt.Errorf("process downloaded file: %w", err)
	}

	if request.RemoveArchiveAfterSuccess && result.Extracted {
		if err := os.Remove(result.ArchivePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, fmt.Errorf("remove downloaded archive after extraction: %w", err)
		}
		result.ArchiveRemoved = true
	}
	return result, nil
}

func validateRequest(request Request) (Mode, dezip.Options, string, error) {
	mode, err := parseMode(request.ExtractAs)
	if err != nil {
		return "", dezip.Options{}, "", err
	}
	expectedFile, err := normalizeExpectedFilePath(request.ExpectedFile)
	if err != nil {
		return "", dezip.Options{}, "", err
	}
	if mode == ModeSkip && expectedFile != "" {
		return "", dezip.Options{}, "", errors.New("expected file is only valid when extracting")
	}
	if mode != ModeSkip {
		if request.Destination == "" {
			return "", dezip.Options{}, "", errors.New("destination is required unless extraction is skipped")
		}
		destination, err := absoluteCleanPath(request.Destination)
		if err != nil {
			return "", dezip.Options{}, "", fmt.Errorf("resolve extraction destination: %w", err)
		}
		downloadPath, err := resolveDownloadPath(request)
		if err != nil {
			return "", dezip.Options{}, "", fmt.Errorf("resolve download path: %w", err)
		}
		if downloadPath != "" && pathsOverlap(downloadPath, destination) {
			return "", dezip.Options{}, "", fmt.Errorf("%w: download path %q overlaps extraction destination %q", ErrPathConflict, downloadPath, destination)
		}
		if _, err := os.Lstat(destination); err == nil {
			return "", dezip.Options{}, "", dezip.ErrDestinationExists
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", dezip.Options{}, "", fmt.Errorf("check extraction destination: %w", err)
		}
	}
	return mode, request.ExtractOptions, expectedFile, nil
}

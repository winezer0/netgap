package downloadunpack

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/winezer0/dezip"
)

// ProcessFile processes an existing local file using the same extraction
// rules as Run, without downloading it.
func ProcessFile(ctx context.Context, request ProcessRequest) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	mode, err := parseMode(request.ExtractAs)
	if err != nil {
		return Result{}, err
	}
	expectedFile, err := normalizeExpectedFilePath(request.ExpectedFile)
	if err != nil {
		return Result{}, err
	}
	if mode == ModeSkip && expectedFile != "" {
		return Result{}, errors.New("expected file is only valid when extracting")
	}
	if request.SourcePath == "" {
		return Result{}, errors.New("source path is required")
	}

	sourcePath, err := absoluteCleanPath(request.SourcePath)
	if err != nil {
		return Result{}, fmt.Errorf("resolve source path: %w", err)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return Result{}, fmt.Errorf("inspect source file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Result{}, errors.New("source path is not a regular file")
	}

	result := Result{Mode: mode, ArchivePath: sourcePath}
	if mode == ModeSkip {
		result.Skipped = true
		result.OutputPath = sourcePath
		result.MainFile = sourcePath
		return result, nil
	}
	if request.Destination == "" {
		return result, errors.New("destination is required unless extraction is skipped")
	}
	destination, err := absoluteCleanPath(request.Destination)
	if err != nil {
		return result, fmt.Errorf("resolve extraction destination: %w", err)
	}
	if pathsOverlap(sourcePath, destination) {
		return result, fmt.Errorf("%w: source path %q overlaps extraction destination %q", ErrPathConflict, sourcePath, destination)
	}
	if _, err := os.Lstat(destination); err == nil {
		return result, dezip.ErrDestinationExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, fmt.Errorf("check extraction destination: %w", err)
	}

	processed, mainFile, skipped, err := processDownloadedFile(
		ctx, sourcePath, destination, mode, request.ExtractOptions, expectedFile,
	)
	result.Extraction = processed
	if err != nil {
		return result, fmt.Errorf("process local file: %w", err)
	}
	if skipped {
		result.Skipped = true
		result.OutputPath = sourcePath
		result.MainFile = sourcePath
		return result, nil
	}
	result.Extracted = true
	result.OutputPath = processed.OutputDir
	result.MainFile = mainFile
	return result, nil
}

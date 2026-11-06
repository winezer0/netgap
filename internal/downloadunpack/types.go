package downloadunpack

import (
	"errors"
	"fmt"
	"strings"

	"github.com/winezer0/dezip"
	"github.com/winezer0/downutils"
)

var (
	ErrDownloadNotCompleted = errors.New("download did not complete successfully")
	ErrInvalidMode          = errors.New("invalid extraction mode")
	ErrModeMismatch         = errors.New("input format does not match extraction mode")
	ErrPathConflict         = errors.New("request paths conflict")
)

type Mode string

const (
	ModeAuto Mode = "auto"
	ModeZip  Mode = "zip"
	ModeTar  Mode = "tar"
	ModeGzip Mode = "gzip"
	ModeSkip Mode = "skip"
)

// Request describes one download and its optional post-download processing.
type Request struct {
	Item                      downutils.DownItem
	DownloadOptions           *downutils.DownOptions
	ExtractAs                 string
	Destination               string
	ExtractOptions            dezip.Options
	ExpectedFile              string
	RemoveArchiveAfterSuccess bool
}

// ProcessRequest describes post-processing for an existing local file.
type ProcessRequest struct {
	SourcePath     string
	ExtractAs      string
	Destination    string
	ExtractOptions dezip.Options
	ExpectedFile   string
}

// Result identifies the downloaded asset and any extracted output.
type Result struct {
	Mode            Mode
	Download        downutils.TaskInfo
	ArchivePath     string
	DownloadSkipped bool
	Skipped         bool
	Extracted       bool
	Extraction      dezip.Result
	OutputPath      string
	MainFile        string
	ArchiveRemoved  bool
}

// Outcome holds one result from RunMany. Per-item failures do not stop later items.
type Outcome struct {
	Result Result
	Err    error
}

func parseMode(value string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(ModeSkip):
		return ModeSkip, nil
	case string(ModeAuto):
		return ModeAuto, nil
	case string(ModeZip):
		return ModeZip, nil
	case string(ModeTar), "tar.gz", "tar.xz", "tar.bz2", "tar.lz", "tar.lz4", "tar.br", "tar.sz", "tar.s2", "tar.zz", "tar.zst":
		return ModeTar, nil
	case string(ModeGzip):
		return ModeGzip, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidMode, value)
	}
}

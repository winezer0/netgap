package downloadunpack

import (
	"context"
	"fmt"
)

// RunMany validates and processes requests sequentially. Invalid requests reject
// the whole batch before any download; runtime failures are recorded per item.
func RunMany(ctx context.Context, requests []Request) ([]Outcome, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	type requestPaths struct {
		download    string
		destination string
	}
	resolvedPaths := make([]requestPaths, len(requests))
	for i, request := range requests {
		mode, _, _, err := validateRequest(request)
		if err != nil {
			return nil, fmt.Errorf("request %d: %w", i+1, err)
		}
		resolvedPaths[i].download, err = resolveDownloadPath(request)
		if err != nil {
			return nil, fmt.Errorf("request %d: resolve download path: %w", i+1, err)
		}
		if mode == ModeSkip {
			continue
		}
		resolvedPaths[i].destination, err = absoluteCleanPath(request.Destination)
		if err != nil {
			return nil, fmt.Errorf("request %d: resolve destination: %w", i+1, err)
		}
	}

	for i, current := range resolvedPaths {
		for previousIndex := 0; previousIndex < i; previousIndex++ {
			previous := resolvedPaths[previousIndex]
			if current.download != "" && previous.download != "" && pathsOverlap(current.download, previous.download) {
				return nil, fmt.Errorf("requests %d and %d have overlapping download paths: %w", previousIndex+1, i+1, ErrPathConflict)
			}
			if current.destination != "" && previous.destination != "" && pathsOverlap(current.destination, previous.destination) {
				return nil, fmt.Errorf("requests %d and %d have overlapping extraction destinations: %w", previousIndex+1, i+1, ErrPathConflict)
			}
			if current.download != "" && previous.destination != "" && pathsOverlap(current.download, previous.destination) {
				return nil, fmt.Errorf("request %d download path overlaps request %d extraction destination: %w", i+1, previousIndex+1, ErrPathConflict)
			}
			if current.destination != "" && previous.download != "" && pathsOverlap(current.destination, previous.download) {
				return nil, fmt.Errorf("request %d extraction destination overlaps request %d download path: %w", i+1, previousIndex+1, ErrPathConflict)
			}
		}
	}

	outcomes := make([]Outcome, len(requests))
	for i, request := range requests {
		if err := ctx.Err(); err != nil {
			for j := i; j < len(outcomes); j++ {
				outcomes[j].Err = err
			}
			break
		}
		outcomes[i].Result, outcomes[i].Err = Run(ctx, request)
	}
	return outcomes, nil
}

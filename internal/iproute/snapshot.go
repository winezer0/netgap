package iproute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"netgap/internal/detector"
)

// RouteEntry preserves both address families and the store owning each route.
type RouteEntry struct {
	CapturedAt     time.Time `json:"capturedAt,omitempty"`
	Destination    string    `json:"destination"`
	NextHop        string    `json:"nextHop"`
	InterfaceIndex uint32    `json:"interfaceIndex"`
	InterfaceGUID  string    `json:"interfaceGuid"`
	Addresses      []string  `json:"addresses"`
	Network        string    `json:"network"`
	Metric         uint32    `json:"metric"`
	Store          string    `json:"store"`
	Protocol       string    `json:"protocol"`
	Publish        string    `json:"publish"`
	ValidTicks     string    `json:"validTicks"`
	PreferredTicks string    `json:"preferredTicks"`
}

type routeSnapshot struct {
	Version int          `json:"version"`
	SavedAt time.Time    `json:"savedAt"`
	Routes  []RouteEntry `json:"routes"`
}

type routeBackend interface {
	list(context.Context) ([]RouteEntry, error)
	validate(context.Context, []RouteEntry) error
	remove(context.Context, RouteEntry) error
	restore(context.Context, RouteEntry, time.Time) error
}

var routeOperation sync.Mutex
var snapshotBackend routeBackend = newRouteBackend()

func snapshotPath() string { return GetCacheFilePath() + ".v2.json" }

func saveSnapshot(file string, snapshot *routeSnapshot) error {
	content, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(file), ".route-snapshot-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), file)
}

func loadSnapshot(file string) (*routeSnapshot, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var snapshot routeSnapshot
	if err := json.Unmarshal(content, &snapshot); err != nil {
		return nil, err
	}
	if snapshot.Version != 2 || snapshot.SavedAt.IsZero() || len(snapshot.Routes) == 0 {
		return nil, errors.New("invalid or empty route snapshot")
	}
	for _, r := range snapshot.Routes {
		if (r.Destination != "0.0.0.0/0" && r.Destination != "::/0") || r.InterfaceGUID == "" || r.InterfaceIndex == 0 || (r.Store != "ActiveStore" && r.Store != "PersistentStore") {
			return nil, errors.New("invalid route snapshot entry")
		}
	}
	return &snapshot, nil
}

// DeleteAllDefaultRoutes saves durably before the first mutation. A pending
// recovery record is lost on retry, partial failure, or a process restart.
func DeleteAllDefaultRoutes(ctx context.Context) error {
	return DeleteAllDefaultRoutesGuarded(ctx, nil)
}

func DeleteAllDefaultRoutesGuarded(ctx context.Context, validate func(context.Context) error) error {
	routeOperation.Lock()
	defer routeOperation.Unlock()
	unlock, err := lockSnapshotFile(snapshotPath())
	if err != nil {
		return err
	}
	defer unlock()
	before, err := detector.StateFingerprint(ctx)
	if err != nil {
		return err
	}
	guard := func() error {
		if validate != nil {
			if err := validate(ctx); err != nil {
				return err
			}
		}
		after, err := detector.StateFingerprint(ctx)
		if err != nil {
			return err
		}
		if before != after {
			return fmt.Errorf("%w while preparing disconnect", detector.ErrStateChanged)
		}
		return nil
	}
	if err := guard(); err != nil {
		return err
	}
	return deleteAllDefaultRoutes(ctx, snapshotBackend, snapshotPath(), guard)
}

func deleteAllDefaultRoutes(ctx context.Context, backend routeBackend, file string, guards ...func() error) error {
	snapshot, loadErr := loadSnapshot(file)
	if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		return fmt.Errorf("cannot preserve existing recovery data: %w", loadErr)
	}
	hadPending := loadErr == nil
	capturedAt := time.Now().UTC()
	routes, err := backend.list(ctx)
	if err != nil {
		return err
	}
	if len(routes) == 0 {
		if hadPending {
			return nil
		}
		return errors.New("no default routes to remove")
	}
	if err := backend.validate(ctx, routes); err != nil {
		return err
	}
	if snapshot == nil {
		snapshot = &routeSnapshot{Version: 2, SavedAt: time.Now().UTC()}
	}
	// Retries and later networks append recovery records instead of losing
	// earlier removals. Reappearing DHCP routes can therefore be removed again.
	for _, route := range routes {
		found := false
		for i, saved := range snapshot.Routes {
			if sameRoute(saved, route) {
				route.CapturedAt = capturedAt
				snapshot.Routes[i] = route
				found = true
				break
			}
		}
		if !found {
			route.CapturedAt = capturedAt
			snapshot.Routes = append(snapshot.Routes, route)
		}
	}
	if err := saveSnapshot(file, snapshot); err != nil {
		return fmt.Errorf("save all default routes before disconnect: %w", err)
	}
	for _, guard := range guards {
		if err := guard(); err != nil {
			if hadPending {
				return err
			}
			return errors.Join(err, os.Remove(file))
		}
	}
	// Persistent entries are removed first so they cannot recreate active rows.
	for _, store := range []string{"PersistentStore", "ActiveStore"} {
		for _, route := range routes {
			if route.Store != store {
				continue
			}
			if err := backend.remove(ctx, route); err != nil {
				return fmt.Errorf("disconnect incomplete; recovery snapshot retained: %w", err)
			}
		}
	}
	remaining, err := backend.list(ctx)
	if err != nil {
		return fmt.Errorf("cannot confirm disconnect; recovery snapshot retained: %w", err)
	}
	if len(remaining) != 0 {
		return errors.New("default routes remain or changed during disconnect; recovery snapshot retained")
	}
	return nil
}

// RestoreAllDefaultRoutes is idempotent after partial restoration. Existing
// conflicting rows and changed interface/network identities are never replaced.
func RestoreAllDefaultRoutes(ctx context.Context) error {
	routeOperation.Lock()
	defer routeOperation.Unlock()
	unlock, err := lockSnapshotFile(snapshotPath())
	if err != nil {
		return err
	}
	defer unlock()
	return restoreAllDefaultRoutes(ctx, snapshotBackend, snapshotPath())
}

func restoreAllDefaultRoutes(ctx context.Context, backend routeBackend, file string) error {
	snapshot, err := loadSnapshot(file)
	if err != nil {
		return fmt.Errorf("load complete recovery snapshot (legacy single-gateway cache is insufficient): %w", err)
	}
	var failures []error
	var pending []RouteEntry
	for _, store := range []string{"PersistentStore", "ActiveStore"} {
		for _, route := range snapshot.Routes {
			if route.Store == store {
				if err := backend.validate(ctx, []RouteEntry{route}); err != nil {
					failures = append(failures, fmt.Errorf("network changed; refusing stale route restore: %w", err))
					pending = append(pending, route)
					continue
				}
				captured := route.CapturedAt
				if captured.IsZero() {
					captured = snapshot.SavedAt
				}
				if err := backend.restore(ctx, route, captured); err != nil {
					failures = append(failures, err)
					pending = append(pending, route)
				}
			}
		}
	}
	if err := errors.Join(failures...); err != nil {
		snapshot.Routes = pending
		if saveErr := saveSnapshot(file, snapshot); saveErr != nil {
			return errors.Join(err, saveErr)
		}
		return fmt.Errorf("restore incomplete; recovery snapshot retained: %w", err)
	}
	return os.Remove(file)
}

func sameRoute(a, b RouteEntry) bool {
	a.CapturedAt, b.CapturedAt = time.Time{}, time.Time{}
	a.ValidTicks, b.ValidTicks, a.PreferredTicks, b.PreferredTicks = "", "", "", ""
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

package iproute

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"netgap/internal/detector"
)

// RouteEntry preserves both address families and the store owning each route.
type RouteEntry struct {
	CapturedAt         time.Time    `json:"capturedAt,omitempty"`
	Destination        string       `json:"destination"`
	NextHop            string       `json:"nextHop"`
	InterfaceIndex     uint32       `json:"interfaceIndex"`
	InterfaceGUID      string       `json:"interfaceGuid"`
	Addresses          []string     `json:"addresses"`
	Network            string       `json:"network"`
	Metric             uint32       `json:"metric"`
	Store              string       `json:"store"`
	Protocol           string       `json:"protocol"`
	Publish            string       `json:"publish"`
	ValidTicks         string       `json:"validTicks"`
	PreferredTicks     string       `json:"preferredTicks"`
	Raw                []byte       `json:"raw,omitempty"` // Native Linux/macOS route message, including attributes.
	NativeKey          []byte       `json:"nativeKey,omitempty"`
	Bindings           []RouteEntry `json:"bindings,omitempty"` // Native interface/network bindings.
	CreatedByRecovery  *RouteEntry  `json:"createdByRecovery,omitempty"`
	ActiveBefore       *RouteEntry  `json:"activeBefore,omitempty"`
	RemoveAfterRestore bool         `json:"removeAfterRestore,omitempty"`
}

type routeSnapshot struct {
	Version    int             `json:"version"`
	SavedAt    time.Time       `json:"savedAt"`
	Routes     []RouteEntry    `json:"routes"`
	Disconnect *disconnectTask `json:"disconnect,omitempty"`
}

const snapshotVersion = 3

type disconnectTask struct {
	ID         string       `json:"id"`
	Targets    []RouteEntry `json:"targets"`
	ProxyState string       `json:"proxyState"`
}

// DisconnectTask is a capability for one saved operation, never a fresh query.
type DisconnectTask struct{ ID string }

type PendingDisconnectError struct {
	Task *DisconnectTask
	Err  error
}

func (e *PendingDisconnectError) Error() string {
	return "disconnect incomplete; recovery snapshot retained: " + e.Err.Error()
}
func (e *PendingDisconnectError) Unwrap() error { return e.Err }

var ErrDisconnectObsolete = errors.New("saved disconnect task was restored or replaced")

type routeBackend interface {
	list(context.Context) ([]RouteEntry, error)
	validate(context.Context, []RouteEntry) error
	remove(context.Context, RouteEntry) error
	restore(context.Context, RouteEntry, time.Time) error
}

type restorePreparer interface {
	prepareRestore(context.Context, *routeSnapshot) error
}

var routeOperation = make(chan struct{}, 1)

func acquireRouteOperation(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case routeOperation <- struct{}{}:
		return func() { <-routeOperation }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

var snapshotBackend routeBackend = newRouteBackend()

func EnsureProtectionSupported() error {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return errors.New("complete default-route protection requires Windows, Linux, or macOS")
	}
	return nativeProtectionSupported()
}

func snapshotPath() string { return GetCacheFilePath() + ".json" }

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
	if snapshot.Version != snapshotVersion || snapshot.SavedAt.IsZero() || len(snapshot.Routes) == 0 {
		return nil, errors.New("invalid or empty route snapshot")
	}
	for _, r := range snapshot.Routes {
		if (r.Destination != "0.0.0.0/0" && r.Destination != "::/0") || r.InterfaceGUID == "" || r.InterfaceIndex == 0 || (r.Store != "ActiveStore" && r.Store != "PersistentStore" && r.Store != "Linux" && r.Store != "Darwin") {
			return nil, errors.New("invalid route snapshot entry")
		}
	}
	if snapshot.Disconnect != nil {
		if snapshot.Disconnect.ID == "" || snapshot.Disconnect.ProxyState == "" || len(snapshot.Disconnect.Targets) == 0 {
			return nil, errors.New("invalid disconnect task")
		}
		for _, target := range snapshot.Disconnect.Targets {
			found := false
			for _, saved := range snapshot.Routes {
				if sameRoute(saved, target) {
					found = true
					break
				}
			}
			if !found {
				return nil, errors.New("disconnect target missing from recovery snapshot")
			}
		}
	}
	return &snapshot, nil
}

// DeleteAllDefaultRoutes saves durably before the first mutation. A pending
// recovery record is never lost on retry, partial failure, or a process restart.
func DeleteAllDefaultRoutes(ctx context.Context) error {
	return DeleteAllDefaultRoutesGuarded(ctx, nil)
}

func DeleteAllDefaultRoutesGuarded(ctx context.Context, validate func(context.Context) error) error {
	release, err := acquireRouteOperation(ctx)
	if err != nil {
		return err
	}
	defer release()
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
	var original *routeSnapshot
	if snapshot != nil {
		copySnapshot := *snapshot
		copySnapshot.Routes = append([]RouteEntry(nil), snapshot.Routes...)
		original = &copySnapshot
	}
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
		snapshot = &routeSnapshot{Version: snapshotVersion, SavedAt: time.Now().UTC()}
	}
	// Retries and later networks append recovery records instead of losing
	// earlier removals. Reappearing DHCP routes can therefore be removed again.
	for _, route := range routes {
		found := false
		for i, saved := range snapshot.Routes {
			if sameRouteIdentity(saved, route) {
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
	// Persist the bounded operation separately from the complete recovery set.
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return err
	}
	proxyState, err := detector.ProxyFingerprint(ctx)
	if err != nil {
		return err
	}
	snapshot.Disconnect = &disconnectTask{ID: fmt.Sprintf("%x", id), Targets: append([]RouteEntry(nil), routes...), ProxyState: proxyState}
	if err := saveSnapshot(file, snapshot); err != nil {
		return fmt.Errorf("save all default routes before disconnect: %w", err)
	}
	for _, guard := range guards {
		if err := guard(); err != nil {
			if hadPending {
				return errors.Join(err, saveSnapshot(file, original))
			}
			return errors.Join(err, os.Remove(file))
		}
	}
	return executeDisconnect(ctx, backend, file, snapshot)
}

// PendingDefaultRouteDisconnect does not authorize mutations by itself.
func PendingDefaultRouteDisconnect(ctx context.Context) (*DisconnectTask, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	release, err := acquireRouteOperation(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	unlock, err := lockSnapshotFile(snapshotPath())
	if err != nil {
		return nil, err
	}
	defer unlock()
	snapshot, err := loadSnapshot(snapshotPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if snapshot.Disconnect == nil {
		return nil, nil
	}
	return &DisconnectTask{ID: snapshot.Disconnect.ID}, nil
}

func ResumeDefaultRouteDisconnect(ctx context.Context, task *DisconnectTask) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	release, err := acquireRouteOperation(ctx)
	if err != nil {
		return err
	}
	defer release()
	unlock, err := lockSnapshotFile(snapshotPath())
	if err != nil {
		return err
	}
	defer unlock()
	snapshot, err := loadSnapshot(snapshotPath())
	if errors.Is(err, os.ErrNotExist) {
		return ErrDisconnectObsolete
	}
	if err != nil {
		return err
	}
	if task == nil || snapshot.Disconnect == nil || snapshot.Disconnect.ID != task.ID {
		return ErrDisconnectObsolete
	}
	return executeDisconnect(ctx, snapshotBackend, snapshotPath(), snapshot)
}

func executeDisconnect(ctx context.Context, backend routeBackend, file string, snapshot *routeSnapshot) error {
	task := snapshot.Disconnect
	pending := func(err error) error { return &PendingDisconnectError{Task: &DisconnectTask{ID: task.ID}, Err: err} }
	proxyState, err := detector.ProxyFingerprint(ctx)
	if err != nil {
		return pending(err)
	}
	if proxyState != task.ProxyState {
		snapshot.Disconnect = nil
		return errors.Join(fmt.Errorf("%w: proxy rules changed; new exit check required", detector.ErrStateChanged), saveSnapshot(file, snapshot))
	}
	current, err := backend.list(ctx)
	if err != nil {
		return pending(err)
	}
	// Refuse to expand an old decision to a newly appeared/changed route.
	for _, row := range current {
		found := false
		for _, target := range task.Targets {
			if sameRoute(row, target) {
				found = true
				break
			}
		}
		if !found {
			snapshot.Disconnect = nil
			return errors.Join(fmt.Errorf("%w: default routes changed; new exit check required", detector.ErrStateChanged), saveSnapshot(file, snapshot))
		}
	}
	if len(current) != 0 {
		if err := backend.validate(ctx, task.Targets); err != nil {
			snapshot.Disconnect = nil
			return errors.Join(fmt.Errorf("%w: %v", detector.ErrStateChanged, err), saveSnapshot(file, snapshot))
		}
	}
	var failures []error
	for _, store := range []string{"PersistentStore", "ActiveStore", "Linux", "Darwin"} {
		for _, row := range current {
			if row.Store != store {
				continue
			}
			if err := ctx.Err(); err != nil {
				return pending(err)
			}
			if err := backend.remove(ctx, row); err != nil {
				failures = append(failures, err)
			}
		}
	}
	remaining, err := backend.list(ctx)
	if err != nil {
		return pending(errors.Join(append(failures, err)...))
	}
	if len(remaining) != 0 {
		failures = append(failures, fmt.Errorf("%d default routes remain", len(remaining)))
		return pending(errors.Join(failures...))
	}
	snapshot.Disconnect = nil
	if err := saveSnapshot(file, snapshot); err != nil {
		return pending(err)
	}
	return nil
}

// RestoreAllDefaultRoutes is idempotent after partial restoration. Existing
// conflicting rows and changed interface/network identities are never replaced.
func RestoreAllDefaultRoutes(ctx context.Context) error {
	release, err := acquireRouteOperation(ctx)
	if err != nil {
		return err
	}
	defer release()
	unlock, err := lockSnapshotFile(snapshotPath())
	if err != nil {
		return err
	}
	defer unlock()
	if err := restoreAllDefaultRoutes(ctx, snapshotBackend, snapshotPath()); err != nil {
		return fmt.Errorf("restore failed; recovery snapshot path: %s (preserve remaining data and use the same execution identity/data directory): %w", snapshotPath(), err)
	}
	return nil
}

func restoreAllDefaultRoutes(ctx context.Context, backend routeBackend, file string) error {
	snapshot, err := loadSnapshot(file)
	if err != nil {
		return fmt.Errorf("load complete recovery snapshot (legacy single-gateway cache is insufficient): %w", err)
	}
	var failures []error
	var pending []RouteEntry
	// Invalidate a monitor's saved capability before restoring any connectivity.
	snapshot.Disconnect = nil
	if preparer, ok := backend.(restorePreparer); ok {
		if err := preparer.prepareRestore(ctx, snapshot); err != nil {
			return err
		}
	}
	if err := saveSnapshot(file, snapshot); err != nil {
		return err
	}
	for _, store := range []string{"PersistentStore", "ActiveStore", "Linux", "Darwin"} {
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
		// Retain each failed Windows route's sibling store and its side-effect
		// journal until the whole pair is restored, including after a restart.
		failed := append([]RouteEntry(nil), pending...)
		for _, row := range snapshot.Routes {
			if row.Store == "Linux" || row.Store == "Darwin" {
				continue
			}
			for _, failure := range failed {
				a, b := row, failure
				a.Store, b.Store = "", ""
				if !sameRouteIdentity(a, b) {
					continue
				}
				found := false
				for _, saved := range pending {
					if sameRouteIdentity(saved, row) {
						found = true
						break
					}
				}
				if !found {
					pending = append(pending, row)
				}
				break
			}
		}
		snapshot.Routes = pending
		if saveErr := saveSnapshot(file, snapshot); saveErr != nil {
			return errors.Join(err, saveErr)
		}
		return fmt.Errorf("restore incomplete; recovery snapshot retained: %w", err)
	}
	return os.Remove(file)
}

func sameRoute(a, b RouteEntry) bool {
	if !sameRouteIdentity(a, b) {
		return false
	}
	if a.Store == "Linux" || a.Store == "Darwin" {
		return len(a.NativeKey) != 0 && bytes.Equal(a.NativeKey, b.NativeKey)
	}
	a.InterfaceIndex, b.InterfaceIndex = 0, 0
	a.Addresses, b.Addresses = nil, nil
	a.CreatedByRecovery, b.CreatedByRecovery = nil, nil
	a.ActiveBefore, b.ActiveBefore = nil, nil
	a.RemoveAfterRestore, b.RemoveAfterRestore = false, false
	a.CapturedAt, b.CapturedAt = time.Time{}, time.Time{}
	a.ValidTicks, b.ValidTicks, a.PreferredTicks, b.PreferredTicks = "", "", "", ""
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func sameRouteIdentity(a, b RouteEntry) bool {
	return strings.EqualFold(a.InterfaceGUID, b.InterfaceGUID) && a.Network == b.Network && a.Destination == b.Destination && a.NextHop == b.NextHop && a.Store == b.Store
}

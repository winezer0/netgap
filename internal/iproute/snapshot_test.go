package iproute

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeRouteBackend struct {
	rows                                          []RouteEntry
	removed, restored                             []RouteEntry
	removeFailure, restoreFailure, bindingFailure bool
	file                                          string
}

func (f *fakeRouteBackend) list(context.Context) ([]RouteEntry, error) {
	return append([]RouteEntry(nil), f.rows...), nil
}
func (f *fakeRouteBackend) validate(context.Context, []RouteEntry) error {
	if f.bindingFailure {
		return errors.New("network changed")
	}
	return nil
}
func (f *fakeRouteBackend) remove(ctx context.Context, r RouteEntry) error {
	if _, err := loadSnapshot(f.file); err != nil {
		return errors.New("mutation before durable snapshot")
	}
	f.removed = append(f.removed, r)
	if f.removeFailure {
		return errors.New("remove failed")
	}
	for i, row := range f.rows {
		if row.Store == r.Store && row.NextHop == r.NextHop {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			break
		}
	}
	return nil
}
func (f *fakeRouteBackend) restore(ctx context.Context, r RouteEntry, saved time.Time) error {
	f.restored = append(f.restored, r)
	if f.restoreFailure {
		return errors.New("restore failed")
	}
	return nil
}
func fakeRoutes() []RouteEntry {
	return []RouteEntry{
		{Destination: "0.0.0.0/0", NextHop: "192.0.2.1", InterfaceIndex: 4, InterfaceGUID: "adapter-a", Addresses: []string{"192.0.2.10/24"}, Metric: 12, Store: "ActiveStore"},
		{Destination: "::/0", NextHop: "fe80::1", InterfaceIndex: 8, InterfaceGUID: "adapter-b", Addresses: []string{"2001:db8::10/64"}, Metric: 30, Store: "ActiveStore"},
		{Destination: "0.0.0.0/0", NextHop: "192.0.2.1", InterfaceIndex: 4, InterfaceGUID: "adapter-a", Addresses: []string{"192.0.2.10/24"}, Metric: 12, Store: "PersistentStore"},
	}
}
func TestSnapshotSavesEveryRouteBeforeRemoval(t *testing.T) {
	file := filepath.Join(t.TempDir(), "routes.json")
	f := &fakeRouteBackend{rows: fakeRoutes(), file: file}
	if err := deleteAllDefaultRoutes(context.Background(), f, file); err != nil {
		t.Fatal(err)
	}
	snapshot, err := loadSnapshot(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Routes) != 3 || len(f.removed) != 3 || f.removed[0].Store != "PersistentStore" {
		t.Fatalf("incomplete snapshot/removal: %+v", snapshot)
	}
	if err := deleteAllDefaultRoutes(context.Background(), f, file); err != nil {
		t.Fatal("already disconnected retry should succeed", err)
	}
	if err := restoreAllDefaultRoutes(context.Background(), f, file); err != nil {
		t.Fatal(err)
	}
	if len(f.restored) != 3 {
		t.Fatal("did not restore every route")
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("completed recovery snapshot not cleared")
	}
}
func TestSnapshotRetainedAfterPartialFailure(t *testing.T) {
	file := filepath.Join(t.TempDir(), "routes.json")
	f := &fakeRouteBackend{rows: fakeRoutes(), file: file, removeFailure: true}
	if err := deleteAllDefaultRoutes(context.Background(), f, file); err == nil {
		t.Fatal("hid deletion failure")
	}
	f.restoreFailure = true
	if err := restoreAllDefaultRoutes(context.Background(), f, file); err == nil {
		t.Fatal("hid restoration failure")
	}
	if _, err := loadSnapshot(file); err != nil {
		t.Fatal("lost recovery data", err)
	}
}
func TestSnapshotRejectsChangedNetworkAndUnsavedDelete(t *testing.T) {
	file := filepath.Join(t.TempDir(), "routes.json")
	f := &fakeRouteBackend{rows: fakeRoutes(), file: file, bindingFailure: true}
	if err := deleteAllDefaultRoutes(context.Background(), f, file); err == nil {
		t.Fatal("accepted changed network")
	}
	if len(f.removed) != 0 {
		t.Fatal("deleted despite changed network")
	}
	f.bindingFailure = false
	if err := saveSnapshot(file, &routeSnapshot{Version: snapshotVersion, SavedAt: time.Now(), Routes: fakeRoutes()}); err != nil {
		t.Fatal(err)
	}
	f.bindingFailure = true
	if err := restoreAllDefaultRoutes(context.Background(), f, file); err == nil || len(f.restored) != 0 {
		t.Fatal("restored stale network")
	}
}
func TestSnapshotGuardRechecksBeforeFirstMutation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "routes.json")
	f := &fakeRouteBackend{rows: fakeRoutes(), file: file}
	err := deleteAllDefaultRoutes(context.Background(), f, file, func() error { return errors.New("proxy changed") })
	if err == nil || len(f.removed) != 0 {
		t.Fatal("stale proxy allowed deletion")
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unused snapshot blocks next check")
	}
}

func TestSnapshotRetryPreservesEarlierNetwork(t *testing.T) {
	file := filepath.Join(t.TempDir(), "routes.json")
	f := &fakeRouteBackend{rows: fakeRoutes(), file: file}
	if err := deleteAllDefaultRoutes(context.Background(), f, file); err != nil {
		t.Fatal(err)
	}
	newRoute := fakeRoutes()[0]
	newRoute.InterfaceGUID = "new-adapter"
	newRoute.InterfaceIndex = 20
	newRoute.NextHop = "198.51.100.1"
	f.rows = []RouteEntry{newRoute}
	if err := deleteAllDefaultRoutes(context.Background(), f, file); err != nil {
		t.Fatal(err)
	}
	snapshot, err := loadSnapshot(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Routes) != 4 {
		t.Fatal("lost earlier recovery records")
	}
}

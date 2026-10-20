package iproute

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestPreflightStoragePreservesRecovery(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "snapshot.json")
	if err := preflightStorage(context.Background(), file, false); err != nil {
		t.Fatal(err)
	}
	if err := preflightStorage(context.Background(), file, true); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing recovery accepted", err)
	}
	data := []byte("invalid recovery data")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := preflightStorage(context.Background(), file, false); err == nil {
		t.Fatal("invalid recovery overwritten")
	}
	after, _ := os.ReadFile(file)
	if string(after) != string(data) {
		t.Fatal("backup changed")
	}
	snapshot := &routeSnapshot{Version: snapshotVersion, SavedAt: time.Now(), Routes: fakeRoutes()}
	if err := saveSnapshot(file, snapshot); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(file)
	if err := preflightStorage(context.Background(), file, true); err != nil {
		t.Fatal(err)
	}
	after, _ = os.ReadFile(file)
	if string(after) != string(before) {
		t.Fatal("preflight changed recovery")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal("probe files leaked", entries)
	}
}

func TestPreflightRejectsDamagedSnapshotsWithoutChangingBytes(t *testing.T) {
	valid := routeSnapshot{Version: snapshotVersion, SavedAt: time.Now(), Routes: fakeRoutes()}
	legacy, empty, invalidRoute := valid, valid, valid
	legacy.Version--
	empty.Routes = nil
	invalidRoute.Routes = append([]RouteEntry(nil), valid.Routes...)
	invalidRoute.Routes[0].Destination = "192.0.2.0/24"
	encode := func(value routeSnapshot) []byte {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	for name, data := range map[string][]byte{
		"truncated": []byte(`{"version":`), "legacy": encode(legacy),
		"empty": encode(empty), "invalid-route": encode(invalidRoute),
	} {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "snapshot.json")
			if err := os.WriteFile(file, data, 0600); err != nil {
				t.Fatal(err)
			}
			for _, restore := range []bool{false, true} {
				if err := preflightStorage(context.Background(), file, restore); err == nil {
					t.Fatal("damaged snapshot accepted", restore)
				}
				after, err := os.ReadFile(file)
				if err != nil || string(after) != string(data) {
					t.Fatal("damaged recovery bytes changed", err)
				}
			}
			entries, err := os.ReadDir(filepath.Dir(file))
			if err != nil || len(entries) != 1 {
				t.Fatal("probe leaked", entries, err)
			}
		})
	}
}

func TestPreflightConcurrentStorageProbesPreserveSnapshot(t *testing.T) {
	file := filepath.Join(t.TempDir(), "snapshot.json")
	if err := saveSnapshot(file, &routeSnapshot{Version: snapshotVersion, SavedAt: time.Now(), Routes: fakeRoutes()}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	errors := make(chan error, 16)
	for range 16 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 10 {
				if err := preflightStorage(context.Background(), file, true); err != nil {
					errors <- err
					return
				}
			}
		}()
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	after, err := os.ReadFile(file)
	if err != nil || string(after) != string(before) {
		t.Fatal("concurrent preflight changed backup", err)
	}
	entries, err := os.ReadDir(filepath.Dir(file))
	if err != nil || len(entries) != 1 {
		t.Fatal("concurrent probe leaked", entries, err)
	}
}

func TestPreflightStorageCancellationAndInvalidDirectory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := preflightStorage(ctx, filepath.Join(t.TempDir(), "snapshot"), false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "not-directory")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := preflightStorage(context.Background(), filepath.Join(file, "snapshot"), false); err == nil {
		t.Fatal("invalid directory accepted")
	}
}

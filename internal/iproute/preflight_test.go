package iproute

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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

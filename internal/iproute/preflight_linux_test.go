//go:build linux

package iproute

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPreflightUnwritableDirectoryPreservesRecovery(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires an unprivileged user to enforce directory permissions")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "snapshot.json")
	if err := saveSnapshot(file, &routeSnapshot{Version: snapshotVersion, SavedAt: time.Now(), Routes: fakeRoutes()}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0700); err != nil {
			t.Error(err)
		}
	})
	for _, restore := range []bool{false, true} {
		if err := preflightStorage(context.Background(), file, restore); !errors.Is(err, os.ErrPermission) {
			t.Fatal("unwritable recovery directory accepted", err)
		}
	}
	after, err := os.ReadFile(file)
	if err != nil || string(before) != string(after) {
		t.Fatal("recovery changed on permission failure", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("probe leaked on permission failure", entries, err)
	}
}

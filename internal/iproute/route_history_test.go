package iproute

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type historyBackend struct {
	rows        []RouteEntry
	unavailable map[string]bool
	removed     []RouteEntry
}

func (b *historyBackend) list(context.Context) ([]RouteEntry, error) {
	return append([]RouteEntry(nil), b.rows...), nil
}
func (b *historyBackend) validate(_ context.Context, rows []RouteEntry) error {
	for _, r := range rows {
		if b.unavailable[r.InterfaceGUID] {
			return os.ErrNotExist
		}
	}
	return nil
}
func (b *historyBackend) remove(_ context.Context, r RouteEntry) error {
	b.removed = append(b.removed, r)
	for i, v := range b.rows {
		if sameRoute(v, r) {
			b.rows = append(b.rows[:i], b.rows[i+1:]...)
			break
		}
	}
	return nil
}
func (b *historyBackend) restore(_ context.Context, r RouteEntry, _ time.Time) error {
	b.rows = append(b.rows, r)
	return nil
}

func withHistoryBackend(t *testing.T, rows []RouteEntry) *historyBackend {
	t.Helper()
	oldDir, oldBackend := cacheDirOverride, snapshotBackend
	cacheDirOverride, snapshotBackend = t.TempDir(), &historyBackend{rows: rows}
	t.Cleanup(func() { cacheDirOverride, snapshotBackend = oldDir, oldBackend })
	return snapshotBackend.(*historyBackend)
}

func TestRouteBackupHistoryListsStableIDsAndSkipsDamagedLines(t *testing.T) {
	withHistoryBackend(t, fakeRoutes())
	if err := BackupDefaultRoutes(context.Background()); err != nil {
		t.Fatal(err)
	}
	history, err := ListRouteBackups()
	if err != nil || len(history.Records) != 1 || history.Records[0].IPv4Count != 2 || history.Records[0].IPv6Count != 1 {
		t.Fatalf("unexpected history: %+v, %v", history, err)
	}
	id := history.Records[0].ID
	if err := os.WriteFile(historyPath(), append(mustRead(t, historyPath()), []byte("{bad}\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	history, err = ListRouteBackups()
	if err != nil || history.InvalidLines != 1 || history.Records[0].ID != id {
		t.Fatalf("damaged line affected history: %+v, %v", history, err)
	}
}

func TestRestoreRouteBackupReplacesConflictsAndRetainsHistory(t *testing.T) {
	b := withHistoryBackend(t, fakeRoutes())
	if err := BackupDefaultRoutes(context.Background()); err != nil {
		t.Fatal(err)
	}
	history, _ := ListRouteBackups()
	b.rows[0].NextHop = "192.0.2.99"
	if _, err := RestoreRouteBackup(context.Background(), history.Records[0].ID, false); err == nil {
		t.Fatal("replaced a conflict without explicit confirmation")
	}
	result, err := RestoreRouteBackup(context.Background(), history.Records[0].ID, true)
	if err != nil || !result.Complete || result.Replaced != 1 {
		t.Fatalf("restore result %+v, err %v", result, err)
	}
	if len(b.removed) != 1 {
		t.Fatalf("expected one conflict replacement, got %d", len(b.removed))
	}
	if _, err := os.Stat(routeBackupTaskPath()); !os.IsNotExist(err) {
		t.Fatal("completed restore task retained", err)
	}
	if records, _, err := readRouteBackupRecords(); err != nil || len(records) != 1 {
		t.Fatal("history was modified", err)
	}
}

func TestRestoreRouteBackupSkipsUnavailableInterfaceAndContinues(t *testing.T) {
	b := withHistoryBackend(t, fakeRoutes())
	if err := BackupDefaultRoutes(context.Background()); err != nil {
		t.Fatal(err)
	}
	history, _ := ListRouteBackups()
	b.rows = nil
	b.unavailable = map[string]bool{"adapter-a": true}
	result, err := RestoreRouteBackup(context.Background(), history.Records[0].ID, true)
	if err != nil || result.Complete || result.Skipped != 2 || result.Restored != 1 {
		t.Fatalf("partial restore result %+v, err %v", result, err)
	}
	if len(b.rows) != 1 || b.rows[0].InterfaceGUID != "adapter-b" {
		t.Fatalf("remaining interface was not restored: %+v", b.rows)
	}
}

func TestOrdinaryRestoreDoesNotConsumeRouteHistory(t *testing.T) {
	withHistoryBackend(t, fakeRoutes())
	if err := BackupDefaultRoutes(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := RestoreAllDefaultRoutes(context.Background())
	if err != ErrNoRecoverySnapshot {
		t.Fatalf("ordinary restore consumed history: %v", err)
	}
	if _, err := os.Stat(snapshotPath()); !os.IsNotExist(err) {
		t.Fatal("history was copied into recovery snapshot", err)
	}
}

func mustRead(t *testing.T, file string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Clean(file))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

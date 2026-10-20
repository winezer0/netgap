package iproute

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// RecoveryPath is the exact path for the current execution identity. CLI and
// GUI must display this path instead of guessing another user's cache folder.
func RecoveryPath() string { return snapshotPath() }

// ProtectionPreflight checks permissions, recovery data and atomic file writes.
// It never adds or deletes routes. restore additionally requires a valid backup.
func ProtectionPreflight(ctx context.Context, restore bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := EnsureProtectionSupported(); err != nil {
		return err
	}
	file := snapshotPath()
	if !CheckAdminPermissionContext(ctx) {
		if err := ctx.Err(); err != nil {
			return err
		}
		return fmt.Errorf("route administrator/root permission required; recovery snapshot: %s (use the same execution identity and data directory)", file)
	}
	release, err := acquireRouteOperation(ctx)
	if err != nil {
		return err
	}
	defer release()
	unlock, err := lockSnapshotFile(file)
	if err != nil {
		return fmt.Errorf("recovery snapshot lock %s: %w", file, err)
	}
	defer unlock()
	return preflightStorage(ctx, file, restore)
}

func preflightStorage(ctx context.Context, file string, restore bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := loadSnapshot(file)
	if err != nil && (restore || !errors.Is(err, os.ErrNotExist)) {
		return fmt.Errorf("cannot use recovery snapshot %s; retain this file and use the original identity/data directory: %w", file, err)
	}
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("recovery directory %s: %w", dir, err)
	}
	f, err := os.CreateTemp(dir, ".netgap-preflight-*")
	if err != nil {
		return fmt.Errorf("recovery directory is not writable %s: %w", dir, err)
	}
	original := f.Name()
	renamed := original + ".renamed"
	defer os.Remove(original)
	defer os.Remove(renamed)
	// Exercise the same durable write + rename mechanism used by saveSnapshot,
	// without opening, replacing, or truncating the actual recovery record.
	if _, err = f.Write([]byte("NetGAP recovery storage probe\n")); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Rename(original, renamed); err != nil {
		return fmt.Errorf("recovery snapshot atomic rename unavailable in %s: %w", dir, err)
	}
	return nil
}

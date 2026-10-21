//go:build windows

package iproute

import (
	"context"
	"testing"
	"time"
)

// This exercises enumeration and binding only; it never mutates OS routes.
func TestWindowsSnapshotReadOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	b := windowsRouteBackend{}
	rows, err := b.list(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.InterfaceGUID == "" || row.ValidTicks == "" || row.PreferredTicks == "" {
			t.Fatalf("incomplete route record: %+v", row)
		}
	}
	if err := b.validate(ctx, rows); err != nil {
		t.Fatal(err)
	}
}

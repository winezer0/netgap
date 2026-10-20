//go:build !windows

package iproute

import (
	"context"
	"errors"
	"time"
)

type unsupportedSnapshot struct{}

func lockSnapshotFile(file string) (func(), error) {
	return nil, errors.New("complete route snapshots currently require Windows")
}

func newRouteBackend() routeBackend { return unsupportedSnapshot{} }
func (unsupportedSnapshot) list(context.Context) ([]RouteEntry, error) {
	return nil, errors.New("complete route snapshots currently require Windows")
}
func (unsupportedSnapshot) validate(context.Context, []RouteEntry) error {
	return errors.New("complete route snapshots currently require Windows")
}
func (unsupportedSnapshot) remove(context.Context, RouteEntry) error {
	return errors.New("complete route snapshots currently require Windows")
}
func (unsupportedSnapshot) restore(context.Context, RouteEntry, time.Time) error {
	return errors.New("complete route snapshots currently require Windows")
}

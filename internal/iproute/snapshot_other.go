//go:build !windows && !linux && !darwin

package iproute

import (
	"context"
	"errors"
	"time"
)

type unsupportedSnapshot struct{}

func lockSnapshotFile(file string) (func(), error) {
	return nil, errors.New("complete route snapshots require Windows, Linux, or macOS")
}

func newRouteBackend() routeBackend { return unsupportedSnapshot{} }
func (unsupportedSnapshot) list(context.Context) ([]RouteEntry, error) {
	return nil, errors.New("complete route snapshots require Windows, Linux, or macOS")
}
func (unsupportedSnapshot) validate(context.Context, []RouteEntry) error {
	return errors.New("complete route snapshots require Windows, Linux, or macOS")
}
func (unsupportedSnapshot) remove(context.Context, RouteEntry) error {
	return errors.New("complete route snapshots require Windows, Linux, or macOS")
}
func (unsupportedSnapshot) restore(context.Context, RouteEntry, time.Time) error {
	return errors.New("complete route snapshots require Windows, Linux, or macOS")
}

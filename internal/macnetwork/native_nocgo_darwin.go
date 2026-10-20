//go:build darwin && !cgo

package macnetwork

import (
	"context"
	"errors"
)

var errNative = errors.New("macOS system proxy and network support requires a CGO_ENABLED=1 build with Apple frameworks")

func nativeError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errNative
}

func ProxySettings(ctx context.Context) (Settings, error)      { return Settings{}, nativeError(ctx) }
func ProxyPaths(ctx context.Context, _ string) ([]Path, error) { return nil, nativeError(ctx) }
func NetworkState(ctx context.Context) (string, error)         { return "", nativeError(ctx) }
func Binding(ctx context.Context, _ string, _ int) (InterfaceBinding, error) {
	return InterfaceBinding{}, nativeError(ctx)
}

//go:build !linux

package detector

import (
	"context"
	"errors"
	"net/url"
)

func resolveGIOProxy(context.Context, string, *url.URL) ([]ProxyRoute, error) {
	return nil, errors.New("GIO proxy resolution is Linux-only")
}

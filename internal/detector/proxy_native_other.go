//go:build !darwin

package detector

import (
	"context"
	"fmt"
	"net/url"
)

func resolveNativeProxy(context.Context, *url.URL) ([]ProxyRoute, error) {
	return nil, fmt.Errorf("native URL proxy rules unavailable on this platform")
}

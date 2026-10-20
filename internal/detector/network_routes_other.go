//go:build !windows && !linux && !darwin

package detector

import "context"

func extraNetworkState(ctx context.Context) (string, error) { return "", ctx.Err() }

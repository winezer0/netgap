//go:build !windows && !linux && !darwin

package detector

import "context"

func getProxyFromSystem(context.Context) *ProxyConfig {
	return nil
}

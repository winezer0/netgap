//go:build !windows && !linux

package detector

func getProxyFromSystem() *ProxyConfig {
	return nil
}


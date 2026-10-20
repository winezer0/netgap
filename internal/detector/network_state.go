package detector

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
)

var ErrStateChanged = errors.New("network or proxy settings changed")

// StateFingerprint includes private proxy settings only as an irreversible hash.
func StateFingerprint(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	network, err := networkState()
	if err != nil {
		return "", err
	}
	p, source, err := DetectProxyWithSource()
	if err != nil {
		return "", err
	}
	settings := fmt.Sprintf("%s|%v|%s|%s|%s|%s|%v", source, p.Enabled, p.Type, p.Address, p.Port, p.Scheme, p.rules)
	if p.credentials != nil {
		settings += "|" + p.credentials.String()
	}
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY"} {
		settings += "|" + envValue(name)
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(network+settings))), nil
}

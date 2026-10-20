package detector

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"
)

var ErrStateChanged = errors.New("network or proxy settings changed")

// StateFingerprint includes private proxy settings only as an irreversible hash.
func StateFingerprint(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	network, err := networkState(ctx)
	if err != nil {
		return "", err
	}
	proxy, err := ProxyFingerprint(ctx)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(network+proxy))), nil
}

// ProxyFingerprint can guard a pending operation without treating its own
// route removals as a network switch.
func ProxyFingerprint(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	p, source, err := DetectProxyWithSource(ctx)
	if err != nil {
		return "", err
	}
	settings := fmt.Sprintf("%s|%v|%s|%s|%s|%s|%s", source, p.Enabled, p.Type, p.Address, p.Port, p.Scheme, p.rules.fingerprint())
	if p.credentials != nil {
		settings += "|" + p.credentials.String()
	}
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY"} {
		settings += "|" + envValue(name)
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(settings))), nil
}

package detector

import (
	"context"
	"errors"
	"fmt"
	"runtime"
)

const LinuxPACObservation = "linux_pac_wpad"

const ObservationWarning = "PAC/WPAD is not applied; checking the system-route exit only. Automatic disconnection is suspended; alerts only."

var ErrObservationOnly = errors.New("system-route observation does not authorize automatic disconnection")

func systemRouteObservation(state string) *ProxyConfig {
	return &ProxyConfig{Type: "system_route", Enabled: false, ObservationOnly: LinuxPACObservation,
		rules: &proxyRules{observationState: state}}
}

// AutomaticProtectionError is checked again at automatic action boundaries,
// including saved-task retries. Manual deletion/recovery remain explicit actions.
func AutomaticProtectionError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if runtime.GOOS != "linux" {
		return nil
	}
	proxy, _, err := DetectProxyWithSource(ctx)
	if err != nil {
		return err
	}
	if proxy != nil && proxy.ObservationOnly != "" {
		return fmt.Errorf("%w: %s", ErrObservationOnly, ObservationWarning)
	}
	return nil
}

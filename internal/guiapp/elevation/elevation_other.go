//go:build !windows

package elevation

import "errors"

func GetStatus() *Status { return &Status{} }

func Restart(monitoringState func() bool) error {
	return errors.New("administrator restart is only supported on Windows")
}

func Prepare(args []string) (*LaunchIntent, error) { return nil, nil }

func ShowLaunchError(err error) {}

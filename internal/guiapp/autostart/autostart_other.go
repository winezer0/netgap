//go:build !windows

package autostart

import "errors"

func GetStatus() (*Status, error) { return &Status{}, nil }

func SetEnabled(enabled bool) (*Status, error) {
	return nil, errors.New("login startup is only supported on Windows")
}

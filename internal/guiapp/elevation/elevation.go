// Package elevation manages an explicit, user-requested administrator restart.
package elevation

import "errors"

var ErrCancelled = errors.New("administrator authorization canceled")

type Status struct {
	Supported bool `json:"supported"`
	IsAdmin   bool `json:"isAdmin"`
}

type LaunchIntent struct {
	ResumeMonitoring bool
}

type RestartResult struct {
	Cancelled bool `json:"cancelled"`
}

// Package autostart manages current-user login startup registration.
package autostart

type Status struct {
	Supported   bool `json:"supported"`
	Registered  bool `json:"registered"`
	NeedsRepair bool `json:"needsRepair"`
}

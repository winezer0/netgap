//go:build darwin

// Package macnetwork uses macOS frameworks for system proxy and network rules.
package macnetwork

type Settings struct {
	Configured bool   `json:"configured"`
	State      string `json:"state"`
}

type Path struct {
	Scheme   string `json:"scheme"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type InterfaceBinding struct {
	Identity string `json:"identity"`
	Network  string `json:"network"`
}

//go:build linux && cgo && gio

// netgap-gio-proxy isolates potentially blocking GIO/PAC modules in a process
// that the parent can cancel, including all of its descendants.
package main

/*
#cgo pkg-config: gio-2.0
#include "native.h"
*/
import "C"

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"
)

type response struct {
	Version    int      `json:"version"`
	Backend    string   `json:"backend,omitempty"`
	State      string   `json:"state,omitempty"`
	Configured bool     `json:"configured"`
	Routes     []string `json:"routes,omitempty"`
	Error      string   `json:"error,omitempty"`
}

func main() {
	reply, err := resolve(os.Args[1:])
	if err != nil {
		reply = response{Version: 1, Error: err.Error()}
	}
	_ = json.NewEncoder(os.Stdout).Encode(reply)
}

func resolve(args []string) (response, error) {
	if len(args) != 1 {
		return response{}, errors.New("expected --snapshot or one HTTP/HTTPS URL")
	}
	if uid := os.Getenv("SUDO_UID"); uid != "" && uid != strconv.Itoa(os.Geteuid()) {
		return response{}, errors.New("elevated process cannot use another user's desktop session; run proxy checks in the original user session")
	}
	desktop := os.Getenv("XDG_CURRENT_DESKTOP")
	gnome, kde := false, false
	for _, name := range strings.Split(desktop, ":") {
		gnome = gnome || name == "GNOME"
		kde = kde || name == "KDE"
	}
	if !gnome && !kde {
		return response{}, errors.New("unidentified desktop proxy source; supported sources are GNOME and KDE through GIO")
	}
	backend := C.ng_backend()
	if backend == nil {
		return response{}, errors.New("GIO proxy resolver unavailable")
	}
	name := C.GoString(backend)
	if (gnome && name != "GProxyResolverGnome") || (!gnome && name != "GLibproxyResolver") {
		return response{}, errors.New("required desktop GIO proxy module unavailable or overridden")
	}
	var configured C.int
	raw := C.ng_snapshot(&configured)
	if raw == nil {
		return response{}, errors.New("cannot read GNOME proxy configuration")
	}
	state := C.GoString(raw)
	C.g_free(C.gpointer(unsafe.Pointer(raw)))
	if !gnome {
		// libproxy's KDE source uses kioslaverc. Hash every XDG layer, including
		// absent files and path precedence; never expose file contents or passwords.
		state = "KDE"
		roots := strings.Split(os.Getenv("XDG_CONFIG_DIRS"), ":")
		if len(roots) == 1 && roots[0] == "" {
			roots = []string{"/etc/xdg"}
		}
		home, err := os.UserConfigDir()
		if err != nil {
			return response{}, err
		}
		roots = append([]string{home}, roots...)
		for index, root := range roots {
			if !filepath.IsAbs(root) {
				return response{}, errors.New("invalid XDG configuration root")
			}
			path := filepath.Join(root, "kioslaverc")
			file, err := os.Open(path)
			if index == 0 && err != nil {
				return response{}, errors.New("cannot identify active KDE kioslaverc source")
			}
			if err != nil && !os.IsNotExist(err) {
				return response{}, errors.New("cannot read KDE proxy source")
			}
			exists := err == nil
			var data []byte
			if file != nil {
				data, err = io.ReadAll(io.LimitReader(file, 1024*1024+1))
				file.Close()
				if err != nil {
					return response{}, errors.New("cannot read KDE proxy source")
				}
			}
			if len(data) > 1024*1024 {
				return response{}, errors.New("KDE proxy source exceeds limit")
			}
			state += fmt.Sprintf("|%q:%t:%x", path, exists, sha256.Sum256(data))
		}
		configured = 1
	}
	for _, key := range []string{"XDG_CURRENT_DESKTOP", "GIO_USE_PROXY_RESOLVER", "GIO_EXTRA_MODULES", "GSETTINGS_BACKEND", "GSETTINGS_SCHEMA_DIR", "XDG_CONFIG_HOME", "XDG_CONFIG_DIRS", "DBUS_SESSION_BUS_ADDRESS", "HOME", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"} {
		state += fmt.Sprintf("|%s=%q", key, os.Getenv(key))
	}
	reply := response{Version: 1, Backend: name, Configured: configured != 0, State: fmt.Sprintf("%x", sha256.Sum256([]byte(name+state)))}
	if args[0] == "--snapshot" {
		return reply, nil
	}
	target := C.CString(args[0])
	defer C.free(unsafe.Pointer(target))
	var nativeError *C.char
	routes := C.ng_lookup(target, &nativeError)
	if nativeError != nil {
		C.g_free(C.gpointer(unsafe.Pointer(nativeError)))
		return response{}, errors.New("system PAC/proxy lookup failed")
	}
	if routes == nil {
		return response{}, errors.New("system proxy lookup returned no routes")
	}
	defer C.g_strfreev(routes)
	for i := 0; i < 65; i++ {
		value := *(**C.char)(unsafe.Add(unsafe.Pointer(routes), uintptr(i)*unsafe.Sizeof(routes)))
		if value == nil {
			return reply, nil
		}
		if i == 64 {
			return response{}, errors.New("too many system proxy candidates")
		}
		reply.Routes = append(reply.Routes, C.GoString(value))
	}
	return response{}, errors.New("invalid system proxy candidates")
}

//go:build darwin && cgo

package macnetwork

/*
#cgo LDFLAGS: -framework Foundation -framework CoreFoundation -framework CFNetwork -framework SystemConfiguration
#include "native_darwin.h"
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"unsafe"
)

func decode(value *C.char, result any) error {
	if value == nil {
		return errors.New("macOS native network operation returned no data")
	}
	defer C.free(unsafe.Pointer(value))
	data := []byte(C.GoString(value))
	var status struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return err
	}
	if status.Error != "" {
		return errors.New(status.Error)
	}
	return json.Unmarshal(data, result)
}

// These synchronous framework reads cannot be interrupted once inside C.
// Check both sides so cancellation never authorizes use of a stale result.
func ProxySettings(ctx context.Context) (Settings, error) {
	if err := ctx.Err(); err != nil {
		return Settings{}, err
	}
	var result Settings
	err := decode(C.ng_proxy_settings(), &result)
	if ctx.Err() != nil {
		return Settings{}, ctx.Err()
	}
	return result, err
}

func ProxyPaths(ctx context.Context, target string) ([]Path, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request := C.ng_request_new()
	if request == nil {
		return nil, errors.New("allocate macOS proxy request")
	}
	defer C.ng_request_free(request)
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { C.ng_request_cancel(request); close(done) })
	// Wait for an already running cancellation before freeing native memory.
	defer func() {
		if !stop() {
			<-done
		}
	}()
	url := C.CString(target)
	defer C.free(unsafe.Pointer(url))
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var result struct {
		Paths []Path `json:"paths"`
	}
	err := decode(C.ng_proxy_routes(request, url), &result)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return result.Paths, err
}

func NetworkState(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var result struct {
		State string `json:"state"`
	}
	err := decode(C.ng_network_state(), &result)
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	return result.State, err
}

func Binding(ctx context.Context, name string, family int) (InterfaceBinding, error) {
	if err := ctx.Err(); err != nil {
		return InterfaceBinding{}, err
	}
	value := C.CString(name)
	defer C.free(unsafe.Pointer(value))
	var result InterfaceBinding
	err := decode(C.ng_interface_binding(value, C.int(family)), &result)
	if ctx.Err() != nil {
		return InterfaceBinding{}, ctx.Err()
	}
	return result, err
}

//go:build windows

package detector

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	winhttp       = windows.NewLazySystemDLL("winhttp.dll")
	winOpen       = winhttp.NewProc("WinHttpOpen")
	winClose      = winhttp.NewProc("WinHttpCloseHandle")
	winConfig     = winhttp.NewProc("WinHttpGetIEProxyConfigForCurrentUser")
	winCallback   = winhttp.NewProc("WinHttpSetStatusCallback")
	winSetOption  = winhttp.NewProc("WinHttpSetOption")
	winResolver   = winhttp.NewProc("WinHttpCreateProxyResolver")
	winResolve    = winhttp.NewProc("WinHttpGetProxyForUrlEx")
	winResult     = winhttp.NewProc("WinHttpGetProxyResult")
	winFreeResult = winhttp.NewProc("WinHttpFreeProxyResult")
	globalFree    = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalFree")
	proxyPending  sync.Map
	proxySequence atomic.Uint64
	proxyCallback = syscall.NewCallback(onProxyStatus)
)

type nativeIEConfig struct {
	AutoDetect         int32
	PAC, Proxy, Bypass *uint16
}
type nativeAutoOptions struct {
	Flags, DetectFlags uint32
	URL                *uint16
	Reserved           uintptr
	ReservedFlags      uint32
	AutoLogon          int32
}
type nativeProxyEntry struct {
	Proxy, Bypass, Scheme int32
	Host                  *uint16
	Port                  uint16
}
type nativeProxyResult struct {
	Count   uint32
	Entries *nativeProxyEntry
}
type nativeAsyncResult struct {
	Result uintptr
	Error  uint32
}
type proxyRequest struct {
	done    chan error
	options nativeAutoOptions
	target  *uint16
}

func getProxyFromSystem(ctx context.Context) *ProxyConfig {
	if err := ctx.Err(); err != nil {
		return &ProxyConfig{configErr: err}
	}
	var cfg nativeIEConfig
	ok, _, err := winConfig.Call(uintptr(unsafe.Pointer(&cfg)))
	if ok == 0 {
		return &ProxyConfig{configErr: fmt.Errorf("read Windows system proxy: %w", err)}
	}
	defer func() {
		for _, p := range []*uint16{cfg.PAC, cfg.Proxy, cfg.Bypass} {
			if p != nil {
				globalFree.Call(uintptr(unsafe.Pointer(p)))
			}
		}
	}()
	r := &proxyRules{servers: windows.UTF16PtrToString(cfg.Proxy), bypass: windows.UTF16PtrToString(cfg.Bypass), pacURL: windows.UTF16PtrToString(cfg.PAC), autoDetect: cfg.AutoDetect != 0}
	if r.servers == "" && r.pacURL == "" && !r.autoDetect {
		return nil
	}
	p := &ProxyConfig{Enabled: true, Type: "system", rules: r}
	if r.pacURL != "" {
		p.Type = "pac"
	} else if r.autoDetect {
		p.Type = "auto"
	} else if first := parseWindowsProxyServer(r.servers); first != nil {
		p.Address, p.Port, p.Scheme = first.Address, first.Port, first.Scheme
	}
	return p
}

// Keep native input buffers alive through HANDLE_CLOSING; only integer IDs
// cross the callback boundary, and late notifications cannot touch a new run.
func onProxyStatus(handle, token, status, info, length uintptr) uintptr {
	v, ok := proxyPending.Load(token)
	if !ok {
		return 0
	}
	request := v.(*proxyRequest)
	if status == 0x800 {
		proxyPending.Delete(token)
		return 0
	}
	var err error
	switch status {
	case 0x01000000:
	case 0x00200000:
		if info == 0 || length < unsafe.Sizeof(nativeAsyncResult{}) {
			err = errors.New("invalid Windows proxy callback")
		} else {
			err = syscall.Errno((*nativeAsyncResult)(unsafe.Pointer(info)).Error)
		}
	default:
		return 0
	}
	select {
	case request.done <- err:
	default:
	}
	return 0
}

func resolveAutoProxy(ctx context.Context, rules *proxyRules, target *url.URL) ([]ProxyRoute, error) {
	if rules.pacURL != "" {
		u, err := url.Parse(rules.pacURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, errors.New("Windows PAC URL must use HTTP or HTTPS; file PAC is not supported")
		}
	}
	for _, proc := range []*windows.LazyProc{winResolver, winResolve, winResult, winFreeResult} {
		if err := proc.Find(); err != nil {
			return nil, fmt.Errorf("Windows automatic proxy API unavailable: %w", err)
		}
	}
	session, _, err := winOpen.Call(0, 1, 0, 0, 0x10000000)
	if session == 0 {
		return nil, fmt.Errorf("open Windows proxy session: %w", err)
	}
	defer winClose.Call(session)
	previous, _, err := winCallback.Call(session, proxyCallback, 0x01000000|0x00200000|0x800, 0)
	if previous == ^uintptr(0) {
		return nil, fmt.Errorf("register Windows proxy callback: %w", err)
	}
	var resolver uintptr
	code, _, _ := winResolver.Call(session, uintptr(unsafe.Pointer(&resolver)))
	if code != 0 {
		return nil, fmt.Errorf("create Windows proxy resolver: %w", syscall.Errno(code))
	}
	defer winClose.Call(resolver)
	request := &proxyRequest{done: make(chan error, 1)}
	request.target, err = windows.UTF16PtrFromString(target.String())
	if err != nil {
		return nil, err
	}
	if rules.autoDetect {
		request.options.Flags |= 1
		request.options.DetectFlags = 3
	}
	if rules.pacURL != "" {
		request.options.Flags |= 2
		request.options.URL, err = windows.UTF16PtrFromString(rules.pacURL)
		if err != nil {
			return nil, err
		}
	}
	token := uintptr(proxySequence.Add(1))
	ok, _, optionErr := winSetOption.Call(resolver, 45, uintptr(unsafe.Pointer(&token)), unsafe.Sizeof(token))
	if ok == 0 {
		return nil, fmt.Errorf("set Windows proxy callback context: %w", optionErr)
	}
	proxyPending.Store(token, request)
	code, _, _ = winResolve.Call(resolver, uintptr(unsafe.Pointer(request.target)), uintptr(unsafe.Pointer(&request.options)), token)
	if code != 997 {
		proxyPending.Delete(token)
		if code == 0 {
			return nil, errors.New("Windows automatic proxy returned no completion")
		}
		return autoProxyFailure(rules, target, syscall.Errno(code))
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-request.done:
		if err != nil {
			return autoProxyFailure(rules, target, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var result nativeProxyResult
	code, _, _ = winResult.Call(resolver, uintptr(unsafe.Pointer(&result)))
	if code != 0 {
		return nil, fmt.Errorf("read Windows proxy result: %w", syscall.Errno(code))
	}
	defer winFreeResult.Call(uintptr(unsafe.Pointer(&result)))
	if result.Count == 0 || result.Entries == nil {
		return nil, errors.New("Windows automatic proxy returned no paths")
	}
	routes := make([]ProxyRoute, 0, result.Count)
	for _, entry := range unsafe.Slice(result.Entries, int(result.Count)) {
		if entry.Proxy == 0 || entry.Bypass != 0 {
			routes = append(routes, ProxyRoute{})
			continue
		}
		scheme := ""
		switch entry.Scheme {
		case 1:
			scheme = "http"
		case 2:
			scheme = "https"
		case 4:
			scheme = "socks4"
		default:
			return nil, fmt.Errorf("unsupported Windows proxy scheme %d", entry.Scheme)
		}
		u := &url.URL{Scheme: scheme, Host: net.JoinHostPort(windows.UTF16PtrToString(entry.Host), strconv.Itoa(int(entry.Port)))}
		validated, err := checkedRoute(u)
		if err != nil {
			return nil, err
		}
		routes = append(routes, validated...)
	}
	return routes, nil
}

func autoProxyFailure(rules *proxyRules, target *url.URL, err error) ([]ProxyRoute, error) {
	// Missing WPAD is normal with automatic discovery alone. Explicit PAC
	// failures must never silently become a direct connection.
	if rules.pacURL == "" && errors.Is(err, syscall.Errno(12180)) {
		return staticRoutes(rules, target)
	}
	return nil, fmt.Errorf("resolve Windows PAC/WPAD rules: %w", err)
}

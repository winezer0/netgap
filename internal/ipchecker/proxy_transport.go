package ipchecker

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"netgap/internal/detector"
)

type routeObserverKey struct{}

// ObserveRoute reports a successful HTTP exchange's path without credentials.
func ObserveRoute(ctx context.Context, observe func(*detector.ProxyConfig)) context.Context {
	return context.WithValue(ctx, routeObserverKey{}, observe)
}

type proxyRouteResolver interface {
	Routes(context.Context, *url.URL) ([]detector.ProxyRoute, error)
}
type systemTransport struct{ config proxyRouteResolver }

var errPathBudget = errors.New("proxy candidate connection budget exhausted")

type routeBody struct {
	io.ReadCloser
	cancel context.CancelCauseFunc
}

func (b *routeBody) Close() error { defer b.cancel(nil); return b.ReadCloser.Close() }

func routeBudget(ctx context.Context, slots int) time.Duration {
	budget := 1500 * time.Millisecond
	if deadline, ok := ctx.Deadline(); ok {
		share := time.Until(deadline) / time.Duration(slots)
		if share < budget {
			budget = share
		}
	}
	if budget <= 0 {
		return time.Nanosecond
	}
	return budget
}

func (t *systemTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var routes []detector.ProxyRoute
	var err error
	if t.config == nil {
		routes = []detector.ProxyRoute{{}}
	} else {
		rulesCtx, cancel := context.WithTimeout(req.Context(), routeBudget(req.Context(), 3))
		routes, err = t.config.Routes(rulesCtx, req.URL)
		cancel()
	}
	if err != nil {
		return nil, err
	}
	var lastErr error
	var lastProxy *url.URL
	for index, route := range routes {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		candidateCtx, cancel := context.WithCancelCause(req.Context())
		var wroteRequest atomic.Bool
		timer := time.AfterFunc(routeBudget(req.Context(), len(routes)-index+1), func() {
			if !wroteRequest.Load() {
				cancel(errPathBudget)
			}
		})
		trace := &httptrace.ClientTrace{WroteRequest: func(httptrace.WroteRequestInfo) { wroteRequest.Store(true); timer.Stop() }}
		candidate := req.Clone(httptrace.WithClientTrace(candidateCtx, trace))
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		transport.DisableKeepAlives = true
		if route.URL != nil {
			if route.URL.Scheme == "socks4" || route.URL.Scheme == "socks4a" {
				proxyURL := route.URL
				transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
					return dialSOCKS4(ctx, proxyURL, address)
				}
			} else {
				transport.Proxy = http.ProxyURL(route.URL)
			}
		}
		response, err := transport.RoundTrip(candidate)
		timer.Stop()
		transport.CloseIdleConnections()
		if err == nil {
			response.Body = &routeBody{ReadCloser: response.Body, cancel: cancel}
			if observe, ok := req.Context().Value(routeObserverKey{}).(func(*detector.ProxyConfig)); ok {
				actual := &detector.ProxyConfig{}
				if route.URL != nil {
					actual.Enabled, actual.Type, actual.Scheme = true, "port", route.URL.Scheme
					actual.Address, actual.Port = route.URL.Hostname(), route.URL.Port()
				}
				observe(actual)
			}
			return response, nil
		}
		budgetExpired := errors.Is(context.Cause(candidateCtx), errPathBudget)
		cancel(nil)
		if req.Context().Err() != nil {
			return nil, req.Context().Err()
		}
		if budgetExpired && !wroteRequest.Load() {
			err = &net.OpError{Op: "proxyconnect", Net: "tcp", Err: fmt.Errorf("%w: %w", errPathBudget, context.DeadlineExceeded)}
		}
		lastErr = err
		lastProxy = route.URL
		// Only replay bodyless GETs after connection failure. Origin HTTP
		// errors never authorize a different path or implicit DIRECT.
		if req.Method != http.MethodGet || req.Body != nil || wroteRequest.Load() || !proxyConnectionFailure(err) {
			break
		}
	}
	if lastErr == nil {
		lastErr = errors.New("system proxy rules returned no paths")
	}
	if lastProxy != nil && proxyConnectionFailure(lastErr) {
		lastErr = fmt.Errorf("%w (%s): %w", ErrProxyUnavailable, lastProxy.Host, lastErr)
	}
	return nil, lastErr
}

func proxyConnectionFailure(err error) bool {
	var op *net.OpError
	if !errors.As(err, &op) {
		return false
	}
	return op.Op == "proxyconnect" || op.Op == "socks connect" || op.Op == "dial"
}

func dialSOCKS4(ctx context.Context, proxy *url.URL, address string) (net.Conn, error) {
	conn, err := socks4Connect(ctx, proxy, address)
	if err != nil {
		return nil, &net.OpError{Op: "socks connect", Net: "tcp", Err: err}
	}
	return conn, nil
}

func socks4Connect(ctx context.Context, proxy *url.URL, address string) (net.Conn, error) {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil {
		return nil, err
	}
	ip := net.ParseIP(host).To4()
	remoteDNS := proxy.Scheme == "socks4a" && ip == nil
	if ip == nil && !remoteDNS {
		addresses, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
		if err != nil {
			return nil, err
		}
		if len(addresses) == 0 {
			return nil, fmt.Errorf("SOCKS4 requires an IPv4 destination")
		}
		ip = addresses[0].To4()
	}
	if remoteDNS {
		ip = net.IPv4(0, 0, 0, 1).To4()
	}
	user := ""
	if proxy.User != nil {
		user = proxy.User.Username()
		if _, set := proxy.User.Password(); set {
			return nil, errors.New("SOCKS4 does not support password authentication")
		}
	}
	if strings.ContainsRune(user, 0) || strings.ContainsRune(host, 0) {
		return nil, errors.New("invalid SOCKS4 destination or user")
	}
	proxyAddress := proxy.Host
	if proxy.Port() == "" {
		proxyAddress = net.JoinHostPort(proxy.Hostname(), "1080")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", proxyAddress)
	if err != nil {
		return nil, err
	}
	success := false
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer func() {
		stop()
		if !success {
			conn.Close()
		}
	}()
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	packet := make([]byte, 8)
	packet[0], packet[1] = 4, 1
	binary.BigEndian.PutUint16(packet[2:4], uint16(port))
	copy(packet[4:], ip)
	packet = append(packet, []byte(user)...)
	packet = append(packet, 0)
	if remoteDNS {
		packet = append(packet, []byte(host)...)
		packet = append(packet, 0)
	}
	if _, err := io.Copy(conn, strings.NewReader(string(packet))); err != nil {
		return nil, err
	}
	var reply [8]byte
	if _, err := io.ReadFull(conn, reply[:]); err != nil {
		return nil, err
	}
	if reply[0] != 0 || reply[1] != 90 {
		return nil, fmt.Errorf("SOCKS4 connection rejected (%d)", reply[1])
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Time{})
	success = true
	return conn, nil
}

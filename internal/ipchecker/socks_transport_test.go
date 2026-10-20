package ipchecker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"netgap/internal/detector"
)

// A local protocol server checks the handshake and answers the tunneled HTTP
// request itself. No destination DNS or external network connection is used.
func TestTransportSOCKSHandshakes(t *testing.T) {
	for _, scheme := range []string{"socks4", "socks4a", "socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(3 * time.Second))
				reader := bufio.NewReader(conn)
				var handshakeErr error
				if scheme == "socks4" || scheme == "socks4a" {
					handshakeErr = testSOCKS4(reader, conn, scheme == "socks4a")
				} else {
					handshakeErr = testSOCKS5(reader, conn)
				}
				if handshakeErr != nil {
					done <- handshakeErr
					return
				}
				req, err := http.ReadRequest(reader)
				if err != nil {
					done <- err
					return
				}
				req.Body.Close()
				_, err = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
				done <- err
			}()
			u, _ := url.Parse(scheme + "://" + listener.Addr().String())
			u.User = url.User("alice")
			if scheme == "socks5" || scheme == "socks5h" {
				u.User = url.UserPassword("alice", "secret")
			}
			resolver := testRouteResolver(func(context.Context, *url.URL) ([]detector.ProxyRoute, error) {
				return []detector.ProxyRoute{{URL: u}}, nil
			})
			target := "http://proxy-target.example/"
			if scheme == "socks4" {
				target = "http://192.0.2.1/"
			}
			resp, err := (&http.Client{Transport: &systemTransport{config: resolver}, Timeout: 3 * time.Second}).Get(target)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if string(body) != "ok" {
				t.Fatalf("unexpected body: %s", body)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func testSOCKS4(r *bufio.Reader, w io.Writer, remoteDNS bool) error {
	var header [8]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	if header[0] != 4 || header[1] != 1 {
		return fmt.Errorf("invalid SOCKS4 version/command")
	}
	user, err := r.ReadString(0)
	if err != nil {
		return err
	}
	if user != "alice\x00" {
		return fmt.Errorf("missing SOCKS4 user")
	}
	if remoteDNS {
		host, err := r.ReadString(0)
		if err != nil {
			return err
		}
		if host != "proxy-target.example\x00" {
			return fmt.Errorf("missing SOCKS4a hostname")
		}
	}
	_, err = w.Write([]byte{0, 90, 0, 0, 0, 0, 0, 0})
	return err
}
func testSOCKS5(r *bufio.Reader, w io.Writer) error {
	var header [2]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	if header[0] != 5 {
		return fmt.Errorf("invalid SOCKS5 version")
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(r, methods); err != nil {
		return err
	}
	if _, err := w.Write([]byte{5, 2}); err != nil {
		return err
	}
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	user := make([]byte, int(header[1]))
	if _, err := io.ReadFull(r, user); err != nil {
		return err
	}
	length, err := r.ReadByte()
	if err != nil {
		return err
	}
	pass := make([]byte, int(length))
	if _, err := io.ReadFull(r, pass); err != nil {
		return err
	}
	if header[0] != 1 || string(user) != "alice" || string(pass) != "secret" {
		return fmt.Errorf("invalid SOCKS5 credentials")
	}
	if _, err := w.Write([]byte{1, 0}); err != nil {
		return err
	}
	var request [4]byte
	if _, err := io.ReadFull(r, request[:]); err != nil {
		return err
	}
	if request[0] != 5 || request[1] != 1 {
		return fmt.Errorf("invalid SOCKS5 connect")
	}
	var size int
	switch request[3] {
	case 1:
		size = 4
	case 4:
		size = 16
	case 3:
		n, err := r.ReadByte()
		if err != nil {
			return err
		}
		size = int(n)
	default:
		return fmt.Errorf("invalid SOCKS5 address type")
	}
	address := make([]byte, size+2)
	if _, err := io.ReadFull(r, address); err != nil {
		return err
	}
	_, err = w.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 80})
	return err
}

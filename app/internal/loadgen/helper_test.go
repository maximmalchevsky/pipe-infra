package loadgen

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type pipeAddr string

func (a pipeAddr) Network() string { return "pipe" }
func (a pipeAddr) String() string  { return string(a) }

type pipeListener struct {
	addr      net.Addr
	ch        chan net.Conn
	done      chan struct{}
	closeOnce sync.Once
}

func newPipeListener(addr string) *pipeListener {
	return &pipeListener{
		addr: pipeAddr(addr),
		ch:   make(chan net.Conn, 256),
		done: make(chan struct{}),
	}
}

func (p *pipeListener) Accept() (net.Conn, error) {
	select {
	case c, ok := <-p.ch:
		if !ok {
			return nil, net.ErrClosed
		}
		return c, nil
	case <-p.done:
		return nil, net.ErrClosed
	}
}

func (p *pipeListener) Close() error {
	p.closeOnce.Do(func() {
		close(p.done)
	})
	return nil
}

func (p *pipeListener) Addr() net.Addr {
	return p.addr
}

func (p *pipeListener) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	sConn, cConn := net.Pipe()
	select {
	case p.ch <- sConn:
		return cConn, nil
	case <-p.done:
		_ = sConn.Close()
		_ = cConn.Close()
		return nil, net.ErrClosed
	case <-ctx.Done():
		_ = sConn.Close()
		_ = cConn.Close()
		return nil, ctx.Err()
	}
}

func startTLSTestServer(t *testing.T, handler http.Handler) (*httptest.Server, func(ctx context.Context, network, addr string) (net.Conn, error)) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err == nil {
		ts := &httptest.Server{
			Listener: ln,
			Config:   &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second},
		}
		ts.StartTLS()
		return ts, nil
	}

	pl := newPipeListener("127.0.0.1:443")
	ts := &httptest.Server{
		Listener: pl,
		Config:   &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second},
	}
	ts.StartTLS()
	return ts, pl.DialContext
}

func startHTTPTestServer(t *testing.T, handler http.Handler) (*httptest.Server, func(ctx context.Context, network, addr string) (net.Conn, error)) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err == nil {
		ts := &httptest.Server{
			Listener: ln,
			Config:   &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second},
		}
		ts.Start()
		return ts, nil
	}

	pl := newPipeListener("127.0.0.1:80")
	ts := &httptest.Server{
		Listener: pl,
		Config:   &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second},
	}
	ts.Start()
	return ts, pl.DialContext
}

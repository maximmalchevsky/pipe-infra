// Package loadgen provides statistics collection, HTTP client configuration,
// and workload execution for load generation.
package loadgen

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// ClientConfig specifies configuration for the loadgen HTTP client.
type ClientConfig struct {
	Host        string
	Insecure    bool
	Workers     int
	Timeout     time.Duration
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
}

// hostTransport wraps an http.RoundTripper to inject the Host header into requests.
type hostTransport struct {
	base http.RoundTripper
	host string
}

func (t *hostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.host != "" {
		req = req.Clone(req.Context())
		req.Host = t.host
	}
	return t.base.RoundTrip(req)
}

func (t *hostTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

// NewClient returns a shared *http.Client configured with timeouts,
// connection pooling, TLS settings, and Host header/SNI overrides.
func NewClient(cfg ClientConfig) *http.Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	maxIdle := cfg.Workers
	if maxIdle < 1 {
		maxIdle = 10
	}

	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConnsPerHost = maxIdle
	tr.MaxIdleConns = maxIdle * 4
	tr.IdleConnTimeout = 90 * time.Second
	tr.DisableKeepAlives = false
	tr.DisableCompression = true
	if cfg.DialContext != nil {
		tr.DialContext = cfg.DialContext
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: cfg.Insecure, //nolint:gosec // G402: lab-only, in-cluster synthetic traffic with a private CA
	}
	if cfg.Host != "" {
		serverName := cfg.Host
		if h, _, err := net.SplitHostPort(cfg.Host); err == nil {
			serverName = h
		}
		tlsConfig.ServerName = serverName
	}
	tr.TLSClientConfig = tlsConfig

	var rt http.RoundTripper = tr
	if cfg.Host != "" {
		rt = &hostTransport{
			base: tr,
			host: cfg.Host,
		}
	}

	return &http.Client{
		Transport: rt,
		Timeout:   timeout,
	}
}

// NewHTTPClient creates a shared *http.Client given workers, host override, and insecure flag.
func NewHTTPClient(workers int, host string, insecure bool) *http.Client {
	return NewClient(ClientConfig{
		Workers:  workers,
		Host:     host,
		Insecure: insecure,
	})
}

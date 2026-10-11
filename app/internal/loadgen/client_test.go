package loadgen

import (
	"net/http"
	"testing"
	"time"
)

func TestNewClient_HostAndInsecure(t *testing.T) {
	var (
		receivedHost string
		receivedSNI  string
	)

	server, dialCtx := startTLSTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHost = r.Host
		if r.TLS != nil {
			receivedSNI = r.TLS.ServerName
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 1. With -insecure and -host set:
	// - Insecure accepts the self-signed httptest cert
	// - Host override is applied to r.Host and TLS SNI
	overrideHost := "api.hackathon.lan"
	client := NewClient(ClientConfig{
		Host:        overrideHost,
		Insecure:    true,
		Workers:     2,
		DialContext: dialCtx,
	})

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("expected request with -insecure=true to succeed, got: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got: %d", resp.StatusCode)
	}
	if receivedHost != overrideHost {
		t.Errorf("expected r.Host=%q, got: %q", overrideHost, receivedHost)
	}
	if receivedSNI != overrideHost {
		t.Errorf("expected SNI ServerName=%q, got: %q", overrideHost, receivedSNI)
	}

	// 2. Without -insecure:
	// Request against self-signed httptest server must fail
	secureClient := NewClient(ClientConfig{
		Host:        overrideHost,
		Insecure:    false,
		Workers:     2,
		DialContext: dialCtx,
	})

	_, err = secureClient.Get(server.URL)
	if err == nil {
		t.Fatal("expected request without -insecure to fail on self-signed cert, got nil error")
	}

	// 3. Positional constructor NewHTTPClient works identically
	posClient := NewHTTPClient(4, overrideHost, true)
	if dialCtx != nil {
		posClient = NewClient(ClientConfig{
			Host:        overrideHost,
			Insecure:    true,
			Workers:     4,
			DialContext: dialCtx,
		})
	}
	resp2, err := posClient.Get(server.URL)
	if err != nil {
		t.Fatalf("expected NewHTTPClient to succeed with insecure=true, got: %v", err)
	}
	_ = resp2.Body.Close()
	if receivedHost != overrideHost {
		t.Errorf("expected r.Host=%q via NewHTTPClient, got: %q", overrideHost, receivedHost)
	}
}

func TestNewClient_TimeoutsAndSettings(t *testing.T) {
	client := NewClient(ClientConfig{
		Workers: 16,
	})
	if client.Timeout != 10*time.Second {
		t.Errorf("expected default timeout 10s, got %v", client.Timeout)
	}

	customClient := NewClient(ClientConfig{
		Workers: 8,
		Timeout: 5 * time.Second,
	})
	if customClient.Timeout != 5*time.Second {
		t.Errorf("expected custom timeout 5s, got %v", customClient.Timeout)
	}
}

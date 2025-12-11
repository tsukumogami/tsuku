package httputil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewSecureClient_DefaultOptions(t *testing.T) {
	client := NewSecureClient(ClientOptions{})

	if client.Timeout != 30*time.Second {
		t.Errorf("Expected default timeout 30s, got %v", client.Timeout)
	}

	transport := client.Transport.(*http.Transport)
	if !transport.DisableCompression {
		t.Error("Expected DisableCompression to be true by default")
	}
}

func TestNewSecureClient_CustomTimeout(t *testing.T) {
	opts := ClientOptions{
		Timeout: 5 * time.Minute,
	}
	client := NewSecureClient(opts)

	if client.Timeout != 5*time.Minute {
		t.Errorf("Expected timeout 5m, got %v", client.Timeout)
	}
}

func TestNewSecureClient_Compression(t *testing.T) {
	// Default: compression disabled for security
	client := NewSecureClient(ClientOptions{})
	transport := client.Transport.(*http.Transport)
	if !transport.DisableCompression {
		t.Error("Expected DisableCompression to be true by default")
	}

	// EnableCompression: false explicitly (same as default)
	client2 := NewSecureClient(ClientOptions{EnableCompression: false})
	transport2 := client2.Transport.(*http.Transport)
	if !transport2.DisableCompression {
		t.Error("Expected DisableCompression to be true when EnableCompression=false")
	}

	// EnableCompression: true (opt-in to compression)
	client3 := NewSecureClient(ClientOptions{EnableCompression: true})
	transport3 := client3.Transport.(*http.Transport)
	if transport3.DisableCompression {
		t.Error("Expected DisableCompression to be false when EnableCompression=true")
	}
}

func TestNewSecureClient_RedirectToHTTP_Blocked(t *testing.T) {
	// Create a server that redirects to HTTP
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/evil", http.StatusFound)
	}))
	defer server.Close()

	client := NewSecureClient(ClientOptions{})
	// Use the test server's client transport for TLS
	client.Transport = server.Client().Transport

	// Re-add our redirect checker
	client.CheckRedirect = makeRedirectChecker(10)

	resp, err := client.Get(server.URL)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("Expected error for redirect to HTTP, got nil")
	}

	if !strings.Contains(err.Error(), "non-HTTPS") {
		t.Errorf("Expected 'non-HTTPS' in error, got: %v", err)
	}
}

func TestNewSecureClient_RedirectToPrivateIP_Blocked(t *testing.T) {
	// Create a server that redirects to a private IP
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://192.168.1.1/admin", http.StatusFound)
	}))
	defer server.Close()

	client := NewSecureClient(ClientOptions{})
	client.Transport = server.Client().Transport
	client.CheckRedirect = makeRedirectChecker(10)

	resp, err := client.Get(server.URL)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("Expected error for redirect to private IP, got nil")
	}

	if !strings.Contains(err.Error(), "private") {
		t.Errorf("Expected 'private' in error, got: %v", err)
	}
}

func TestNewSecureClient_RedirectToLoopback_Blocked(t *testing.T) {
	// Create a server that redirects to localhost
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://127.0.0.1/evil", http.StatusFound)
	}))
	defer server.Close()

	client := NewSecureClient(ClientOptions{})
	client.Transport = server.Client().Transport
	client.CheckRedirect = makeRedirectChecker(10)

	resp, err := client.Get(server.URL)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Fatal("Expected error for redirect to loopback, got nil")
	}

	if !strings.Contains(err.Error(), "loopback") {
		t.Errorf("Expected 'loopback' in error, got: %v", err)
	}
}

func TestNewSecureClient_TooManyRedirects(t *testing.T) {
	// Test the redirect checker directly since setting up a server that
	// redirects to itself in HTTPS is complex
	checker := makeRedirectChecker(3)

	// Simulate being called after 3 redirects
	via := make([]*http.Request, 3)
	req, _ := http.NewRequest("GET", "https://example.com/page4", nil)

	err := checker(req, via)
	if err == nil {
		t.Fatal("Expected error for too many redirects, got nil")
	}

	if !strings.Contains(err.Error(), "too many redirects") {
		t.Errorf("Expected 'too many redirects' in error, got: %v", err)
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.Timeout != 30*time.Second {
		t.Errorf("Expected default Timeout 30s, got %v", opts.Timeout)
	}
	if opts.DialTimeout != 30*time.Second {
		t.Errorf("Expected default DialTimeout 30s, got %v", opts.DialTimeout)
	}
	if opts.TLSHandshakeTimeout != 10*time.Second {
		t.Errorf("Expected default TLSHandshakeTimeout 10s, got %v", opts.TLSHandshakeTimeout)
	}
	if opts.MaxRedirects != 10 {
		t.Errorf("Expected default MaxRedirects 10, got %d", opts.MaxRedirects)
	}
	if opts.EnableCompression {
		t.Error("Expected default EnableCompression false")
	}
}

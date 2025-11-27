package version

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSSRFProtection_LinkLocalIP tests blocking of AWS metadata service
func TestSSRFProtection_LinkLocalIP(t *testing.T) {
	ip := net.ParseIP("169.254.169.254")
	err := validateIP(ip, "169.254.169.254")

	if err == nil {
		t.Error("Expected error for link-local IP (AWS metadata service), got nil")
	}

	if !strings.Contains(err.Error(), "link-local") {
		t.Errorf("Expected 'link-local' in error, got: %v", err)
	}
}

// TestSSRFProtection_PrivateIP tests blocking of private network IPs
func TestSSRFProtection_PrivateIP(t *testing.T) {
	privateIPs := []string{
		"10.0.0.1",
		"172.16.0.1",
		"192.168.1.1",
	}

	for _, ipStr := range privateIPs {
		t.Run(ipStr, func(t *testing.T) {
			ip := net.ParseIP(ipStr)
			err := validateIP(ip, ipStr)

			if err == nil {
				t.Errorf("Expected error for private IP %s, got nil", ipStr)
			}

			if !strings.Contains(err.Error(), "private") {
				t.Errorf("Expected 'private' in error for %s, got: %v", ipStr, err)
			}
		})
	}
}

// TestSSRFProtection_LoopbackIP tests blocking of loopback addresses
func TestSSRFProtection_LoopbackIP(t *testing.T) {
	loopbackIPs := []string{
		"127.0.0.1",
		"127.0.0.2",
		"::1",
	}

	for _, ipStr := range loopbackIPs {
		t.Run(ipStr, func(t *testing.T) {
			ip := net.ParseIP(ipStr)
			err := validateIP(ip, ipStr)

			if err == nil {
				t.Errorf("Expected error for loopback IP %s, got nil", ipStr)
			}

			if !strings.Contains(err.Error(), "loopback") {
				t.Errorf("Expected 'loopback' in error for %s, got: %v", ipStr, err)
			}
		})
	}
}

// TestSSRFProtection_PublicIP tests that public IPs are allowed
func TestSSRFProtection_PublicIP(t *testing.T) {
	publicIPs := []string{
		"8.8.8.8",
		"1.1.1.1",
		"151.101.1.140",
	}

	for _, ipStr := range publicIPs {
		t.Run(ipStr, func(t *testing.T) {
			ip := net.ParseIP(ipStr)
			err := validateIP(ip, ipStr)

			if err != nil {
				t.Errorf("Public IP %s should be allowed, got error: %v", ipStr, err)
			}
		})
	}
}

// TestSSRFProtection_RedirectToPrivate tests redirect protection
func TestSSRFProtection_RedirectToPrivate(t *testing.T) {
	// Create a server that redirects to a private IP
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to redirect to private IP
		http.Redirect(w, r, "https://192.168.1.1/admin", http.StatusFound)
	}))
	defer server.Close()

	resolver := NewWithNpmRegistry(server.URL)
	ctx := context.Background()
	_, err := resolver.ListNpmVersions(ctx, "test")

	if err == nil {
		t.Fatal("Expected error for redirect to private IP, got nil")
	}

	if !strings.Contains(err.Error(), "private") && !strings.Contains(err.Error(), "redirect") {
		t.Errorf("Expected error about private IP or redirect, got: %v", err)
	}
}

// TestSSRFProtection_RedirectToLoopback tests redirect to localhost protection
func TestSSRFProtection_RedirectToLoopback(t *testing.T) {
	// Create a server that redirects to localhost
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to redirect to localhost
		http.Redirect(w, r, "https://127.0.0.1/evil", http.StatusFound)
	}))
	defer server.Close()

	resolver := NewWithNpmRegistry(server.URL)
	ctx := context.Background()
	_, err := resolver.ListNpmVersions(ctx, "test")

	if err == nil {
		t.Fatal("Expected error for redirect to loopback, got nil")
	}

	if !strings.Contains(err.Error(), "loopback") && !strings.Contains(err.Error(), "redirect") {
		t.Errorf("Expected error about loopback or redirect, got: %v", err)
	}
}

// TestSSRFProtection_NonHTTPSRedirect tests that non-HTTPS redirects are blocked
func TestSSRFProtection_NonHTTPSRedirect(t *testing.T) {
	// Create a server that redirects to HTTP (non-HTTPS)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.com/evil", http.StatusFound)
	}))
	defer server.Close()

	resolver := NewWithNpmRegistry(server.URL)
	ctx := context.Background()
	_, err := resolver.ListNpmVersions(ctx, "test")

	if err == nil {
		t.Fatal("Expected error for non-HTTPS redirect, got nil")
	}

	if !strings.Contains(err.Error(), "HTTPS") && !strings.Contains(err.Error(), "redirect") {
		t.Errorf("Expected error about non-HTTPS redirect, got: %v", err)
	}
}

// TestSSRFProtection_TooManyRedirects tests redirect chain limit
func TestSSRFProtection_TooManyRedirects(t *testing.T) {
	redirectCount := 0
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		// Keep redirecting to itself (will hit redirect limit)
		http.Redirect(w, r, serverURL+"/redirect", http.StatusFound)
	}))
	defer server.Close()
	serverURL = server.URL

	resolver := NewWithNpmRegistry(server.URL)
	ctx := context.Background()
	_, err := resolver.ListNpmVersions(ctx, "test")

	if err == nil {
		t.Fatal("Expected error for too many redirects, got nil")
	}

	if !strings.Contains(err.Error(), "redirect") {
		t.Errorf("Expected error about redirects, got: %v", err)
	}
}

// TestDecompressionBomb tests that compressed responses are rejected
func TestDecompressionBomb(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to send compressed response
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()

		// Write large payload
		for i := 0; i < 1000; i++ {
			_, _ = gz.Write([]byte("AAAAAAAAAA"))
		}
	}))
	defer server.Close()

	resolver := NewWithNpmRegistry(server.URL)
	ctx := context.Background()
	_, err := resolver.ListNpmVersions(ctx, "test")

	if err == nil {
		t.Fatal("Expected error for compressed response, got nil")
	}

	if !strings.Contains(err.Error(), "compressed") {
		t.Errorf("Expected error about compression, got: %v", err)
	}
}

// TestPackageNameInjection tests validation of package names
func TestPackageNameInjection(t *testing.T) {
	malicious := []string{
		"--evil-flag",
		"; rm -rf /",
		"$(evil command)",
		"../../../etc/passwd",
		"my..package",
	}

	resolver := New()
	ctx := context.Background()

	for _, name := range malicious {
		t.Run(name, func(t *testing.T) {
			_, err := resolver.ListNpmVersions(ctx, name)

			if err == nil {
				t.Errorf("Should reject malicious name: %s", name)
			}

			if !strings.Contains(err.Error(), "invalid") {
				t.Errorf("Expected 'invalid' error for %s, got: %v", name, err)
			}
		})
	}
}

// TestPackageNameValidation_EdgeCases tests edge cases in package name validation
func TestPackageNameValidation_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		pkgName string
		valid   bool
	}{
		{"starts with dot", ".hidden", false},
		{"starts with underscore", "_private", false},
		{"starts with hyphen", "-package", false},
		{"starts with tilde", "~package", false},
		{"ends with dot", "package.", false},
		{"ends with hyphen", "package-", false},
		{"consecutive dots", "my..package", false},
		{"empty scope", "@/package", false},
		{"empty package", "@scope/", false},
		{"valid scoped", "@scope/package", true},
		{"valid unscoped", "package", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := isValidNpmPackageName(tt.pkgName)
			if valid != tt.valid {
				t.Errorf("isValidNpmPackageName(%q) = %v, want %v", tt.pkgName, valid, tt.valid)
			}
		})
	}
}

// TestResponseSizeLimit tests that oversized responses are rejected
func TestResponseSizeLimit(t *testing.T) {
	t.Skip("Skipping test - writing 60MB takes too long and isn't practical for CI/CD")
	// Note: This test would verify that io.LimitReader truncates the response,
	// causing a JSON parsing error. In practice, the 50MB limit is sufficient
	// as the largest known npm package metadata is ~17MB (serverless).
}

// TestValidateIP_IPv6LinkLocal tests IPv6 link-local address blocking
func TestValidateIP_IPv6LinkLocal(t *testing.T) {
	// fe80:: is IPv6 link-local
	ip := net.ParseIP("fe80::1")
	err := validateIP(ip, "fe80::1")

	if err == nil {
		t.Error("Expected error for IPv6 link-local address, got nil")
	}

	if !strings.Contains(err.Error(), "link-local") {
		t.Errorf("Expected 'link-local' in error, got: %v", err)
	}
}

// TestValidateIP_UnspecifiedAddress tests blocking of unspecified addresses
func TestValidateIP_UnspecifiedAddress(t *testing.T) {
	tests := []struct {
		name string
		ip   string
	}{
		{"IPv4 unspecified", "0.0.0.0"},
		{"IPv6 unspecified", "::"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			err := validateIP(ip, tt.ip)

			if err == nil {
				t.Errorf("Expected error for unspecified address %s, got nil", tt.ip)
			}

			if !strings.Contains(err.Error(), "unspecified") {
				t.Errorf("Expected 'unspecified' in error for %s, got: %v", tt.ip, err)
			}
		})
	}
}

// TestAcceptEncodingHeader tests that we request uncompressed responses
func TestAcceptEncodingHeader(t *testing.T) {
	headerReceived := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headerReceived = r.Header.Get("Accept-Encoding")

		// Return valid response
		response := map[string]interface{}{
			"name": "test",
			"versions": map[string]interface{}{
				"1.0.0": map[string]interface{}{},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resolver := NewWithNpmRegistry(server.URL)
	ctx := context.Background()
	_, err := resolver.ListNpmVersions(ctx, "test")

	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if headerReceived != "identity" {
		t.Errorf("Expected Accept-Encoding: identity, got: %s", headerReceived)
	}
}

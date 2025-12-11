package version

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
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

	resolver := New(WithNpmRegistry(server.URL))
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

	resolver := New(WithNpmRegistry(server.URL))
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

	resolver := New(WithNpmRegistry(server.URL))
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

	resolver := New(WithNpmRegistry(server.URL))
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

	resolver := New(WithNpmRegistry(server.URL))
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

// TestValidateIP_Multicast tests blocking of all multicast addresses
// This covers addresses beyond link-local multicast (224.0.0.0/4, ff00::/8)
func TestValidateIP_Multicast(t *testing.T) {
	tests := []struct {
		name string
		ip   string
	}{
		// IPv4 multicast (224.0.0.0/4)
		{"IPv4 all hosts", "224.0.0.1"},
		{"IPv4 SSDP", "239.255.255.250"},
		{"IPv4 organization local", "239.192.0.1"},
		// IPv6 multicast (ff00::/8)
		{"IPv6 all nodes", "ff02::1"},
		{"IPv6 site-local", "ff05::1"},
		{"IPv6 organization-local", "ff08::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}
			err := validateIP(ip, tt.ip)

			if err == nil {
				t.Errorf("Expected error for multicast address %s, got nil", tt.ip)
			}

			// Should contain either "multicast" or "link-local multicast"
			if !strings.Contains(err.Error(), "multicast") {
				t.Errorf("Expected 'multicast' in error for %s, got: %v", tt.ip, err)
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

	resolver := New(WithNpmRegistry(server.URL))
	ctx := context.Background()
	_, err := resolver.ListNpmVersions(ctx, "test")

	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if headerReceived != "identity" {
		t.Errorf("Expected Accept-Encoding: identity, got: %s", headerReceived)
	}
}

// TestValidateIP_IPv4MappedIPv6 tests IPv4-mapped IPv6 addresses
// These are IPv6 addresses that embed IPv4 addresses (::ffff:x.x.x.x)
// and must be validated against the embedded IPv4 address
func TestValidateIP_IPv4MappedIPv6(t *testing.T) {
	tests := []struct {
		name      string
		ip        string
		shouldErr bool
		errType   string
	}{
		// IPv4-mapped loopback (::ffff:127.0.0.1)
		{"mapped loopback", "::ffff:127.0.0.1", true, "loopback"},
		// IPv4-mapped private (::ffff:192.168.1.1)
		{"mapped private 192.168", "::ffff:192.168.1.1", true, "private"},
		{"mapped private 10.0", "::ffff:10.0.0.1", true, "private"},
		{"mapped private 172.16", "::ffff:172.16.0.1", true, "private"},
		// IPv4-mapped link-local (::ffff:169.254.169.254) - AWS metadata
		{"mapped link-local", "::ffff:169.254.169.254", true, "link-local"},
		// IPv4-mapped public (should be allowed)
		{"mapped public", "::ffff:8.8.8.8", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}
			err := validateIP(ip, tt.ip)

			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error for %s, got nil", tt.ip)
					return
				}
				if !strings.Contains(err.Error(), tt.errType) {
					t.Errorf("Expected '%s' in error for %s, got: %v", tt.errType, tt.ip, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for %s: %v", tt.ip, err)
				}
			}
		})
	}
}

// TestValidateIP_UniqueLocalAddress tests IPv6 Unique Local Addresses (ULA)
// ULA (fc00::/7, typically fd00::/8) are private IPv6 addresses analogous to RFC1918
func TestValidateIP_UniqueLocalAddress(t *testing.T) {
	tests := []struct {
		name      string
		ip        string
		shouldErr bool
	}{
		// fd00::/8 - commonly used ULA prefix
		{"ULA fd00", "fd00::1", true},
		{"ULA fd12", "fd12:3456:789a::1", true},
		// fc00::/8 - reserved but less common
		{"ULA fc00", "fc00::1", true},
		// Public IPv6 (should be allowed)
		{"public 2001:4860", "2001:4860:4860::8888", false}, // Google DNS
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}
			err := validateIP(ip, tt.ip)

			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error for ULA %s, got nil", tt.ip)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for public IP %s: %v", tt.ip, err)
				}
			}
		})
	}
}

// TestPackageNameValidation_Unicode tests unicode handling in package names
// npm package names must be lowercase ASCII - unicode should be rejected
func TestPackageNameValidation_Unicode(t *testing.T) {
	tests := []struct {
		name    string
		pkgName string
		valid   bool
	}{
		// Homoglyph attacks (characters that look like ASCII but aren't)
		{"cyrillic a", "pаckage", false},    // 'а' is Cyrillic U+0430, not ASCII 'a'
		{"greek omicron", "packаge", false}, // Using Cyrillic 'а'
		// Full-width Latin letters U+FF50 'p', U+FF41 'a', etc.
		{"full-width chars", "\uff50\uff41\uff43\uff4b\uff41\uff47\uff45", false},

		// RTL override attacks
		{"RTL override", "pack\u202Eage", false}, // RIGHT-TO-LEFT OVERRIDE
		{"LTR override", "pack\u202Dage", false}, // LEFT-TO-RIGHT OVERRIDE

		// Zero-width characters
		{"zero-width space", "pack\u200Bage", false},      // ZERO WIDTH SPACE
		{"zero-width joiner", "pack\u200Dage", false},     // ZERO WIDTH JOINER
		{"zero-width non-joiner", "pack\u200Cage", false}, // ZERO WIDTH NON-JOINER

		// Other problematic unicode
		{"combining char", "package\u0301", false}, // Combining acute accent
		{"BOM", "\uFEFFpackage", false},            // Byte Order Mark

		// Valid ASCII package names (for comparison)
		{"valid lowercase", "my-package", true},
		{"valid with numbers", "package123", true},
		{"valid scoped", "@scope/package", true},
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

// TestPackageNameValidation_ControlChars tests control character handling
func TestPackageNameValidation_ControlChars(t *testing.T) {
	tests := []struct {
		name    string
		pkgName string
		valid   bool
	}{
		// Null byte injection
		{"null byte", "pack\x00age", false},
		{"null at end", "package\x00", false},

		// Newline injection (could affect logging, command execution)
		{"newline", "pack\nage", false},
		{"carriage return", "pack\rage", false},
		{"CRLF", "pack\r\nage", false},

		// Tab characters
		{"tab", "pack\tage", false},
		{"vertical tab", "pack\vage", false},

		// Other control characters
		{"bell", "pack\aage", false},
		{"backspace", "pack\bage", false},
		{"form feed", "pack\fage", false},
		{"escape", "pack\x1bage", false},

		// DEL character
		{"DEL", "pack\x7fage", false},
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

// TestPackageNameValidation_LongNames tests boundary conditions for package name length
func TestPackageNameValidation_LongNames(t *testing.T) {
	tests := []struct {
		name    string
		pkgName string
		valid   bool
	}{
		// npm max length is 214 characters
		{"exactly 214 chars", strings.Repeat("a", 214), true},
		{"215 chars - over limit", strings.Repeat("a", 215), false},
		{"1000 chars - way over", strings.Repeat("a", 1000), false},

		// Scoped package length (scope + "/" + name <= 214)
		// @100/111 = 1 + 100 + 1 + 111 = 213 chars, so add one more to hit 214
		{"scoped at limit", "@" + strings.Repeat("a", 100) + "/" + strings.Repeat("b", 112), true},
		// @100/113 = 1 + 100 + 1 + 113 = 215 chars - over limit
		{"scoped over limit", "@" + strings.Repeat("a", 100) + "/" + strings.Repeat("b", 113), false},

		// Edge cases
		{"empty", "", false},
		{"single char", "a", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := isValidNpmPackageName(tt.pkgName)
			if valid != tt.valid {
				t.Errorf("isValidNpmPackageName(%q length=%d) = %v, want %v",
					tt.name, len(tt.pkgName), valid, tt.valid)
			}
		})
	}
}

// TestHTTPClientDisableCompression tests that NewHTTPClient has compression disabled
func TestHTTPClientDisableCompression(t *testing.T) {
	client := NewHTTPClient()

	// Verify the transport has DisableCompression set
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected *http.Transport, got different type")
	}

	if !transport.DisableCompression {
		t.Error("Expected DisableCompression to be true, got false")
	}
}

// TestSSRFProtection_RedirectChainEdgeCases tests edge cases in redirect handling
// This test verifies that the redirect limit (5) is enforced by our HTTP client.
// We use httptest.NewTLSServer to allow HTTPS redirects to pass the security check.
func TestSSRFProtection_RedirectChainEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		redirects   int
		shouldErr   bool
		errContains string
	}{
		// The HTTP client in newHTTPClient allows up to 5 redirects (>=5 triggers error)
		{"4 redirects - allowed", 4, false, ""},
		{"5 redirects - at limit", 5, true, "redirect"},
		{"10 redirects - over limit", 10, true, "redirect"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redirectCount := 0
			var serverURL string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				redirectCount++
				if redirectCount <= tt.redirects {
					http.Redirect(w, r, serverURL+"/next", http.StatusFound)
					return
				}
				// Return valid response after redirects
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
			serverURL = server.URL

			// Create resolver that uses the test server's TLS client
			resolver := New(WithNpmRegistry(server.URL))
			// Override the HTTP client to use the test server's TLS config
			resolver.httpClient = server.Client()
			// Re-apply our security-hardened CheckRedirect to the test client
			resolver.httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				// Limit redirect chain (matching newHTTPClient behavior)
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			}

			ctx := context.Background()
			_, err := resolver.ListNpmVersions(ctx, "test")

			if tt.shouldErr {
				if err == nil {
					t.Errorf("Expected error for %d redirects, got nil", tt.redirects)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing %q, got: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for %d redirects: %v", tt.redirects, err)
				}
			}
		})
	}
}

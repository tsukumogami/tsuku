package version

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
)

// NewHTTPClient creates an HTTP client with security hardening and proper timeouts.
// The timeout is configurable via TSUKU_API_TIMEOUT environment variable (default: 30s).
//
// Security features:
//   - DisableCompression: true - prevents decompression bomb attacks
//   - SSRF protection via redirect validation (blocks private, loopback, link-local IPs)
//   - DNS rebinding protection (resolves hostnames and validates all IPs)
//   - HTTPS-only redirects
//   - Redirect chain limit (5 redirects max)
//
// This function is exported for use by other packages that need secure HTTP clients.
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: config.GetAPITimeout(),
		Transport: &http.Transport{
			DisableCompression: true, // CRITICAL: Prevents decompression bomb attacks
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			DisableKeepAlives:     false,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// 1. Prevent redirect to non-HTTPS
			if req.URL.Scheme != "https" {
				return fmt.Errorf("refusing redirect to non-HTTPS URL: %s", req.URL)
			}

			// 2. Limit redirect chain
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}

			// 3. SSRF Protection: Check redirect target
			host := req.URL.Hostname()

			// 3a. If hostname is already an IP, check it directly
			if ip := net.ParseIP(host); ip != nil {
				if err := validateIP(ip, host); err != nil {
					return err
				}
			} else {
				// 3b. Hostname is a domain - resolve DNS and check ALL resulting IPs
				// This prevents DNS rebinding attacks where evil.com resolves to 127.0.0.1
				ips, err := net.LookupIP(host)
				if err != nil {
					return fmt.Errorf("failed to resolve redirect host %s: %w", host, err)
				}

				for _, ip := range ips {
					if err := validateIP(ip, host); err != nil {
						return fmt.Errorf("refusing redirect: %s resolves to blocked IP %s", host, ip)
					}
				}
			}

			return nil
		},
	}
}

// validateIP checks if an IP is allowed (not private, loopback, link-local, etc.)
func validateIP(ip net.IP, host string) error {
	// Block private IPs (RFC 1918: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
	if ip.IsPrivate() {
		return fmt.Errorf("refusing redirect to private IP: %s (%s)", host, ip)
	}

	// Block loopback (127.0.0.0/8, ::1)
	if ip.IsLoopback() {
		return fmt.Errorf("refusing redirect to loopback IP: %s (%s)", host, ip)
	}

	// Block link-local unicast (169.254.0.0/16, fe80::/10)
	// CRITICAL: This includes AWS metadata service at 169.254.169.254
	if ip.IsLinkLocalUnicast() {
		return fmt.Errorf("refusing redirect to link-local IP: %s (%s)", host, ip)
	}

	// Block link-local multicast (224.0.0.0/24, ff02::/16)
	if ip.IsLinkLocalMulticast() {
		return fmt.Errorf("refusing redirect to link-local multicast: %s (%s)", host, ip)
	}

	// Block ALL multicast addresses (224.0.0.0/4 for IPv4, ff00::/8 for IPv6)
	// This is broader than link-local multicast and blocks site-local, organization-local, etc.
	if ip.IsMulticast() {
		return fmt.Errorf("refusing redirect to multicast IP: %s (%s)", host, ip)
	}

	// Block unspecified address (0.0.0.0, ::)
	if ip.IsUnspecified() {
		return fmt.Errorf("refusing redirect to unspecified IP: %s (%s)", host, ip)
	}

	return nil
}

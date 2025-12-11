package httputil

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// ClientOptions configures the secure HTTP client.
type ClientOptions struct {
	// Timeout is the overall request timeout. Default: 30s.
	Timeout time.Duration

	// DialTimeout is the TCP dial timeout. Default: 30s.
	DialTimeout time.Duration

	// TLSHandshakeTimeout is the TLS handshake timeout. Default: 10s.
	TLSHandshakeTimeout time.Duration

	// ResponseHeaderTimeout is the time to wait for response headers. Default: 10s.
	ResponseHeaderTimeout time.Duration

	// MaxRedirects is the maximum redirect depth. Default: 10.
	MaxRedirects int

	// EnableCompression enables Accept-Encoding header. Default: false (disabled for security).
	// Keeping compression disabled prevents decompression bomb attacks.
	EnableCompression bool

	// MaxIdleConns is the maximum number of idle connections. Default: 10.
	MaxIdleConns int

	// IdleConnTimeout is how long idle connections stay open. Default: 90s.
	IdleConnTimeout time.Duration
}

// DefaultOptions returns the default client options with security-focused defaults.
func DefaultOptions() ClientOptions {
	return ClientOptions{
		Timeout:               30 * time.Second,
		DialTimeout:           30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxRedirects:          10,
		EnableCompression:     false, // Disabled for security (decompression bomb protection)
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
	}
}

// NewSecureClient creates an HTTP client with SSRF protection and security hardening.
//
// Security features:
//   - DisableCompression: true by default - prevents decompression bomb attacks
//   - SSRF protection via redirect validation (blocks private, loopback, link-local IPs)
//   - DNS rebinding protection (resolves hostnames and validates all IPs)
//   - HTTPS-only redirects
//   - Configurable redirect chain limit
func NewSecureClient(opts ClientOptions) *http.Client {
	// Apply defaults for zero values
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.DialTimeout == 0 {
		opts.DialTimeout = 30 * time.Second
	}
	if opts.TLSHandshakeTimeout == 0 {
		opts.TLSHandshakeTimeout = 10 * time.Second
	}
	if opts.ResponseHeaderTimeout == 0 {
		opts.ResponseHeaderTimeout = 10 * time.Second
	}
	if opts.MaxRedirects == 0 {
		opts.MaxRedirects = 10
	}
	if opts.MaxIdleConns == 0 {
		opts.MaxIdleConns = 10
	}
	if opts.IdleConnTimeout == 0 {
		opts.IdleConnTimeout = 90 * time.Second
	}

	// DisableCompression is the inverse of EnableCompression.
	// By default (EnableCompression=false), we disable compression for security.
	disableCompression := !opts.EnableCompression

	return &http.Client{
		Timeout: opts.Timeout,
		Transport: &http.Transport{
			DisableCompression: disableCompression,
			DialContext: (&net.Dialer{
				Timeout:   opts.DialTimeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   opts.TLSHandshakeTimeout,
			ResponseHeaderTimeout: opts.ResponseHeaderTimeout,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConns:          opts.MaxIdleConns,
			IdleConnTimeout:       opts.IdleConnTimeout,
		},
		CheckRedirect: makeRedirectChecker(opts.MaxRedirects),
	}
}

// makeRedirectChecker creates a redirect validation function.
func makeRedirectChecker(maxRedirects int) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		// SECURITY: Prevent redirect downgrade attacks (HTTPS -> HTTP)
		if req.URL.Scheme != "https" {
			return fmt.Errorf("redirect to non-HTTPS URL is not allowed: %s", req.URL)
		}

		// Limit redirect depth
		if len(via) >= maxRedirects {
			return fmt.Errorf("too many redirects")
		}

		// SSRF Protection: Check redirect target
		host := req.URL.Hostname()

		// If hostname is already an IP, check it directly
		if ip := net.ParseIP(host); ip != nil {
			if err := ValidateIP(ip, host); err != nil {
				return err
			}
		} else {
			// Hostname is a domain - resolve DNS and check ALL resulting IPs
			// This prevents DNS rebinding attacks
			ips, err := net.LookupIP(host)
			if err != nil {
				return fmt.Errorf("failed to resolve redirect host %s: %w", host, err)
			}

			for _, ip := range ips {
				if err := ValidateIP(ip, host); err != nil {
					return fmt.Errorf("refusing redirect: %s resolves to blocked IP %s", host, ip)
				}
			}
		}

		return nil
	}
}

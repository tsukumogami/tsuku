package httputil

import (
	"net"
	"strings"
	"testing"
)

func TestValidateIP_LinkLocal(t *testing.T) {
	// AWS metadata service
	ip := net.ParseIP("169.254.169.254")
	err := ValidateIP(ip, "169.254.169.254")

	if err == nil {
		t.Error("Expected error for link-local IP (AWS metadata service), got nil")
	}

	if !strings.Contains(err.Error(), "link-local") {
		t.Errorf("Expected 'link-local' in error, got: %v", err)
	}
}

func TestValidateIP_Private(t *testing.T) {
	privateIPs := []string{
		"10.0.0.1",
		"10.255.255.255",
		"172.16.0.1",
		"172.31.255.255",
		"192.168.0.1",
		"192.168.255.255",
	}

	for _, ipStr := range privateIPs {
		t.Run(ipStr, func(t *testing.T) {
			ip := net.ParseIP(ipStr)
			err := ValidateIP(ip, ipStr)

			if err == nil {
				t.Errorf("Expected error for private IP %s, got nil", ipStr)
			}

			if !strings.Contains(err.Error(), "private") {
				t.Errorf("Expected 'private' in error for %s, got: %v", ipStr, err)
			}
		})
	}
}

func TestValidateIP_Loopback(t *testing.T) {
	loopbackIPs := []string{
		"127.0.0.1",
		"127.0.0.2",
		"127.255.255.255",
		"::1",
	}

	for _, ipStr := range loopbackIPs {
		t.Run(ipStr, func(t *testing.T) {
			ip := net.ParseIP(ipStr)
			err := ValidateIP(ip, ipStr)

			if err == nil {
				t.Errorf("Expected error for loopback IP %s, got nil", ipStr)
			}

			if !strings.Contains(err.Error(), "loopback") {
				t.Errorf("Expected 'loopback' in error for %s, got: %v", ipStr, err)
			}
		})
	}
}

func TestValidateIP_Multicast(t *testing.T) {
	multicastIPs := []string{
		"224.0.0.1",
		"239.255.255.255",
		"ff00::1",
	}

	for _, ipStr := range multicastIPs {
		t.Run(ipStr, func(t *testing.T) {
			ip := net.ParseIP(ipStr)
			err := ValidateIP(ip, ipStr)

			if err == nil {
				t.Errorf("Expected error for multicast IP %s, got nil", ipStr)
			}

			// Could be "multicast" or "link-local multicast"
			if !strings.Contains(err.Error(), "multicast") {
				t.Errorf("Expected 'multicast' in error for %s, got: %v", ipStr, err)
			}
		})
	}
}

func TestValidateIP_Unspecified(t *testing.T) {
	unspecifiedIPs := []string{
		"0.0.0.0",
		"::",
	}

	for _, ipStr := range unspecifiedIPs {
		t.Run(ipStr, func(t *testing.T) {
			ip := net.ParseIP(ipStr)
			err := ValidateIP(ip, ipStr)

			if err == nil {
				t.Errorf("Expected error for unspecified IP %s, got nil", ipStr)
			}

			if !strings.Contains(err.Error(), "unspecified") {
				t.Errorf("Expected 'unspecified' in error for %s, got: %v", ipStr, err)
			}
		})
	}
}

func TestValidateIP_Public(t *testing.T) {
	publicIPs := []string{
		"8.8.8.8",
		"1.1.1.1",
		"151.101.1.140",
		"185.199.108.153",          // GitHub
		"2607:f8b0:4004:800::200e", // Google IPv6
	}

	for _, ipStr := range publicIPs {
		t.Run(ipStr, func(t *testing.T) {
			ip := net.ParseIP(ipStr)
			err := ValidateIP(ip, ipStr)

			if err != nil {
				t.Errorf("Public IP %s should be allowed, got error: %v", ipStr, err)
			}
		})
	}
}

func TestValidateIP_HostIncludedInError(t *testing.T) {
	ip := net.ParseIP("127.0.0.1")
	err := ValidateIP(ip, "evil.com")

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !strings.Contains(err.Error(), "evil.com") {
		t.Errorf("Expected hostname in error, got: %v", err)
	}
}

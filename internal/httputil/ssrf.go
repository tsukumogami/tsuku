package httputil

import (
	"fmt"
	"net"
)

// ValidateIP checks if an IP address is allowed for HTTP requests.
// Returns an error if the IP is:
//   - Private (RFC 1918: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
//   - Loopback (127.0.0.0/8, ::1)
//   - Link-local unicast (169.254.0.0/16, fe80::/10) - includes AWS metadata service
//   - Link-local multicast (224.0.0.0/24, ff02::/16)
//   - Multicast (224.0.0.0/4 for IPv4, ff00::/8 for IPv6)
//   - Unspecified (0.0.0.0, ::)
//
// The host parameter is included in error messages for debugging.
func ValidateIP(ip net.IP, host string) error {
	if ip.IsPrivate() {
		return fmt.Errorf("refusing redirect to private IP: %s (%s)", host, ip)
	}
	if ip.IsLoopback() {
		return fmt.Errorf("refusing redirect to loopback IP: %s (%s)", host, ip)
	}
	if ip.IsLinkLocalUnicast() {
		return fmt.Errorf("refusing redirect to link-local IP: %s (%s)", host, ip)
	}
	if ip.IsLinkLocalMulticast() {
		return fmt.Errorf("refusing redirect to link-local multicast: %s (%s)", host, ip)
	}
	if ip.IsMulticast() {
		return fmt.Errorf("refusing redirect to multicast IP: %s (%s)", host, ip)
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("refusing redirect to unspecified IP: %s (%s)", host, ip)
	}
	return nil
}

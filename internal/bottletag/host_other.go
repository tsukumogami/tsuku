//go:build !darwin

package bottletag

// hostMacOSVersion returns 0: the host isn't macOS.
func hostMacOSVersion() int { return 0 }

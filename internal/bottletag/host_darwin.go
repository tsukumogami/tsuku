//go:build darwin

package bottletag

import "syscall"

// hostMacOSVersion reads the product version from the kernel. The sysctl
// goes through libSystem, so it works in a binary built without cgo.
func hostMacOSVersion() int {
	v, err := syscall.Sysctl("kern.osproductversion")
	if err != nil {
		return 0
	}
	return parseMacOSVersion(v)
}

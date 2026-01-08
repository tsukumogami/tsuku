// Package platform provides targeting types for plan generation.
//
// This package defines the Target struct which represents the platform
// and Linux family tuple used for filtering installation plans. It is
// distinct from executor.Platform which represents static plan metadata.
package platform

import "strings"

// ValidLinuxFamilies lists the recognized linux_family values.
// Each family corresponds to a package manager ecosystem:
//   - debian: apt (Ubuntu, Debian, Mint, Pop!_OS)
//   - rhel: dnf (Fedora, RHEL, CentOS, Rocky, Alma)
//   - arch: pacman (Arch Linux, Manjaro)
//   - alpine: apk (Alpine Linux)
//   - suse: zypper (openSUSE, SLES)
var ValidLinuxFamilies = []string{"debian", "rhel", "arch", "alpine", "suse"}

// Target represents the platform being targeted for plan generation.
// It combines platform (os/arch) with linux_family for filtering
// package manager actions.
type Target struct {
	// Platform is the combined os/arch string (e.g., "linux/amd64", "darwin/arm64").
	Platform string

	// linuxFamily identifies the Linux distribution family (e.g., "debian", "rhel").
	// This is only set when OS is "linux". For other operating systems
	// (darwin, windows), this field is empty.
	// Access via LinuxFamily() method.
	linuxFamily string
}

// NewTarget creates a Target with the given platform and linux family.
func NewTarget(platform, linuxFamily string) Target {
	return Target{
		Platform:    platform,
		linuxFamily: linuxFamily,
	}
}

// OS returns the operating system from the Platform field.
// For "linux/amd64" it returns "linux".
// Returns empty string if Platform is empty or malformed.
func (t Target) OS() string {
	if t.Platform == "" {
		return ""
	}
	parts := strings.SplitN(t.Platform, "/", 2)
	return parts[0]
}

// Arch returns the architecture from the Platform field.
// For "linux/amd64" it returns "amd64".
// Returns empty string if Platform is empty or malformed.
func (t Target) Arch() string {
	if t.Platform == "" {
		return ""
	}
	parts := strings.SplitN(t.Platform, "/", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// LinuxFamily returns the Linux distribution family.
// Returns empty string for non-Linux platforms.
func (t Target) LinuxFamily() string {
	return t.linuxFamily
}

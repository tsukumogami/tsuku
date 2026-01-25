package platform

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"strings"
)

// OSRelease contains parsed values from /etc/os-release.
type OSRelease struct {
	ID              string   // Canonical distro identifier (e.g., "ubuntu", "fedora")
	IDLike          []string // Parent/similar distros (e.g., ["debian"] for Ubuntu)
	VersionID       string   // Version number (e.g., "22.04")
	VersionCodename string   // Codename (e.g., "jammy")
}

// distroToFamily maps distro IDs to linux_family values.
// The family corresponds to the package manager ecosystem.
var distroToFamily = map[string]string{
	// Debian family (apt)
	"debian": "debian", "ubuntu": "debian", "linuxmint": "debian",
	"pop": "debian", "elementary": "debian", "zorin": "debian",
	// RHEL family (dnf)
	"fedora": "rhel", "rhel": "rhel", "centos": "rhel",
	"rocky": "rhel", "almalinux": "rhel", "ol": "rhel",
	// Arch family (pacman)
	"arch": "arch", "manjaro": "arch", "endeavouros": "arch",
	// Alpine (apk)
	"alpine": "alpine",
	// SUSE family (zypper)
	"opensuse":            "suse",
	"opensuse-leap":       "suse",
	"opensuse-tumbleweed": "suse",
	"sles":                "suse",
}

// ParseOSRelease parses the /etc/os-release file format.
// Returns an error if the file cannot be read or parsed.
func ParseOSRelease(path string) (*OSRelease, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	release := &OSRelease{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		// Remove quotes from value
		value = strings.Trim(value, `"'`)

		switch key {
		case "ID":
			release.ID = value
		case "ID_LIKE":
			// ID_LIKE is space-separated
			release.IDLike = strings.Fields(value)
		case "VERSION_ID":
			release.VersionID = value
		case "VERSION_CODENAME":
			release.VersionCodename = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return release, nil
}

// MapDistroToFamily maps a distro ID to its linux_family.
// Falls back to ID_LIKE chain if ID is not directly recognized.
// Returns an error if the distro cannot be mapped to a family.
func MapDistroToFamily(id string, idLike []string) (string, error) {
	// Try ID first
	if family, ok := distroToFamily[id]; ok {
		return family, nil
	}

	// Fall back to ID_LIKE chain
	for _, like := range idLike {
		if family, ok := distroToFamily[like]; ok {
			return family, nil
		}
	}

	return "", fmt.Errorf("unknown distro: %s", id)
}

// DetectFamily returns the linux_family for the current system.
// Returns empty string and nil error if /etc/os-release is missing.
// Returns an error if the file exists but family cannot be determined.
func DetectFamily() (string, error) {
	osRelease, err := ParseOSRelease("/etc/os-release")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}

	return MapDistroToFamily(osRelease.ID, osRelease.IDLike)
}

// DetectTarget returns the full target tuple for the current host.
// For non-Linux platforms, returns Target with empty LinuxFamily and Libc.
func DetectTarget() (Target, error) {
	platform := runtime.GOOS + "/" + runtime.GOARCH
	if runtime.GOOS != "linux" {
		return Target{Platform: platform}, nil
	}

	family, err := DetectFamily()
	if err != nil {
		return Target{}, err
	}

	libc := DetectLibc()
	return NewTarget(platform, family, libc), nil
}

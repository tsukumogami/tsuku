package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ContainerSpec defines a container specification for building test environments.
// It maps package requirements to a base container image and build commands.
type ContainerSpec struct {
	BaseImage     string              // Base container image (e.g., "debian:bookworm-slim")
	LinuxFamily   string              // Linux family (e.g., "debian", "rhel", "arch")
	Packages      map[string][]string // Package manager to package list mapping
	BuildCommands []string            // Docker RUN commands to install packages
}

// Package manager to linux_family mapping.
// Each PM corresponds to exactly one family.
var pmToFamily = map[string]string{
	"apt":    "debian",
	"dnf":    "rhel",
	"pacman": "arch",
	"apk":    "alpine",
	"zypper": "suse",
}

// Linux family to base container image mapping.
// Base images are selected for:
// - Small size (slim/minimal variants where available)
// - Stability (current stable releases, not rolling)
// - Official maintenance (from distribution maintainers)
//
// debian: bookworm-slim is Debian 12 (current stable)
// rhel: fedora:41 provides dnf and modern tooling
// arch: archlinux:base is the minimal Arch base
// alpine: alpine:3.19 is current stable
// suse: opensuse/leap:15 provides zypper (Leap 15 is current stable)
var familyToBaseImage = map[string]string{
	"debian": "debian:bookworm-slim",
	"rhel":   "fedora:41",
	"arch":   "archlinux:base",
	"alpine": "alpine:3.19",
	"suse":   "opensuse/leap:15",
}

// DeriveContainerSpec creates a container specification from extracted packages.
//
// The packages map comes from ExtractPackages() and contains package manager names
// as keys (e.g., "apt", "dnf") and package lists as values.
//
// This function:
// - Infers the linux_family from the package manager(s) present
// - Selects an appropriate base image for that family
// - Generates Dockerfile RUN commands to install the packages
//
// Returns nil if packages is nil or empty (no system dependencies needed).
//
// Returns an error if:
// - Packages use incompatible package managers (e.g., both apt and dnf)
// - Packages use a package manager not applicable to containers (e.g., brew)
// - The linux_family cannot be determined
//
// Example:
//
//	packages := map[string][]string{"apt": {"curl", "jq"}}
//	spec, err := DeriveContainerSpec(packages)
//	// spec.BaseImage = "debian:bookworm-slim"
//	// spec.LinuxFamily = "debian"
//	// spec.BuildCommands = ["RUN apt-get update && apt-get install -y curl jq"]
func DeriveContainerSpec(packages map[string][]string) (*ContainerSpec, error) {
	if len(packages) == 0 {
		return nil, nil
	}

	// Extract package managers from the map
	var pms []string
	for pm := range packages {
		pms = append(pms, pm)
	}
	sort.Strings(pms) // Deterministic ordering for error messages

	// Validate: all PMs must map to the same family
	var family string
	for _, pm := range pms {
		pmFamily, ok := pmToFamily[pm]
		if !ok {
			return nil, fmt.Errorf("package manager %q not applicable to Linux containers", pm)
		}

		if family == "" {
			family = pmFamily
		} else if family != pmFamily {
			return nil, fmt.Errorf("incompatible package managers: %v require different Linux families", pms)
		}
	}

	// Select base image for the family
	baseImage, ok := familyToBaseImage[family]
	if !ok {
		return nil, fmt.Errorf("no base image configured for linux_family %q", family)
	}

	// Generate build commands
	buildCommands, err := generateBuildCommands(family, packages)
	if err != nil {
		return nil, err
	}

	return &ContainerSpec{
		BaseImage:     baseImage,
		LinuxFamily:   family,
		Packages:      packages,
		BuildCommands: buildCommands,
	}, nil
}

// generateBuildCommands creates Docker RUN commands for package installation.
func generateBuildCommands(family string, packages map[string][]string) ([]string, error) {
	var commands []string

	switch family {
	case "debian":
		// Debian/Ubuntu use apt-get
		// Update package lists first, then install
		pkgs := packages["apt"]
		if len(pkgs) > 0 {
			sort.Strings(pkgs) // Deterministic order
			pkgList := strings.Join(pkgs, " ")
			commands = append(commands, fmt.Sprintf("RUN apt-get update && apt-get install -y %s", pkgList))
		}

	case "rhel":
		// Fedora/RHEL use dnf
		pkgs := packages["dnf"]
		if len(pkgs) > 0 {
			sort.Strings(pkgs)
			pkgList := strings.Join(pkgs, " ")
			commands = append(commands, fmt.Sprintf("RUN dnf install -y %s", pkgList))
		}

	case "arch":
		// Arch uses pacman
		pkgs := packages["pacman"]
		if len(pkgs) > 0 {
			sort.Strings(pkgs)
			pkgList := strings.Join(pkgs, " ")
			commands = append(commands, fmt.Sprintf("RUN pacman -Sy --noconfirm %s", pkgList))
		}

	case "alpine":
		// Alpine uses apk
		pkgs := packages["apk"]
		if len(pkgs) > 0 {
			sort.Strings(pkgs)
			pkgList := strings.Join(pkgs, " ")
			commands = append(commands, fmt.Sprintf("RUN apk add --no-cache %s", pkgList))
		}

	case "suse":
		// SUSE uses zypper
		pkgs := packages["zypper"]
		if len(pkgs) > 0 {
			sort.Strings(pkgs)
			pkgList := strings.Join(pkgs, " ")
			commands = append(commands, fmt.Sprintf("RUN zypper install -y %s", pkgList))
		}

	default:
		return nil, fmt.Errorf("unknown linux_family %q", family)
	}

	if len(commands) == 0 {
		return nil, errors.New("no packages found for the detected family")
	}

	return commands, nil
}

// ContainerImageName generates a deterministic cache image name for a container specification.
//
// The function produces names like "tsuku/sandbox-cache:debian-a1b2c3d4e5f6g7h8" where:
// - "debian" is the linux_family from the spec (for human readability)
// - "a1b2c3d4e5f6g7h8" is the first 16 hex characters of the SHA256 hash
//
// The hash is computed from all package manager + package combinations, sorted
// deterministically to ensure the same package set always produces the same hash.
// This enables container image caching: if two test runs require the same packages,
// they can reuse the same cached container image.
//
// Hash input format: sorted list of "pm:package" strings joined with newlines.
// Example: For packages {"apt": ["curl", "jq"]}, the hash input is "apt:curl\napt:jq".
//
// The family prefix in the tag is technically redundant (the hash encodes the package
// manager, which determines the family), but improves human readability when debugging
// or managing images manually.
func ContainerImageName(spec *ContainerSpec) string {
	// Extract and sort all pm:package pairs for deterministic hashing
	var parts []string
	for manager, pkgs := range spec.Packages {
		// Sort packages within each manager
		sorted := make([]string, len(pkgs))
		copy(sorted, pkgs)
		sort.Strings(sorted)

		// Create pm:package pairs
		for _, pkg := range sorted {
			parts = append(parts, fmt.Sprintf("%s:%s", manager, pkg))
		}
	}

	// Sort all parts to ensure deterministic ordering regardless of map iteration
	sort.Strings(parts)

	// Compute SHA256 hash of the sorted parts
	hashInput := strings.Join(parts, "\n")
	hash := sha256.Sum256([]byte(hashInput))

	// Use first 16 hex characters (64 bits) for the tag
	// This provides sufficient uniqueness for realistic package combinations
	hashStr := hex.EncodeToString(hash[:])[:16]

	// Return image name with family prefix for readability
	return fmt.Sprintf("tsuku/sandbox-cache:%s-%s", spec.LinuxFamily, hashStr)
}

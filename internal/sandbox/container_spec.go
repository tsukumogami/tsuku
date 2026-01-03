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
	Repositories  []RepositoryConfig  // Repository configurations
	BuildCommands []string            // Docker RUN commands to install packages and setup repositories
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

// DeriveContainerSpec creates a container specification from system requirements.
//
// This function:
// - Infers the linux_family from the package manager(s) and repository types
// - Selects an appropriate base image for that family
// - Generates Docker RUN commands for repository setup (with GPG verification) and package installation
//
// Returns nil if reqs is nil or has no packages/repositories.
//
// Returns an error if:
// - Packages use incompatible package managers (e.g., both apt and dnf)
// - Packages use a package manager not applicable to containers (e.g., brew on Linux)
// - The linux_family cannot be determined
//
// Example:
//
//	reqs := &SystemRequirements{
//	    Packages: map[string][]string{"apt": {"curl", "jq"}},
//	    Repositories: []RepositoryConfig{{Manager: "apt", Type: "repo", URL: "...", KeyURL: "...", KeySHA256: "..."}},
//	}
//	spec, err := DeriveContainerSpec(reqs)
//	// spec.BaseImage = "debian:bookworm-slim"
//	// spec.LinuxFamily = "debian"
//	// spec.BuildCommands = [GPG key download, verification, repo setup, apt-get update, apt-get install]
func DeriveContainerSpec(reqs *SystemRequirements) (*ContainerSpec, error) {
	if reqs == nil || (len(reqs.Packages) == 0 && len(reqs.Repositories) == 0) {
		return nil, nil
	}

	packages := reqs.Packages
	if packages == nil {
		packages = make(map[string][]string)
	}

	// Extract package managers from packages and repositories
	pmSet := make(map[string]bool)
	for pm := range packages {
		pmSet[pm] = true
	}
	for _, repo := range reqs.Repositories {
		pmSet[repo.Manager] = true
	}

	// Convert set to sorted slice for deterministic ordering
	var pms []string
	for pm := range pmSet {
		pms = append(pms, pm)
	}
	sort.Strings(pms)

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

	// Generate build commands (repositories first, then packages)
	buildCommands, err := generateBuildCommands(family, packages, reqs.Repositories)
	if err != nil {
		return nil, err
	}

	return &ContainerSpec{
		BaseImage:     baseImage,
		LinuxFamily:   family,
		Packages:      packages,
		Repositories:  reqs.Repositories,
		BuildCommands: buildCommands,
	}, nil
}

// generateBuildCommands creates Docker RUN commands for repository setup and package installation.
// Command ordering: prerequisites → repositories (with GPG) → update → packages
func generateBuildCommands(family string, packages map[string][]string, repositories []RepositoryConfig) ([]string, error) {
	var commands []string

	switch family {
	case "debian":
		commands = generateDebianCommands(packages["apt"], repositories)
	case "rhel":
		commands = generateRhelCommands(packages["dnf"], repositories)
	case "arch":
		commands = generateArchCommands(packages["pacman"], repositories)
	case "alpine":
		commands = generateAlpineCommands(packages["apk"], repositories)
	case "suse":
		commands = generateSuseCommands(packages["zypper"], repositories)
	default:
		return nil, fmt.Errorf("unknown linux_family %q", family)
	}

	if len(commands) == 0 {
		return nil, errors.New("no packages or repositories found")
	}

	return commands, nil
}

// generateDebianCommands creates Docker RUN commands for Debian/Ubuntu.
func generateDebianCommands(packages []string, repositories []RepositoryConfig) []string {
	var commands []string
	hasRepos := false

	// Filter repositories for apt
	var aptRepos []RepositoryConfig
	for _, repo := range repositories {
		if repo.Manager == "apt" {
			aptRepos = append(aptRepos, repo)
			hasRepos = true
		}
	}

	// Install prerequisites only if we have repositories (need wget for GPG keys)
	if hasRepos {
		prereqs := []string{"wget", "ca-certificates", "software-properties-common", "gpg"}
		commands = append(commands, "RUN apt-get update && apt-get install -y "+strings.Join(prereqs, " "))
	}

	// Setup repositories
	for i, repo := range aptRepos {
		if repo.Type == "repo" {
			// Repository with GPG key verification
			keyFile := fmt.Sprintf("/tmp/repo-key-%d.gpg", i)

			// Download GPG key
			commands = append(commands, fmt.Sprintf("RUN wget -O %s %s", keyFile, repo.KeyURL))

			// Verify GPG key hash (fail build if mismatch)
			commands = append(commands, fmt.Sprintf("RUN echo \"%s  %s\" | sha256sum -c || (echo \"GPG key hash mismatch for %s\" && exit 1)", repo.KeySHA256, keyFile, repo.URL))

			// Import GPG key
			commands = append(commands, fmt.Sprintf("RUN apt-key add %s", keyFile))

			// Add repository
			repoName := fmt.Sprintf("custom-repo-%d", i)
			commands = append(commands, fmt.Sprintf("RUN echo \"deb %s\" > /etc/apt/sources.list.d/%s.list", repo.URL, repoName))

		} else if repo.Type == "ppa" {
			// PPA addition (uses add-apt-repository)
			commands = append(commands, fmt.Sprintf("RUN add-apt-repository -y ppa:%s", repo.PPA))
		}
	}

	// Update package cache if we added repositories
	if hasRepos {
		commands = append(commands, "RUN apt-get update")
	}

	// Install packages
	if len(packages) > 0 {
		sorted := make([]string, len(packages))
		copy(sorted, packages)
		sort.Strings(sorted)
		pkgList := strings.Join(sorted, " ")

		if hasRepos {
			// Already updated, just install
			commands = append(commands, fmt.Sprintf("RUN apt-get install -y %s", pkgList))
		} else {
			// Update and install in one command
			commands = append(commands, fmt.Sprintf("RUN apt-get update && apt-get install -y %s", pkgList))
		}
	}

	return commands
}

// generateRhelCommands creates Docker RUN commands for Fedora/RHEL.
func generateRhelCommands(packages []string, repositories []RepositoryConfig) []string {
	var commands []string
	hasRepos := false

	// Filter repositories for dnf
	var dnfRepos []RepositoryConfig
	for _, repo := range repositories {
		if repo.Manager == "dnf" {
			dnfRepos = append(dnfRepos, repo)
			hasRepos = true
		}
	}

	// Install prerequisites if we have repositories with GPG keys
	if hasRepos {
		commands = append(commands, "RUN dnf install -y wget")
	}

	// Setup repositories
	for i, repo := range dnfRepos {
		if repo.KeyURL != "" {
			// Repository with GPG key
			keyFile := fmt.Sprintf("/tmp/repo-key-%d.gpg", i)

			// Download GPG key
			commands = append(commands, fmt.Sprintf("RUN wget -O %s %s", keyFile, repo.KeyURL))

			// Import GPG key
			commands = append(commands, fmt.Sprintf("RUN rpm --import %s", keyFile))
		}

		// Add repository configuration
		repoName := fmt.Sprintf("custom-repo-%d", i)
		repoConfig := fmt.Sprintf("[%s]\\nname=Custom Repository %d\\nbaseurl=%s\\nenabled=1\\ngpgcheck=1", repoName, i, repo.URL)
		if repo.KeyURL != "" {
			repoConfig += fmt.Sprintf("\\ngpgkey=file:///tmp/repo-key-%d.gpg", i)
		}
		commands = append(commands, fmt.Sprintf("RUN echo -e \"%s\" > /etc/yum.repos.d/%s.repo", repoConfig, repoName))
	}

	// Install packages
	if len(packages) > 0 {
		sorted := make([]string, len(packages))
		copy(sorted, packages)
		sort.Strings(sorted)
		pkgList := strings.Join(sorted, " ")
		commands = append(commands, fmt.Sprintf("RUN dnf install -y %s", pkgList))
	}

	return commands
}

// generateArchCommands creates Docker RUN commands for Arch Linux.
func generateArchCommands(packages []string, repositories []RepositoryConfig) []string {
	var commands []string

	// Arch doesn't commonly use third-party repositories in containers
	// If needed, repository support can be added later

	// Install packages
	if len(packages) > 0 {
		sorted := make([]string, len(packages))
		copy(sorted, packages)
		sort.Strings(sorted)
		pkgList := strings.Join(sorted, " ")
		commands = append(commands, fmt.Sprintf("RUN pacman -Sy --noconfirm %s", pkgList))
	}

	return commands
}

// generateAlpineCommands creates Docker RUN commands for Alpine Linux.
func generateAlpineCommands(packages []string, repositories []RepositoryConfig) []string {
	var commands []string

	// Alpine doesn't commonly use third-party repositories in containers
	// If needed, repository support can be added later

	// Install packages
	if len(packages) > 0 {
		sorted := make([]string, len(packages))
		copy(sorted, packages)
		sort.Strings(sorted)
		pkgList := strings.Join(sorted, " ")
		commands = append(commands, fmt.Sprintf("RUN apk add --no-cache %s", pkgList))
	}

	return commands
}

// generateSuseCommands creates Docker RUN commands for SUSE Linux.
func generateSuseCommands(packages []string, repositories []RepositoryConfig) []string {
	var commands []string

	// SUSE doesn't commonly use third-party repositories in containers
	// If needed, repository support can be added later

	// Install packages
	if len(packages) > 0 {
		sorted := make([]string, len(packages))
		copy(sorted, packages)
		sort.Strings(sorted)
		pkgList := strings.Join(sorted, " ")
		commands = append(commands, fmt.Sprintf("RUN zypper install -y %s", pkgList))
	}

	return commands
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
	// Include base image in hash to distinguish different base versions
	// (e.g., debian:bookworm vs debian:bullseye)
	// Note: This uses the image tag, not digest, so it doesn't catch
	// time-based staleness when the tag is updated. See issue #799 for
	// proper version pinning solution.
	var parts []string
	parts = append(parts, fmt.Sprintf("base:%s", spec.BaseImage))

	// Extract and sort all pm:package pairs for deterministic hashing
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

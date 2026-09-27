// Package bottletag chooses Homebrew bottle platform tags.
//
// Homebrew names macOS bottles after the release they were built on
// ("arm64_sequoia" on Apple Silicon, "sequoia" on Intel). A bottle built
// for one release runs on that release and every newer one, never on an
// older one. Linux bottles are "x86_64_linux" and "arm64_linux".
package bottletag

import (
	"fmt"
	"strconv"
	"strings"
)

// CrossMachineMacOSVersion is the newest macOS release whose bottles a
// plan may use when it isn't generated for the machine that will run it:
// `tsuku eval` output and golden files. macOS 15 (Sequoia) is the oldest
// release tsuku verifies, so a plan made this way runs on any verified
// Mac, and golden files don't depend on the runner that generated them.
const CrossMachineMacOSVersion = 15

// preferredMacOSVersion is where selection starts: Sonoma, the tag tsuku
// requested for every Mac before selection became version-aware. Starting
// here keeps every formula that still ships a Sonoma bottle on the bottle
// it had. Homebrew itself prefers the host's own release, but Homebrew
// stopped building Sonoma bottles in September 2026, so each rebottled
// formula loses the tag and selection moves on to the same newer bottles
// Homebrew would pick. Once Sonoma bottles are gone from nearly every
// formula, starting at the ceiling instead costs nothing.
const preferredMacOSVersion = 14

// macOSReleases maps macOS major versions to Homebrew's tag names, oldest
// first.
var macOSReleases = []struct {
	version int
	name    string
}{
	{11, "big_sur"},
	{12, "monterey"},
	{13, "ventura"},
	{14, "sonoma"},
	{15, "sequoia"},
	{26, "tahoe"},
	{27, "golden_gate"},
}

// darwinTag returns the bottle tag for a macOS release on an architecture
// ("arm64" or "amd64").
func darwinTag(arch string, version int) (string, bool) {
	for _, r := range macOSReleases {
		if r.version != version {
			continue
		}
		switch arch {
		case "arm64":
			return "arm64_" + r.name, true
		case "amd64":
			return r.name, true
		}
	}
	return "", false
}

// MacOSVersion returns the macOS major version and architecture a darwin
// bottle tag was built for. ok is false for Linux tags and for names this
// version of tsuku doesn't know.
func MacOSVersion(tag string) (version int, arch string, ok bool) {
	name, arch := tag, "amd64"
	if rest, found := strings.CutPrefix(tag, "arm64_"); found {
		name, arch = rest, "arm64"
	}
	for _, r := range macOSReleases {
		if r.name == name {
			return r.version, arch, true
		}
	}
	return 0, "", false
}

// Candidates returns the bottle tags to try for a platform, in order of
// preference. Linux has a single tag. On darwin, the candidates are every
// known release at or below macOSCeiling: Sonoma first, then newer
// releases in ascending order, then older ones in descending order. A
// macOSCeiling of 0 means the plan isn't for this machine and uses
// CrossMachineMacOSVersion.
func Candidates(os, arch string, macOSCeiling int) ([]string, error) {
	switch {
	case os == "linux" && arch == "arm64":
		return []string{"arm64_linux"}, nil
	case os == "linux" && arch == "amd64":
		return []string{"x86_64_linux"}, nil
	case os == "darwin" && (arch == "arm64" || arch == "amd64"):
	default:
		return nil, fmt.Errorf("unsupported platform: %s/%s", os, arch)
	}

	if macOSCeiling <= 0 {
		macOSCeiling = CrossMachineMacOSVersion
	}
	var newer, older []string
	for _, r := range macOSReleases {
		if r.version > macOSCeiling {
			break
		}
		tag, _ := darwinTag(arch, r.version)
		if r.version >= preferredMacOSVersion {
			newer = append(newer, tag)
		} else {
			older = append([]string{tag}, older...)
		}
	}
	candidates := append(newer, older...)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no Homebrew bottles exist for macOS %d", macOSCeiling)
	}
	return candidates, nil
}

// parseMacOSVersion returns the major version from a macOS product
// version string ("15.6.1" -> 15). Compatibility values are mapped to the
// real release, as Homebrew does: "10.16" is macOS 11, and "16.x" (what
// some binaries were shown on macOS 26 betas) is macOS 26. Returns 0 when
// the string can't be parsed.
func parseMacOSVersion(s string) int {
	parts := strings.Split(strings.TrimSpace(s), ".")
	major, err := strconv.Atoi(parts[0])
	if err != nil || major <= 0 {
		return 0
	}
	if major == 10 && len(parts) > 1 && parts[1] == "16" {
		return 11
	}
	if major == 16 {
		return 26
	}
	return major
}

// HostMacOSVersion returns the running system's macOS major version, or 0
// when it isn't macOS or the version can't be read.
func HostMacOSVersion() int {
	return hostMacOSVersion()
}

// CheckForHost returns an error when a darwin bottle tag was built for a
// newer macOS than hostVersion. Linux tags always pass. A hostVersion of 0
// (unknown) passes. A darwin tag this version of tsuku doesn't know fails,
// since it most likely names a newer release.
func CheckForHost(tag string, hostVersion int) error {
	if strings.HasSuffix(tag, "_linux") || hostVersion <= 0 {
		return nil
	}
	floor, _, ok := MacOSVersion(tag)
	if !ok {
		return fmt.Errorf("bottle tag %q names a macOS release this version of tsuku doesn't know; upgrade tsuku or regenerate the plan", tag)
	}
	if floor > hostVersion {
		return fmt.Errorf("bottle %s is built for macOS %d, but this system runs macOS %d; regenerate the plan on this machine", tag, floor, hostVersion)
	}
	return nil
}

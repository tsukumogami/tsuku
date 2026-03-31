package install

import (
	"fmt"
	"strings"
	"unicode"
)

// PinLevel represents how tightly a tool is pinned to a version range.
type PinLevel int

const (
	// PinLatest tracks the latest stable version (Requested is empty or "latest").
	PinLatest PinLevel = iota
	// PinMajor pins to a major version (e.g., Requested "20" allows 20.x.y).
	PinMajor
	// PinMinor pins to a minor version (e.g., Requested "1.29" allows 1.29.z).
	PinMinor
	// PinExact pins to an exact version (e.g., Requested "1.29.3" never auto-updates).
	PinExact
	// PinChannel tracks a named channel (e.g., Requested "@lts").
	PinChannel
)

// String returns a human-readable representation of the pin level.
func (p PinLevel) String() string {
	switch p {
	case PinLatest:
		return "latest"
	case PinMajor:
		return "major"
	case PinMinor:
		return "minor"
	case PinExact:
		return "exact"
	case PinChannel:
		return "channel"
	default:
		return "unknown"
	}
}

// PinLevelFromRequested derives the pin level from the Requested field stored
// in VersionState. The number of dot-separated components determines the level:
//
//	""        -> PinLatest
//	"latest"  -> PinLatest
//	"@lts"    -> PinChannel
//	"20"      -> PinMajor
//	"1.29"    -> PinMinor
//	"1.29.3"  -> PinExact
func PinLevelFromRequested(requested string) PinLevel {
	if requested == "" || requested == "latest" {
		return PinLatest
	}
	if strings.HasPrefix(requested, "@") {
		return PinChannel
	}

	parts := strings.Split(requested, ".")
	switch len(parts) {
	case 1:
		return PinMajor
	case 2:
		return PinMinor
	default:
		return PinExact
	}
}

// VersionMatchesPin reports whether version falls within the pin boundary
// defined by requested. Uses dot-boundary matching to prevent "1" from
// matching "10.0.0".
func VersionMatchesPin(version, requested string) bool {
	if requested == "" || requested == "latest" {
		return true
	}
	if strings.HasPrefix(requested, "@") {
		// Channel pins can't be evaluated by string matching.
		// The caller must use provider-specific channel resolution.
		return false
	}
	return version == requested || strings.HasPrefix(version, requested+".")
}

// ValidateRequested checks that a Requested string contains only expected
// characters. This is defense-in-depth against malformed state data reaching
// the version resolution path.
func ValidateRequested(requested string) error {
	if requested == "" {
		return nil
	}
	for _, r := range requested {
		if unicode.IsDigit(r) || r == '.' || r == '@' || unicode.IsLetter(r) || r == '-' {
			continue
		}
		return fmt.Errorf("invalid character %q in requested version %q", string(r), requested)
	}
	if strings.Contains(requested, "..") {
		return fmt.Errorf("path traversal pattern in requested version %q", requested)
	}
	if strings.ContainsAny(requested, "/\\") {
		return fmt.Errorf("path separator in requested version %q", requested)
	}
	return nil
}

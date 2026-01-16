package version

import (
	"fmt"
	"regexp"
	"strings"
)

// tapFormulaInfo contains parsed metadata from a Homebrew formula file
type tapFormulaInfo struct {
	Version   string            // Extracted version (e.g., "1.7.0")
	RootURL   string            // Bottle root URL from bottle block
	Checksums map[string]string // Platform-specific checksums (platform -> sha256)
}

// Regex patterns for parsing Homebrew formula files
var (
	// versionRegex matches: version "1.7.0" (explicit version declaration)
	versionRegex = regexp.MustCompile(`(?m)^\s*version\s+"([^"]+)"`)

	// bottleBlockRegex matches: bottle do ... end (multiline block)
	bottleBlockRegex = regexp.MustCompile(`(?s)bottle\s+do\s+(.*?)\s+end`)

	// rootURLRegex matches: root_url "https://..." inside bottle block
	rootURLRegex = regexp.MustCompile(`root_url\s+"([^"]+)"`)

	// sha256Regex matches two patterns:
	// Pattern 1: sha256 "abc123" => :arm64_sonoma
	// Pattern 2: sha256 arm64_sonoma: "abc123"
	sha256Regex = regexp.MustCompile(`sha256\s+(?:"([a-f0-9]{64})"\s*=>\s*:(\w+)|(\w+):\s*"([a-f0-9]{64})")`)
)

// parseFormulaFile extracts version and bottle metadata from a Homebrew formula file.
// Version is required; bottle metadata is optional (many third-party taps don't use bottles).
func parseFormulaFile(content string) (*tapFormulaInfo, error) {
	info := &tapFormulaInfo{
		Checksums: make(map[string]string),
	}

	// 1. Extract version (required)
	if match := versionRegex.FindStringSubmatch(content); len(match) > 1 {
		info.Version = match[1]
	} else {
		return nil, fmt.Errorf("no version found in formula")
	}

	// 2. Extract bottle block (optional - many third-party taps don't use bottles)
	bottleMatch := bottleBlockRegex.FindStringSubmatch(content)
	if len(bottleMatch) >= 2 {
		bottleBlock := bottleMatch[1]

		// 3. Extract root_url from bottle block
		if rootMatch := rootURLRegex.FindStringSubmatch(bottleBlock); len(rootMatch) > 1 {
			info.RootURL = rootMatch[1]
		}

		// 4. Extract checksums per platform
		for _, match := range sha256Regex.FindAllStringSubmatch(bottleBlock, -1) {
			// Handle both patterns:
			// match[1], match[2] for: sha256 "hash" => :platform
			// match[3], match[4] for: sha256 platform: "hash"
			if match[1] != "" && match[2] != "" {
				info.Checksums[match[2]] = match[1]
			} else if match[3] != "" && match[4] != "" {
				info.Checksums[match[3]] = match[4]
			}
		}
	}
	// Note: If no bottle block, info.RootURL and info.Checksums remain empty.
	// This is valid - the recipe can construct download URLs using the version.

	return info, nil
}

// macOSCodenames maps macOS major versions to Homebrew codenames
var macOSCodenames = map[int]string{
	15: "sequoia",
	14: "sonoma",
	13: "ventura",
	12: "monterey",
	11: "big_sur",
}

// getPlatformTags returns a list of Homebrew platform tags for the given OS and architecture.
// Returns multiple tags in order of preference for fallback.
//
// Examples:
//   - darwin/arm64 -> ["arm64_sonoma", "arm64_ventura", "arm64_monterey"]
//   - darwin/amd64 -> ["sonoma", "ventura", "monterey"]
//   - linux/amd64 -> ["x86_64_linux"]
//   - linux/arm64 -> ["arm64_linux"]
func getPlatformTags(goos, goarch string, macOSVersion int) []string {
	if goos == "linux" {
		if goarch == "arm64" {
			return []string{"arm64_linux"}
		}
		return []string{"x86_64_linux"}
	}

	if goos == "darwin" {
		// Build fallback chain starting from current version going backwards
		var tags []string

		// Determine starting version
		startVersion := macOSVersion
		if startVersion == 0 {
			startVersion = 14 // Default to Sonoma if unknown
		}

		// Add tags from current version backwards
		for v := startVersion; v >= 11; v-- {
			codename, ok := macOSCodenames[v]
			if !ok {
				continue
			}
			if goarch == "arm64" {
				tags = append(tags, "arm64_"+codename)
			} else {
				tags = append(tags, codename)
			}
		}

		return tags
	}

	// Unknown platform - return empty list
	return nil
}

// buildBottleURL constructs the bottle download URL from formula metadata.
// URL format: {root_url}/{formula}--{version}.{platform}.bottle.tar.gz
func buildBottleURL(rootURL, formula, version, platform string) string {
	// Handle version with underscores (some versions have them)
	// Homebrew uses double-dash between formula and version
	filename := fmt.Sprintf("%s--%s.%s.bottle.tar.gz", formula, version, platform)
	return strings.TrimSuffix(rootURL, "/") + "/" + filename
}

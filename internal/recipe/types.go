package recipe

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Recipe represents an action-based recipe
type Recipe struct {
	Metadata  MetadataSection `toml:"metadata"`
	Version   VersionSection  `toml:"version"`
	Resources []Resource      `toml:"resources,omitempty"`
	Patches   []Patch         `toml:"patches,omitempty"`
	Steps     []Step          `toml:"steps"`
	Verify    VerifySection   `toml:"verify"`
}

// Resource represents an additional download required for source builds.
// Resources are staged to specified directories before the build starts.
type Resource struct {
	Name     string `toml:"name"`     // Unique identifier for the resource
	URL      string `toml:"url"`      // Download URL for the resource archive
	Checksum string `toml:"checksum"` // SHA256 checksum for verification
	Dest     string `toml:"dest"`     // Destination directory relative to source (e.g., "deps/tree-sitter-c")
}

// Patch represents a source modification to apply before building.
// Patches can be URL-based or inline (embedded in the recipe).
type Patch struct {
	URL      string `toml:"url,omitempty"`      // URL to download patch file (mutually exclusive with Data)
	Data     string `toml:"data,omitempty"`     // Inline patch content (mutually exclusive with URL)
	Checksum string `toml:"checksum,omitempty"` // SHA256 checksum for URL-based patches (required for url, optional for data)
	Strip    int    `toml:"strip,omitempty"`    // Strip level for patch command (-p flag), default 1
	Subdir   string `toml:"subdir,omitempty"`   // Subdirectory to apply patch in (relative to source root)
}

// TextReplace represents a text substitution (maps to Homebrew's inreplace).
type TextReplace struct {
	File        string `toml:"file"`            // File path relative to source root
	Pattern     string `toml:"pattern"`         // Pattern to find (literal string or regex)
	Replacement string `toml:"replacement"`     // Replacement text
	IsRegex     bool   `toml:"regex,omitempty"` // If true, treat pattern as regex
}

// ToTOML serializes the recipe to TOML format.
// This handles the special step encoding where action params are flattened.
func (r *Recipe) ToTOML() ([]byte, error) {
	var buf strings.Builder

	// Encode metadata section
	buf.WriteString("[metadata]\n")
	if r.Metadata.Name != "" {
		buf.WriteString(fmt.Sprintf("name = %q\n", r.Metadata.Name))
	}
	if r.Metadata.Description != "" {
		buf.WriteString(fmt.Sprintf("description = %q\n", r.Metadata.Description))
	}
	if r.Metadata.Homepage != "" {
		buf.WriteString(fmt.Sprintf("homepage = %q\n", r.Metadata.Homepage))
	}
	if r.Metadata.VersionFormat != "" {
		buf.WriteString(fmt.Sprintf("version_format = %q\n", r.Metadata.VersionFormat))
	}
	buf.WriteString("\n")

	// Encode version section
	buf.WriteString("[version]\n")
	if r.Version.Source != "" {
		buf.WriteString(fmt.Sprintf("source = %q\n", r.Version.Source))
	}
	if r.Version.GitHubRepo != "" {
		buf.WriteString(fmt.Sprintf("github_repo = %q\n", r.Version.GitHubRepo))
	}
	if r.Version.TagPrefix != "" {
		buf.WriteString(fmt.Sprintf("tag_prefix = %q\n", r.Version.TagPrefix))
	}
	if r.Version.Module != "" {
		buf.WriteString(fmt.Sprintf("module = %q\n", r.Version.Module))
	}
	if r.Version.Formula != "" {
		buf.WriteString(fmt.Sprintf("formula = %q\n", r.Version.Formula))
	}
	buf.WriteString("\n")

	// Encode resources - each resource as [[resources]]
	for _, res := range r.Resources {
		buf.WriteString("[[resources]]\n")
		buf.WriteString(fmt.Sprintf("name = %q\n", res.Name))
		buf.WriteString(fmt.Sprintf("url = %q\n", res.URL))
		if res.Checksum != "" {
			buf.WriteString(fmt.Sprintf("checksum = %q\n", res.Checksum))
		}
		buf.WriteString(fmt.Sprintf("dest = %q\n", res.Dest))
		buf.WriteString("\n")
	}

	// Encode patches - each patch as [[patches]]
	for _, patch := range r.Patches {
		buf.WriteString("[[patches]]\n")
		if patch.URL != "" {
			buf.WriteString(fmt.Sprintf("url = %q\n", patch.URL))
		}
		if patch.Data != "" {
			// Use triple-quoted string for multiline patch data
			buf.WriteString(fmt.Sprintf("data = %q\n", patch.Data))
		}
		if patch.Checksum != "" {
			buf.WriteString(fmt.Sprintf("checksum = %q\n", patch.Checksum))
		}
		if patch.Strip != 0 {
			buf.WriteString(fmt.Sprintf("strip = %d\n", patch.Strip))
		}
		if patch.Subdir != "" {
			buf.WriteString(fmt.Sprintf("subdir = %q\n", patch.Subdir))
		}
		buf.WriteString("\n")
	}

	// Encode steps - each step as [[steps]] with flattened params
	for _, step := range r.Steps {
		buf.WriteString("[[steps]]\n")
		stepMap := step.ToMap()
		enc := toml.NewEncoder(&buf)
		if err := enc.Encode(stepMap); err != nil {
			return nil, fmt.Errorf("failed to encode step: %w", err)
		}
		buf.WriteString("\n")
	}

	// Encode verify section
	buf.WriteString("[verify]\n")
	if r.Verify.Command != "" {
		buf.WriteString(fmt.Sprintf("command = %q\n", r.Verify.Command))
	}
	if r.Verify.Pattern != "" {
		buf.WriteString(fmt.Sprintf("pattern = %q\n", r.Verify.Pattern))
	}

	return []byte(buf.String()), nil
}

// MetadataSection contains recipe metadata
type MetadataSection struct {
	Name                     string   `toml:"name"`
	Description              string   `toml:"description"`
	Homepage                 string   `toml:"homepage"`
	VersionFormat            string   `toml:"version_format"`
	RequiresSudo             bool     `toml:"requires_sudo"`
	Dependencies             []string `toml:"dependencies"`               // Install-time dependencies (replaces implicit)
	RuntimeDependencies      []string `toml:"runtime_dependencies"`       // Runtime dependencies (replaces implicit)
	ExtraDependencies        []string `toml:"extra_dependencies"`         // Additional install-time dependencies (extends implicit)
	ExtraRuntimeDependencies []string `toml:"extra_runtime_dependencies"` // Additional runtime dependencies (extends implicit)
	Tier                     int      `toml:"tier"`                       // Installation tier: 1=binary, 2=package manager, 3=nix
	Type                     string   `toml:"type"`                       // Recipe type: "tool" (default) or "library"
	LLMValidation            string   `toml:"llm_validation,omitempty"`   // LLM validation status: "skipped" or empty
	Binaries                 []string `toml:"binaries,omitempty"`         // Explicit binary paths for homebrew recipes

	// Platform constraints (optional, defaults provide universal support)
	SupportedOS          []string `toml:"supported_os,omitempty"`          // Allowed OS values (default: all OS)
	SupportedArch        []string `toml:"supported_arch,omitempty"`        // Allowed architecture values (default: all arch)
	UnsupportedPlatforms []string `toml:"unsupported_platforms,omitempty"` // Platform exceptions in "os/arch" format (default: empty)
}

// VersionSection specifies how to resolve versions
type VersionSection struct {
	Source     string `toml:"source"`      // e.g., "nodejs_dist", "github_releases", "npm_registry", "homebrew"
	GitHubRepo string `toml:"github_repo"` // e.g., "rust-lang/rust" - use GitHub for version detection only
	TagPrefix  string `toml:"tag_prefix"`  // e.g., "ruby-" - filter tags by prefix and strip it from version
	Module     string `toml:"module"`      // Go module path for goproxy version resolution (when different from install path)
	Formula    string `toml:"formula"`     // Homebrew formula name for version resolution (e.g., "libyaml")
}

// Step represents a single action step
// We use interface{} for the full step data and extract fields manually
type Step struct {
	Action      string
	When        map[string]string
	Note        string
	Description string
	Params      map[string]interface{}
}

// UnmarshalTOML implements custom TOML unmarshaling for Step
func (s *Step) UnmarshalTOML(data interface{}) error {
	// data is a map[string]interface{} containing all the step fields
	stepMap, ok := data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("step must be a map")
	}

	// Extract known fields
	if action, ok := stepMap["action"].(string); ok {
		s.Action = action
	}

	if when, ok := stepMap["when"].(map[string]interface{}); ok {
		s.When = make(map[string]string)
		for k, v := range when {
			if strVal, ok := v.(string); ok {
				s.When[k] = strVal
			}
		}
	}

	if note, ok := stepMap["note"].(string); ok {
		s.Note = note
	}

	if desc, ok := stepMap["description"].(string); ok {
		s.Description = desc
	}

	// All other fields go into Params
	s.Params = make(map[string]interface{})
	for k, v := range stepMap {
		if k != "action" && k != "when" && k != "note" && k != "description" {
			s.Params[k] = v
		}
	}

	return nil
}

// ToMap converts a Step to a flat map representation for TOML encoding.
// This reconstructs the flat map structure that the TOML encoder expects.
func (s Step) ToMap() map[string]interface{} {
	result := make(map[string]interface{})

	// Add action (required)
	result["action"] = s.Action

	// Add optional fields if set
	if s.Note != "" {
		result["note"] = s.Note
	}
	if s.Description != "" {
		result["description"] = s.Description
	}
	if len(s.When) > 0 {
		result["when"] = s.When
	}

	// Add all params
	for k, v := range s.Params {
		result[k] = v
	}

	return result
}

// Recipe types
const (
	// RecipeTypeTool is the default type for recipes that install tools
	RecipeTypeTool = "tool"
	// RecipeTypeLibrary is for recipes that install shared libraries
	RecipeTypeLibrary = "library"
)

// Verification modes
const (
	// VerifyModeVersion verifies the exact version is installed (default)
	VerifyModeVersion = "version"
	// VerifyModeOutput matches a pattern in command output without version check
	VerifyModeOutput = "output"
)

// Version format transforms
const (
	// VersionFormatRaw leaves the version string unchanged (default)
	VersionFormatRaw = "raw"
	// VersionFormatSemver extracts X.Y.Z from any format
	VersionFormatSemver = "semver"
	// VersionFormatSemverFull extracts X.Y.Z[-pre][+build]
	VersionFormatSemverFull = "semver_full"
	// VersionFormatStripV removes leading "v" from the version
	VersionFormatStripV = "strip_v"
)

// VerifySection defines how to verify the installation
type VerifySection struct {
	Command       string             `toml:"command"`
	Pattern       string             `toml:"pattern"`
	Mode          string             `toml:"mode,omitempty"`
	VersionFormat string             `toml:"version_format,omitempty"`
	Reason        string             `toml:"reason,omitempty"`
	ExitCode      *int               `toml:"exit_code,omitempty"` // Expected exit code (default: 0)
	Additional    []AdditionalVerify `toml:"additional,omitempty"`
}

// AdditionalVerify represents additional verification commands
type AdditionalVerify struct {
	Command string `toml:"command"`
	Pattern string `toml:"pattern"`
}

// Common action parameter structures
// These are used by multiple actions

// BinaryMapping maps source binary to destination
type BinaryMapping struct {
	Src  string `toml:"src"`
	Dest string `toml:"dest"`
}

// EnvVar represents an environment variable
type EnvVar struct {
	Name  string `toml:"name"`
	Value string `toml:"value"`
}

// Replacement represents a text replacement pattern
type Replacement struct {
	Pattern     string `toml:"pattern"`
	Replacement string `toml:"replacement"`
}

// ExtractBinaries extracts all binary names from a recipe
// by looking at metadata.binaries first, then action parameters (binaries, executables, etc.)
func (r *Recipe) ExtractBinaries() []string {
	// Check metadata.binaries first (explicit declaration for homebrew recipes)
	if len(r.Metadata.Binaries) > 0 {
		return r.Metadata.Binaries
	}

	var binaries []string
	seen := make(map[string]bool)

	for _, step := range r.Steps {
		// Check for 'binary' parameter (singular, used by github_file)
		if binaryRaw, ok := step.Params["binary"]; ok {
			if binaryStr, ok := binaryRaw.(string); ok {
				// Apply same logic as parseBinaries: simple strings go to bin/<basename>
				binaryName := filepath.Base(binaryStr)
				destPath := filepath.Join("bin", binaryName)
				if !seen[binaryName] {
					binaries = append(binaries, destPath)
					seen[binaryName] = true
				}
			}
		}

		// Check for 'binaries' parameter (plural, used by download_archive, github_archive)
		if binariesRaw, ok := step.Params["binaries"]; ok {
			// Check install_mode to determine if we should add bin/ prefix
			installMode, _ := step.Params["install_mode"].(string)
			isDirectoryMode := (installMode == "directory" || installMode == "directory_wrapped")

			if binariesList, ok := binariesRaw.([]interface{}); ok {
				for _, b := range binariesList {
					var destPath string

					// Handle two formats:
					// 1. Simple string: behavior depends on install_mode
					// 2. Object: {src = "...", dest = "..."}
					if binStr, ok := b.(string); ok {
						if isDirectoryMode {
							// Directory mode: keep paths as-is (e.g., "zig", "cargo/bin/cargo")
							destPath = binStr
						} else {
							// Binaries mode: add bin/ prefix (e.g., "age" → "bin/age")
							basename := filepath.Base(binStr)
							destPath = filepath.Join("bin", basename)
						}
					} else if binMap, ok := b.(map[string]interface{}); ok {
						if dest, ok := binMap["dest"].(string); ok {
							destPath = dest
						}
					}

					if destPath != "" {
						// For deduplication, check using basename but store full path
						binaryName := filepath.Base(destPath)
						if !seen[binaryName] {
							binaries = append(binaries, destPath)
							seen[binaryName] = true
						}
					}
				}
			}
		}

		// Check for 'executables' parameter (used by npm_install)
		if executablesRaw, ok := step.Params["executables"]; ok {
			if executablesList, ok := executablesRaw.([]interface{}); ok {
				for _, e := range executablesList {
					if exeStr, ok := e.(string); ok {
						// npm_install installs to bin/ directory
						binaryName := filepath.Base(exeStr)
						destPath := filepath.Join("bin", binaryName)
						if !seen[binaryName] {
							binaries = append(binaries, destPath)
							seen[binaryName] = true
						}
					}
				}
			}
		}
	}

	return binaries
}

// IsLibrary returns true if this recipe is a library (type = "library")
func (r *Recipe) IsLibrary() bool {
	return r.Metadata.Type == RecipeTypeLibrary
}

// HasChecksumVerification returns true if any download step includes checksum verification.
// This checks for the presence of "checksum" or "checksum_url" parameters in download-related actions.
func (r *Recipe) HasChecksumVerification() bool {
	// Actions that download external files and can verify checksums
	downloadActions := map[string]bool{
		"download":         true,
		"download_archive": true,
		"github_archive":   true,
		"github_file":      true,
	}

	hasDownloadStep := false
	for _, step := range r.Steps {
		if !downloadActions[step.Action] {
			continue
		}
		hasDownloadStep = true

		// Check for checksum parameters
		if _, hasChecksum := step.Params["checksum"]; hasChecksum {
			return true
		}
		if _, hasChecksumURL := step.Params["checksum_url"]; hasChecksumURL {
			return true
		}
	}

	// If there are no download steps, consider it "verified" (nothing to verify)
	return !hasDownloadStep
}

package recipe

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
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

	// Fossil-specific fields (used by fossil_archive action)
	FossilRepo       string `toml:"fossil_repo"`       // Full URL to Fossil repository (e.g., "https://sqlite.org/src")
	ProjectName      string `toml:"project_name"`      // Project name for tarball filename (e.g., "sqlite")
	VersionSeparator string `toml:"version_separator"` // Separator in version numbers for tag conversion (e.g., "-" converts 9.0.0 to 9-0-0)
	TimelineTag      string `toml:"timeline_tag"`      // Tag filter for timeline URL (default: "release")
}

// Matchable provides platform dimension accessors for matching.
// Both the lightweight MatchTarget (for recipe evaluation) and
// the runtime platform.Target implement this interface.
type Matchable interface {
	OS() string
	Arch() string
	LinuxFamily() string
}

// MatchTarget is a lightweight struct for platform matching in recipe evaluation.
// Use NewMatchTarget() to construct instances.
type MatchTarget struct {
	os          string
	arch        string
	linuxFamily string
}

// NewMatchTarget creates a MatchTarget for platform matching.
func NewMatchTarget(os, arch, linuxFamily string) MatchTarget {
	return MatchTarget{
		os:          os,
		arch:        arch,
		linuxFamily: linuxFamily,
	}
}

// OS returns the operating system.
func (m MatchTarget) OS() string { return m.os }

// Arch returns the architecture.
func (m MatchTarget) Arch() string { return m.arch }

// LinuxFamily returns the Linux distribution family.
func (m MatchTarget) LinuxFamily() string { return m.linuxFamily }

// WhenClause represents platform and runtime conditions for conditional step execution.
// Supports platform tuples ("os/arch"), OS arrays, and package manager filtering.
//
// Matching semantics:
//   - Empty clause (all fields zero) matches all platforms
//   - Platform array: exact platform tuple match ("darwin/arm64")
//   - OS array: matches any architecture on the specified OS
//   - Platform and OS are mutually exclusive (validation enforces this)
//   - PackageManager is evaluated at runtime (if present, others must pass AND pm must match)
type WhenClause struct {
	Platform       []string `toml:"platform,omitempty"`        // Platform tuples: ["darwin/arm64", "linux/amd64"]
	OS             []string `toml:"os,omitempty"`              // OS-only: ["darwin", "linux"] (any arch)
	Arch           string   `toml:"arch,omitempty"`            // Architecture filter: "amd64", "arm64"
	LinuxFamily    string   `toml:"linux_family,omitempty"`    // Linux family filter: "debian", "rhel", etc.
	PackageManager string   `toml:"package_manager,omitempty"` // Runtime check (brew, apt, etc.)
}

// IsEmpty returns true if the when clause has no conditions (matches all platforms).
func (w *WhenClause) IsEmpty() bool {
	return w == nil ||
		(len(w.Platform) == 0 && len(w.OS) == 0 &&
			w.Arch == "" && w.LinuxFamily == "" && w.PackageManager == "")
}

// Matches returns true if the when clause conditions are satisfied for the given target.
// Empty clause matches all platforms. Checks platform array first, then OS array, then
// individual dimension filters (Arch, LinuxFamily).
func (w *WhenClause) Matches(target Matchable) bool {
	if w.IsEmpty() {
		return true // No conditions = match all platforms
	}

	os := target.OS()
	arch := target.Arch()
	linuxFamily := target.LinuxFamily()

	// Check platform tuples first (exact match)
	if len(w.Platform) > 0 {
		tuple := fmt.Sprintf("%s/%s", os, arch)
		for _, p := range w.Platform {
			if p == tuple {
				return true
			}
		}
		return false // Had platform conditions but didn't match
	}

	// Check OS array
	if len(w.OS) > 0 {
		osMatch := false
		for _, o := range w.OS {
			if o == os {
				osMatch = true
				break
			}
		}
		if !osMatch {
			return false // Had OS conditions but didn't match
		}
	}

	// Check Arch filter
	if w.Arch != "" && w.Arch != arch {
		return false
	}

	// Check LinuxFamily filter
	if w.LinuxFamily != "" && w.LinuxFamily != linuxFamily {
		return false
	}

	return true
}

// Step represents a single action step
// We use interface{} for the full step data and extract fields manually
type Step struct {
	Action      string
	When        *WhenClause
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

	// Parse when clause
	if whenData, ok := stepMap["when"].(map[string]interface{}); ok {
		s.When = &WhenClause{}

		// Parse platform array
		if platformData, ok := whenData["platform"]; ok {
			switch v := platformData.(type) {
			case []interface{}:
				s.When.Platform = make([]string, 0, len(v))
				for _, item := range v {
					if str, ok := item.(string); ok {
						s.When.Platform = append(s.When.Platform, str)
					}
				}
			case string:
				// Single string value, convert to array
				s.When.Platform = []string{v}
			}
		}

		// Parse os array
		if osData, ok := whenData["os"]; ok {
			switch v := osData.(type) {
			case []interface{}:
				s.When.OS = make([]string, 0, len(v))
				for _, item := range v {
					if str, ok := item.(string); ok {
						s.When.OS = append(s.When.OS, str)
					}
				}
			case string:
				// Single string value, convert to array
				s.When.OS = []string{v}
			}
		}

		// Parse package_manager
		if pm, ok := whenData["package_manager"].(string); ok {
			s.When.PackageManager = pm
		}

		// Parse arch
		if arch, ok := whenData["arch"].(string); ok {
			s.When.Arch = arch
		}

		// Parse linux_family
		if linuxFamily, ok := whenData["linux_family"].(string); ok {
			s.When.LinuxFamily = linuxFamily
		}

		// Validate mutual exclusivity
		if len(s.When.Platform) > 0 && len(s.When.OS) > 0 {
			return fmt.Errorf("when clause cannot have both 'platform' and 'os' fields")
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
	if s.When != nil && !s.When.IsEmpty() {
		whenMap := make(map[string]interface{})
		if len(s.When.Platform) > 0 {
			whenMap["platform"] = s.When.Platform
		}
		if len(s.When.OS) > 0 {
			whenMap["os"] = s.When.OS
		}
		if s.When.Arch != "" {
			whenMap["arch"] = s.When.Arch
		}
		if s.When.LinuxFamily != "" {
			whenMap["linux_family"] = s.When.LinuxFamily
		}
		if s.When.PackageManager != "" {
			whenMap["package_manager"] = s.When.PackageManager
		}
		result["when"] = whenMap
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

// Constraint represents platform requirements for a step.
// Answers: "where can this step run?"
// nil constraint means unconstrained (runs anywhere).
type Constraint struct {
	OS          string // e.g., "linux", "darwin", or empty (any)
	Arch        string // e.g., "amd64", "arm64", or empty (any)
	LinuxFamily string // e.g., "debian", or empty (any linux)
}

// Clone returns a copy of the constraint, or an empty constraint if c is nil.
// Nil-safe: can be called on a nil receiver (idiomatic Go pattern).
func (c *Constraint) Clone() *Constraint {
	if c == nil {
		return &Constraint{}
	}
	return &Constraint{
		OS:          c.OS,
		Arch:        c.Arch,
		LinuxFamily: c.LinuxFamily,
	}
}

// Validate returns an error if the constraint contains invalid combinations.
// Invalid state: LinuxFamily set when OS is not "linux" (or empty).
func (c *Constraint) Validate() error {
	if c == nil {
		return nil
	}
	// LinuxFamily is only valid when OS is empty (implies linux) or OS is "linux"
	if c.LinuxFamily != "" && c.OS != "" && c.OS != "linux" {
		return fmt.Errorf("linux_family %q is only valid with os=\"linux\", got os=%q", c.LinuxFamily, c.OS)
	}
	return nil
}

// MergeWhenClause merges an explicit when clause with an implicit constraint.
// Returns the merged constraint, or error if any dimension conflicts.
// Used during step analysis to combine action constraints with explicit when clauses.
func MergeWhenClause(implicit *Constraint, when *WhenClause) (*Constraint, error) {
	result := implicit.Clone() // nil-safe from Clone()

	// If no when clause, just return the cloned implicit constraint
	if when == nil {
		return result, nil
	}

	// Check Platform array conflict (e.g., apt_install + when.platform: ["darwin/arm64"])
	// Platform entries are "os/arch" tuples that must be compatible with implicit OS
	if len(when.Platform) > 0 && result.OS != "" {
		compatible := false
		for _, p := range when.Platform {
			if os, _, _ := strings.Cut(p, "/"); os == result.OS {
				compatible = true
				break
			}
		}
		if !compatible {
			return nil, fmt.Errorf("platform conflict: action requires OS %q but when.platform specifies %v",
				result.OS, when.Platform)
		}
	}

	// Check OS conflict
	// Note: when.os = ["linux", "darwin"] (multi-OS) leaves result.OS empty
	// because we can't pick one. This is intentional - step runs on multiple OSes.
	if len(when.OS) > 0 {
		if result.OS != "" && !slices.Contains(when.OS, result.OS) {
			return nil, fmt.Errorf("OS conflict: action requires %q but when clause specifies %v",
				result.OS, when.OS)
		}
		if result.OS == "" && len(when.OS) == 1 {
			result.OS = when.OS[0]
		}
		// Multi-OS case: result.OS stays empty (unconstrained within the listed OSes)
	}

	// Check LinuxFamily conflict
	if when.LinuxFamily != "" {
		if result.LinuxFamily != "" && result.LinuxFamily != when.LinuxFamily {
			return nil, fmt.Errorf("linux_family conflict: action requires %q but when clause specifies %q",
				result.LinuxFamily, when.LinuxFamily)
		}
		result.LinuxFamily = when.LinuxFamily
	}

	// Check Arch conflict
	if when.Arch != "" {
		if result.Arch != "" && result.Arch != when.Arch {
			return nil, fmt.Errorf("arch conflict: action requires %q but when clause specifies %q",
				result.Arch, when.Arch)
		}
		result.Arch = when.Arch
	}

	// Validate final constraint (catches invalid combinations like darwin+debian)
	if err := result.Validate(); err != nil {
		return nil, err
	}

	return result, nil
}

// StepAnalysis combines constraint with variation detection.
// Stored on Step after construction (pre-computed at load time).
type StepAnalysis struct {
	Constraint    *Constraint // nil means unconstrained (runs anywhere)
	FamilyVarying bool        // true if step uses {{linux_family}} interpolation
}

// knownVars lists interpolation variables that affect platform variance.
var knownVars = []string{"linux_family", "os", "arch"}

// interpolationPattern matches {{varname}} for known variables.
// Built dynamically from knownVars to ensure consistency.
var interpolationPattern = regexp.MustCompile(`\{\{(` + strings.Join(knownVars, "|") + `)\}\}`)

// detectInterpolatedVars scans for {{var}} patterns in any string value.
// Returns a map of variable names found (e.g., {"linux_family": true}).
// Generalized to support future variables like {{arch}}.
func detectInterpolatedVars(v interface{}) map[string]bool {
	result := make(map[string]bool)
	detectInterpolatedVarsInto(v, result)
	return result
}

// detectInterpolatedVarsInto recursively scans v for interpolated variables,
// adding found variables to the result map.
func detectInterpolatedVarsInto(v interface{}, result map[string]bool) {
	if v == nil {
		return
	}

	switch val := v.(type) {
	case string:
		matches := interpolationPattern.FindAllStringSubmatch(val, -1)
		for _, match := range matches {
			if len(match) > 1 {
				result[match[1]] = true
			}
		}
	case map[string]interface{}:
		for _, mapVal := range val {
			detectInterpolatedVarsInto(mapVal, result)
		}
	case []interface{}:
		for _, item := range val {
			detectInterpolatedVarsInto(item, result)
		}
	}
}

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
		// Only process binaries from actions that install binaries, not actions that modify them (like set_rpath, chmod)
		// Installation actions that provide binaries to be symlinked to $TSUKU_HOME/bin/:
		installActions := map[string]bool{
			"install_binaries": true,
			"download_archive": true,
			"github_archive":   true,
			"github_file":      true,
			"npm_install":      true,
		}
		if !installActions[step.Action] {
			continue
		}

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

		// Check for 'binaries' parameter (plural, used by download_archive, github_archive, install_binaries)
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

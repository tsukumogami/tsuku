package recipe

import (
	"fmt"
	"path/filepath"
)

// Recipe represents an action-based recipe
type Recipe struct {
	Metadata MetadataSection `toml:"metadata"`
	Version  VersionSection  `toml:"version"`
	Steps    []Step          `toml:"steps"`
	Verify   VerifySection   `toml:"verify"`
}

// MetadataSection contains recipe metadata
type MetadataSection struct {
	Name                string   `toml:"name"`
	Description         string   `toml:"description"`
	Homepage            string   `toml:"homepage"`
	VersionFormat       string   `toml:"version_format"`
	RequiresSudo        bool     `toml:"requires_sudo"`
	Dependencies        []string `toml:"dependencies"`          // Install-time dependencies
	RuntimeDependencies []string `toml:"runtime_dependencies"`  // Runtime dependencies (must be exposed)
	Tier                int      `toml:"tier"`                  // Installation tier: 1=binary, 2=package manager, 3=nix
}

// VersionSection specifies how to resolve versions
type VersionSection struct {
	Source     string `toml:"source"`      // e.g., "nodejs_dist", "github_releases", "npm_registry"
	GitHubRepo string `toml:"github_repo"` // e.g., "rust-lang/rust" - use GitHub for version detection only
	TagPrefix  string `toml:"tag_prefix"`  // e.g., "ruby-" - filter tags by prefix and strip it from version
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

// VerifySection defines how to verify the installation
type VerifySection struct {
	Command    string             `toml:"command"`
	Pattern    string             `toml:"pattern"`
	Additional []AdditionalVerify `toml:"additional,omitempty"`
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
// by looking at common action parameters (binaries, executables, etc.)
func (r *Recipe) ExtractBinaries() []string {
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

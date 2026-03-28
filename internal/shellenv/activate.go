// Package shellenv computes per-directory PATH activation for tsuku projects.
// A project directory with a .tsuku.toml file declares tool requirements;
// ComputeActivation resolves those to concrete bin directories under
// $TSUKU_HOME/tools and builds a modified PATH.
package shellenv

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/project"
)

// ActivationResult holds the computed environment changes for a project
// directory activation.
type ActivationResult struct {
	PATH     string   // new PATH value with project tool bin dirs prepended
	Dir      string   // project directory (set as _TSUKU_DIR)
	PrevPath string   // original PATH before activation (set as _TSUKU_PREV_PATH)
	Active   bool     // true when activating, false when deactivating
	Skipped  []string // tools skipped because their version is not installed
}

// ComputeActivation determines the PATH changes needed for the current
// working directory. It reads .tsuku.toml via project.LoadProjectConfig,
// resolves tool bin directories via cfg.ToolBinDir, and builds a prepended
// PATH.
//
// Returns nil (no-op) when:
//   - cwd equals curDir (directory has not changed)
//   - no .tsuku.toml is found and no prior activation exists
//
// Returns a deactivation result (Active=false) when no .tsuku.toml is found
// but prevPath is set, indicating the user left a project directory.
//
// prevPath is the original PATH saved before any prior activation
// (_TSUKU_PREV_PATH). curDir is the last activated directory (_TSUKU_DIR).
func ComputeActivation(cwd, prevPath, curDir string, cfg *config.Config) (*ActivationResult, error) {
	// Early exit: no directory change.
	if cwd != "" && curDir != "" && cwd == curDir {
		return nil, nil
	}

	result, err := project.LoadProjectConfig(cwd)
	if err != nil {
		return nil, fmt.Errorf("loading project config: %w", err)
	}
	if result == nil {
		if prevPath != "" {
			// Was active, now leaving project directory -- deactivate.
			return &ActivationResult{
				PATH:   prevPath,
				Active: false,
			}, nil
		}
		// No prior activation and no project config -- no-op.
		return nil, nil
	}

	// Determine the base PATH: use prevPath if we already have an activation,
	// otherwise use the current PATH from the environment.
	basePath := prevPath
	if basePath == "" {
		basePath = os.Getenv("PATH")
	}

	// Collect tool bin directories. Sort tool names for deterministic output.
	toolNames := make([]string, 0, len(result.Config.Tools))
	for name := range result.Config.Tools {
		toolNames = append(toolNames, name)
	}
	sort.Strings(toolNames)

	var binDirs []string
	var skipped []string

	for _, name := range toolNames {
		req := result.Config.Tools[name]
		if req.Version == "" {
			// No version pinned -- skip (would need resolution, out of scope
			// for the activation skeleton).
			skipped = append(skipped, name)
			continue
		}

		binDir := cfg.ToolBinDir(name, req.Version)
		if _, err := os.Stat(binDir); os.IsNotExist(err) {
			skipped = append(skipped, name)
			continue
		}

		abs, err := filepath.Abs(binDir)
		if err != nil {
			skipped = append(skipped, name)
			continue
		}
		binDirs = append(binDirs, abs)
	}

	// Build new PATH: tool bin dirs prepended to base PATH.
	var newPath string
	if len(binDirs) > 0 {
		newPath = strings.Join(binDirs, ":") + ":" + basePath
	} else {
		newPath = basePath
	}

	return &ActivationResult{
		PATH:     newPath,
		Dir:      result.Dir,
		PrevPath: basePath,
		Active:   true,
		Skipped:  skipped,
	}, nil
}

// FormatExports renders the activation result as shell export statements for
// the given shell. Supported shells: "bash", "zsh", "fish".
func FormatExports(result *ActivationResult, shell string) string {
	if result == nil {
		return ""
	}

	var b strings.Builder

	if !result.Active {
		// Deactivation: restore PATH and unset tracking variables.
		switch shell {
		case "fish":
			fmt.Fprintf(&b, "set -gx PATH %q\n", result.PATH)
			fmt.Fprintf(&b, "set -e _TSUKU_DIR\n")
			fmt.Fprintf(&b, "set -e _TSUKU_PREV_PATH\n")
		default: // bash, zsh
			fmt.Fprintf(&b, "export PATH=%q\n", result.PATH)
			fmt.Fprintf(&b, "unset _TSUKU_DIR _TSUKU_PREV_PATH\n")
		}
		return b.String()
	}

	switch shell {
	case "fish":
		fmt.Fprintf(&b, "set -gx PATH %q\n", result.PATH)
		fmt.Fprintf(&b, "set -gx _TSUKU_DIR %q\n", result.Dir)
		fmt.Fprintf(&b, "set -gx _TSUKU_PREV_PATH %q\n", result.PrevPath)
	default: // bash, zsh
		fmt.Fprintf(&b, "export PATH=%q\n", result.PATH)
		fmt.Fprintf(&b, "export _TSUKU_DIR=%q\n", result.Dir)
		fmt.Fprintf(&b, "export _TSUKU_PREV_PATH=%q\n", result.PrevPath)
	}

	return b.String()
}

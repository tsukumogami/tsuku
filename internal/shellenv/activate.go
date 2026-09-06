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
	"github.com/tsukumogami/tsuku/internal/shellquote"
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

	// Stderr, never stdout: hook-env's stdout is what the shell hook evaluates,
	// and these lines quote a key that came from the config file.
	//
	// Below the nil check rather than above it. It read correctly above only
	// because FprintDiagnostics guards its own nil receiver, which is a subtle
	// thing to rest on when the equivalent placement needs no guard at all: a
	// nil result means no config was found, so there is nothing to report.
	result.FprintDiagnostics(os.Stderr)

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

		// Compose the path from the bare name, not the declaration key. For an
		// org-scoped entry the key is "owner/repo:tool", which the boundary
		// validated as three separate components -- and it validated them
		// separately precisely because the whole key is not a path component.
		// Passing the key here would compose <tools>/owner/repo:tool-1.0/bin
		// and put a colon inside one PATH entry, which the join below then
		// splits in two: the same PATH-separator failure the name rule exists
		// to prevent, arriving through a value the boundary approved. The
		// resolver already splits; this is the sink that did not.
		_, bare, _, err := project.SplitOrgKey(name)
		if err != nil {
			skipped = append(skipped, name)
			continue
		}

		binDir := cfg.ToolBinDir(bare, req.Version)
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

// setVar writes one assignment that gives the shell a value and nothing else.
//
// It takes the value rather than a format string, deliberately. The defect this
// replaces was eight fmt.Fprintf calls using %q, and %q is a Go string-literal
// quoter: it leaves $ and the backtick live inside the double quotes it
// produces. Routing values through a helper that quotes internally means a new
// emitted variable cannot reintroduce that by forgetting to call something --
// there is no format string left to get wrong.
func setVar(b *strings.Builder, shell, name, value string) {
	switch shell {
	case "fish":
		fmt.Fprintf(b, "set -gx %s %s\n", name, shellquote.Fish(value))
	default: // bash, zsh
		fmt.Fprintf(b, "export %s=%s\n", name, shellquote.POSIX(value))
	}
}

// FormatExports renders the activation result as shell export statements for
// the given shell. Supported shells: "bash", "zsh", "fish".
//
// Every emitted value is quoted for the target dialect. That matters because
// the shell hooks evaluate this output -- eval "$(tsuku hook-env bash)" -- and
// the values are not tsuku's own: PATH carries tool directories built from a
// project config, and _TSUKU_DIR is the directory holding that config, whose
// name is chosen by whoever authored the repository the user cloned.
func FormatExports(result *ActivationResult, shell string) string {
	if result == nil {
		return ""
	}

	var b strings.Builder

	if !result.Active {
		// Deactivation: restore PATH and unset tracking variables.
		setVar(&b, shell, "PATH", result.PATH)
		switch shell {
		case "fish":
			fmt.Fprintf(&b, "set -e _TSUKU_DIR\n")
			fmt.Fprintf(&b, "set -e _TSUKU_PREV_PATH\n")
		default: // bash, zsh
			fmt.Fprintf(&b, "unset _TSUKU_DIR _TSUKU_PREV_PATH\n")
		}
		return b.String()
	}

	setVar(&b, shell, "PATH", result.PATH)
	setVar(&b, shell, "_TSUKU_DIR", result.Dir)
	setVar(&b, shell, "_TSUKU_PREV_PATH", result.PrevPath)

	return b.String()
}

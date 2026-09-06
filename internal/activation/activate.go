// Package activation computes per-directory PATH activation for tsuku projects.
//
// RESOLVED REBASE NOTE, carried from internal/shellenv/activate.go.
//
// The security work in #2563 left a warning here for whoever rebased #2554 over
// it: that PR moves this file into this package and rewrites the emitter around
// a shared helper, and taking its side of the conflict would have compiled
// cleanly, passed review as a package move, and silently reverted the quoting
// fix -- %q is a Go string-literal quoter, so $ and backticks stay live inside
// the double quotes it produces, and the shell hook evaluates this output.
//
// That resolution is done: setVar's body is kept and the pins-side additions
// are layered on it, which is what the warning asked for.
//
// It is kept rather than deleted because the reasoning is not recoverable from
// the code. TestFormatExports_HostileValuesDoNotExecute is what catches a
// revert -- mutation-verified -- and injection_test.go and
// diagnostic_binding_test.go moved into this package with the emitter rather
// than being deleted to clear the build errors the move caused.
//
// The original closed with "this warning lives here because the person
// resolving that conflict is working in this one." That was true of the file it
// was written in and stopped being true the moment the move it warned about
// happened, which is the note's own subject arriving one level up.
// A project directory with a .tsuku.toml file declares tool requirements;
// ComputeActivation resolves those against the versions installation state
// records and builds a modified PATH.
//
// This lives outside internal/shellenv because internal/install imports that
// package for its shell.d cache and PATH-precedence helpers, which would make
// the dependency activation needs -- on the pin-matching routines, the version
// ordering, and installation state -- a cycle. Activation shares no symbol with
// the rest of shellenv, so the split costs nothing and tells the truth about
// the dependency graph.
//
// Invariant: internal/install must never import this package, nor must anything
// in internal/install's dependency graph. That is the cycle this split exists
// to avoid, and re-creating it elsewhere would re-create the bug.
package activation

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/project"
	"github.com/tsukumogami/tsuku/internal/shellquote"
)

// InstalledSet reads what installation state records. It is declared here, by
// the consumer, and is one method wide, so activation reaches internal/install
// for the pin routines and nothing else.
type InstalledSet interface {
	// InstalledVersionsFor returns the recorded versions for each requested
	// name. Names with no entry are absent from the map rather than present
	// and empty. The returned slices are in no particular order.
	InstalledVersionsFor(names []string) (map[string][]string, error)
}

// ActivationResult holds the computed environment changes for a project
// directory activation.
type ActivationResult struct {
	PATH     string // new PATH value with project tool bin dirs prepended
	Dir      string // project directory (set as _TSUKU_DIR)
	PrevPath string // original PATH before activation (set as _TSUKU_PREV_PATH)
	Active   bool   // true when activating, false when deactivating

	// Unhonorable holds the declarations that put nothing on PATH, in PATH
	// order, each with the reason it was not honored.
	Unhonorable []Unhonorable
	// Unreadable is non-nil when installation state could not be read at all,
	// in which case no declaration needing that read was classified.
	Unreadable *StateUnreadable
}

// ComputeActivation determines the PATH changes needed for the current
// working directory. It reads .tsuku.toml via project.LoadProjectConfig,
// resolves each declaration against the versions installation state records,
// and builds a prepended PATH.
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
// installed supplies the recorded versions and is read at most once per call.
func ComputeActivation(cwd, prevPath, curDir string, cfg *config.Config, installed InstalledSet) (*ActivationResult, error) {
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

	// Sort the declared keys so PATH order does not depend on map iteration.
	// This lexical ordering is the one the previous loop established and it
	// survives the rewrite unchanged; the resolution below is the body of this
	// loop, not a replacement for it.
	toolNames := make([]string, 0, len(result.Config.Tools))
	for name := range result.Config.Tools {
		toolNames = append(toolNames, name)
	}
	sort.Strings(toolNames)

	var unhonorable []Unhonorable

	// First pass: everything decidable without installation state. Keeping it
	// separate is what lets an unreadable state file name only the
	// declarations that actually needed the read.
	pending := make([]declaration, 0, len(toolNames))
	for _, name := range toolNames {
		d, u := classifyForm(name, result.Config.Tools[name].Version)
		if u != nil {
			unhonorable = append(unhonorable, *u)
			continue
		}
		pending = append(pending, d)
	}

	// One read for every remaining declaration.
	var recorded map[string][]string
	var unreadable *StateUnreadable
	if len(pending) > 0 {
		names := make([]string, 0, len(pending))
		for _, d := range pending {
			names = append(names, d.bare)
		}
		var err error
		recorded, err = installed.InstalledVersionsFor(names)
		if err != nil {
			tools := make([]string, 0, len(pending))
			for _, d := range pending {
				tools = append(tools, d.key)
			}
			unreadable = &StateUnreadable{Tools: tools, Err: err}
			pending = nil
		}
	}

	var binDirs []string
	for _, d := range pending {
		binDir, u := selectVersion(d, recorded[d.bare], cfg)
		if u != nil {
			unhonorable = append(unhonorable, *u)
			continue
		}
		binDirs = append(binDirs, binDir)
	}

	// Build new PATH: tool bin dirs prepended to base PATH.
	var newPath string
	if len(binDirs) > 0 {
		newPath = strings.Join(binDirs, ":") + ":" + basePath
	} else {
		newPath = basePath
	}

	return &ActivationResult{
		PATH:        newPath,
		Dir:         result.Dir,
		PrevPath:    basePath,
		Active:      true,
		Unhonorable: unhonorable,
		Unreadable:  unreadable,
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

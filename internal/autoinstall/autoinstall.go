// Package autoinstall provides the install-then-exec flow used by
// `tsuku run` and (future) `tsuku exec`. It owns consent mode resolution,
// binary index lookup, installation, and process handoff via syscall.Exec.
package autoinstall

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
	"github.com/tsukumogami/tsuku/internal/project"
)

// Mode controls the consent behavior for auto-install.
type Mode int

const (
	// ModeConfirm prompts the user interactively before installing.
	// This is the default mode.
	ModeConfirm Mode = iota

	// ModeSuggest prints install instructions and exits without installing.
	ModeSuggest

	// ModeAuto installs silently with audit logging.
	// Requires explicit opt-in via config and flag/env corroboration.
	ModeAuto
)

// String returns the string representation of a Mode.
func (m Mode) String() string {
	switch m {
	case ModeConfirm:
		return "confirm"
	case ModeSuggest:
		return "suggest"
	case ModeAuto:
		return "auto"
	default:
		return "unknown"
	}
}

// ParseMode converts a string to a Mode. Returns ok=false for invalid strings.
func ParseMode(s string) (Mode, bool) {
	switch s {
	case "confirm":
		return ModeConfirm, true
	case "suggest":
		return ModeSuggest, true
	case "auto":
		return ModeAuto, true
	default:
		return 0, false
	}
}

// ProjectDeclarationResolver reports which of a command's providers the
// project declared. The implementation is *project.Resolver; pass nil to
// Runner.Run to run as if no .tsuku.toml existed.
//
// It takes the matches rather than the command because Run has already looked
// the command up, and an implementation that resolved from the command would
// open the binary index a second time on every run that finds a config.
//
// The set is how "not declared" is reported, in place of the ok bool the
// previous single-value accessor returned: an empty result is no declaration,
// one element is the recipe to narrow to, and two or more is an ambiguity the
// caller refuses rather than settles.
type ProjectDeclarationResolver interface {
	DeclarationsFor(ctx context.Context, matches []index.BinaryMatch) ([]project.ProjectDeclaration, error)
}

// AmbiguousDeclarationError reports that the project declared more than one
// recipe providing the executed command. Nothing is installed and nothing is
// executed: the file that was meant to settle which provider to use named
// several, and picking one would choose on an ordering the user never
// expressed.
type AmbiguousDeclarationError struct {
	Command      string
	Declarations []project.ProjectDeclaration
}

// Error names the command and every declared recipe, each with the
// configuration key that declared it.
//
// The key is not decoration even in this interim message. Recipe is a match
// key rather than an identity -- two declarations in one set carry the same
// bare name whenever a configuration names one recipe from two registries --
// so a message built from Recipe alone reads "koto and koto" and reinstates
// the confusion the declaration set exists to remove.
//
// What this message does not yet carry is each declaration's version and an
// invocation that reaches a specific one of the recipes. Those come with the
// refusal proper, which formats this error for the user.
func (e *AmbiguousDeclarationError) Error() string {
	named := make([]string, 0, len(e.Declarations))
	for _, d := range e.Declarations {
		named = append(named, fmt.Sprintf("%s (declared as %q)", d.Recipe, d.ConfigKey))
	}
	return fmt.Sprintf("autoinstall: the project declares %d recipes providing %q: %s",
		len(e.Declarations), e.Command, strings.Join(named, ", "))
}

// Installer performs the actual tool installation. cmd/tsuku wires this
// to the full install pipeline (recipe loading, version resolution, etc.).
type Installer interface {
	Install(ctx context.Context, recipe, version string) error
}

// LookupFunc looks up a command in the binary index.
// This is typically bound to cmd/tsuku/lookup.go's lookupBinaryCommand.
type LookupFunc func(ctx context.Context, command string) ([]index.BinaryMatch, error)

// ExecFunc replaces the current process with the given command.
// On Unix this is syscall.Exec; tests inject a no-op or recorder.
type ExecFunc func(binary string, args []string, env []string) error

// Runner executes the install-then-exec flow.
type Runner struct {
	cfg    *config.Config
	stdout io.Writer
	stderr io.Writer

	// ConsentReader is the source for interactive consent input.
	// Inject a bytes.Buffer or strings.Reader in tests.
	ConsentReader io.Reader

	// Lookup resolves a command name to binary index matches.
	Lookup LookupFunc

	// Installer performs the recipe installation.
	Installer Installer

	// Exec replaces the process with the installed binary.
	Exec ExecFunc

	// RecipeHasVerification checks whether a recipe has checksum_url or
	// signature_url. Used by the verification security gate (auto mode).
	// Returns true if the recipe has at least one verification method.
	RecipeHasVerification func(recipe string) bool
}

// NewRunner creates a Runner with the given config and I/O writers.
func NewRunner(cfg *config.Config, stdout, stderr io.Writer) *Runner {
	return &Runner{
		cfg:    cfg,
		stdout: stdout,
		stderr: stderr,
	}
}

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

	// ModeAuto installs without prompting. Requires explicit opt-in via
	// config and flag/env corroboration.
	//
	// It is not the mode audit logging is attached to: every install is
	// recorded, whichever mode governed it, and the entry names that mode.
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

// Origin names where the consent mode came from, before any gate ran.
//
// It exists because the mode alone does not say whether anyone chose it, and
// the bounded elevation turns on exactly that. A confirm nobody asked for and
// a confirm the user set by flag are the same Mode and different states: the
// first is a default a project declaration may raise, the second is a choice
// it may not. So does the escalation restriction's output -- an environment
// variable asking for auto that the persistent config did not corroborate
// arrives here as a confirm whose origin is environment, and raising it would
// reverse the control that produced it.
//
// Five values are the ones the durable record names, and they are exhaustive
// for it: a gate that lowers the mode afterwards is recorded as a gate rather
// than as an origin. The sixth, OriginUnset, is the zero value and is not one
// of them -- see its own note for why it exists.
type Origin int

const (
	// OriginUnset is the zero value, and it is not an origin: it is what a
	// caller that resolved none has. It raises nothing, which is the direction
	// this type has to fail in.
	//
	// Without it OriginDefault would be the zero value, and OriginDefault is
	// the one origin a declaration may raise -- so an entry point that forgot
	// to resolve an origin would not merely lose the bound, it would turn a
	// suggest the user set into an unattended install, which is the defect
	// this whole rule exists to remove. The package defaults in the same
	// direction elsewhere: a nil IsTerminal is no terminal, and a nil
	// RecipeHasVerification is unverified.
	OriginUnset Origin = iota

	// OriginDefault means nothing set a mode: no flag, no environment
	// variable, no configuration key. This is the only origin a project
	// declaration may raise, and it is a resolved answer rather than an
	// absent one -- resolveMode returns it after reading all three sources
	// and finding none of them set.
	OriginDefault

	// OriginFlag is --mode on the command line.
	OriginFlag

	// OriginEnvironment is TSUKU_AUTO_INSTALL_MODE, including the confirm the
	// escalation restriction substitutes for an uncorroborated auto.
	OriginEnvironment

	// OriginConfig is auto_install_mode in $TSUKU_HOME/config.toml.
	OriginConfig

	// OriginProject is a mode a project declaration raised. It outranks
	// OriginDefault and nothing else, so it is the recorded origin only where
	// the mode would otherwise have been the unset default.
	//
	// Like every value here it says where the mode came from, not what the
	// mode ended up being: a gate may have lowered it since, and for a recipe
	// carrying no checksum one ordinarily has. A reader of this value cannot
	// conclude that anything was installed unattended.
	OriginProject
)

// String returns the string representation of an Origin, which is the spelling
// the record uses.
func (o Origin) String() string {
	switch o {
	case OriginUnset:
		return "unset"
	case OriginDefault:
		return "default"
	case OriginFlag:
		return "flag"
	case OriginEnvironment:
		return "environment"
	case OriginConfig:
		return "config"
	case OriginProject:
		return "project"
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
// The key is not decoration: project.ProjectDeclaration documents why Recipe
// cannot identify a declaration, and two registries named for one recipe are
// the case that makes a Recipe-only message useless.
//
// This is the one-line summary, for a caller that wraps or logs the error. The
// refusal the user reads is Runner.refusalMessage, which the run path has
// already printed by the time this error is returned -- it carries each
// declaration's version and an invocation reaching a specific recipe as well,
// which R6 requires and a single line has nowhere to put.
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

	// IsTerminal reports whether a prompt written to stdout can be answered.
	// cmd/tsuku wires this to stdin's terminal check.
	//
	// A nil function means no terminal. A caller that never wired one cannot
	// answer a prompt, so the refusal is the right outcome for it -- the
	// alternative is a prompt written into a stream nobody reads, which is
	// the failure the check exists to prevent rather than a lenient default.
	IsTerminal func() bool

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

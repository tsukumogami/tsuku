package autoinstall

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/tsukumogami/tsuku/internal/index"
	"github.com/tsukumogami/tsuku/internal/project"
)

// Sentinel errors for exit code mapping in cmd/tsuku.
var (
	// ErrIndexNotBuilt wraps index.ErrIndexNotBuilt for callers that don't
	// import the index package.
	ErrIndexNotBuilt = index.ErrIndexNotBuilt

	// ErrForbidden indicates the operation was blocked for security reasons.
	ErrForbidden = errors.New("autoinstall: forbidden")

	// ErrUserDeclined indicates the user declined the install prompt.
	ErrUserDeclined = errors.New("autoinstall: user declined")

	// ErrNoMatch indicates no recipe provides the requested command.
	ErrNoMatch = errors.New("autoinstall: no matching recipe")

	// ErrNotInteractive indicates confirm mode was reached with no terminal
	// to prompt on. cmd/tsuku maps it to ExitNotInteractive, the code the
	// command layer's own check exited with before this one replaced it.
	ErrNotInteractive = errors.New("autoinstall: confirm mode requires a terminal")

	// ErrSuggestOnly is returned in suggest mode after printing instructions.
	ErrSuggestOnly = errors.New("autoinstall: suggest mode, not installing")
)

// auditEntry is one line of the NDJSON audit log.
//
// Mode, Origin and Gate are three facts and none of them can be read off
// another. Mode is what governed the install. Origin is the source the mode
// was resolved from, before any gate ran (R12). Gate names the mode-lowering
// gate that changed it, where one did (R12a), and is absent where none fired.
//
// The pair that makes the separation load-bearing is a confirm the user asked
// for and a confirm a gate produced by lowering an auto: same Mode, and the
// second is the run that did something the user did not ask for. An entry
// carrying only the mode cannot tell them apart, which is the same as not
// having recorded where the mode came from.
type auditEntry struct {
	Timestamp string `json:"ts"`
	Action    string `json:"action"`
	Recipe    string `json:"recipe"`
	Version   string `json:"version"`
	Mode      string `json:"mode"`
	Origin    string `json:"origin"`
	Gate      string `json:"gate,omitempty"`
}

// candidates looks command up in the binary index and narrows the result to
// the recipe the project declared, where it declared exactly one that provides
// the command.
//
//   - no declaration provides it: matches unchanged, nil declaration. The
//     command resolves exactly as it would with no .tsuku.toml present (R5).
//   - exactly one does: matches filtered to that recipe, plus the declaration.
//   - more than one does: the refusal is printed and an
//     AmbiguousDeclarationError carrying all of them is returned (R6).
//
// On a nil error the returned list has at least one element. Run reads
// position zero without checking, and the two guards below -- the ErrNoMatch
// return and the one at the narrowing -- are what keep that true.
//
// The narrowing lives here, at the one site the list is produced, rather than
// at each of the consumers below. That is what makes the consumers correct by
// inheritance in the declared case: they go on reading position zero, and
// position zero is now the declared recipe rather than whichever provider the
// index happened to rank first. It is also what makes the property reviewable:
// the region above the narrowing inside Run is empty by construction, because
// the list does not exist there at all, so there is nowhere above it for a
// positional read to hide.
//
// "Correct by inheritance" holds for the declared case and is not a general
// property of the consumers. Where nothing is declared they read position zero
// of a list the index ranked, which is what R5 requires -- so the zero case is
// a passthrough rather than a narrowing to nothing. The ErrNoMatch check stays
// above the branch on the raw list for the adjacent reason: an empty index
// result is a lookup failure rather than a declaration outcome.
func (r *Runner) candidates(ctx context.Context, command string, resolver ProjectDeclarationResolver) ([]index.BinaryMatch, *project.ProjectDeclaration, error) {
	if r.Lookup == nil {
		return nil, nil, fmt.Errorf("autoinstall: Lookup function not configured")
	}

	matches, err := r.Lookup(ctx, command)
	if err != nil {
		if errors.Is(err, index.ErrIndexNotBuilt) {
			fmt.Fprintf(r.stderr, "Binary index not built. Run 'tsuku update-registry' to build it.\n")
			return nil, nil, ErrIndexNotBuilt
		}
		// StaleIndexWarning: results are still valid.
		var stale index.StaleIndexWarning
		if !errors.As(err, &stale) {
			return nil, nil, fmt.Errorf("autoinstall: lookup failed: %w", err)
		}
		fmt.Fprintf(r.stderr, "Warning: %v\n", err)
	}

	if len(matches) == 0 {
		// Moving the terminal check below this point turned the no-terminal
		// case from an exit code with a line explaining it into a bare exit 1,
		// so this line is what keeps the failure from being silent (AC55).
		//
		// It names no escape hatch, and there is none to name: no recipe
		// provides the command, so every invocation that could be printed here
		// fails the same way this one did. Naming one anyway is the defect
		// R10 is about.
		fmt.Fprintf(r.stderr, "No recipe provides %q.\n", command)
		return nil, nil, ErrNoMatch
	}

	if resolver == nil {
		return matches, nil, nil
	}
	declared, err := resolver.DeclarationsFor(ctx, matches)
	if err != nil {
		return nil, nil, fmt.Errorf("autoinstall: version resolution failed: %w", err)
	}

	switch len(declared) {
	case 0:
		return matches, nil, nil
	case 1:
		declaration := declared[0]
		narrowed := make([]index.BinaryMatch, 0, 1)
		for _, m := range matches {
			if m.Recipe == declaration.Recipe {
				narrowed = append(narrowed, m)
			}
		}
		if len(narrowed) == 0 {
			// Keeps the non-empty postcondition, without which the caller's
			// matches[0] panics. The production resolver derives its answer
			// from the matches it was handed and cannot reach this; the
			// parameter is an interface, so it is checked rather than assumed.
			return nil, nil, fmt.Errorf("autoinstall: the project declared %q for %q, which provides no match",
				declaration.Recipe, command)
		}
		return narrowed, &declaration, nil
	default:
		return nil, nil, r.refuse(&AmbiguousDeclarationError{Command: command, Declarations: declared})
	}
}

// Run executes the install-then-exec flow for a command.
//
// It looks up the command in the binary index, narrows the result to what the
// project declared, applies security gates and the consent mode, installs if
// needed, and hands off execution via syscall.Exec (or the injected ExecFunc).
//
// The resolver parameter reports what the project declared. Pass nil to run as
// if no .tsuku.toml existed.
//
// mode and origin travel together and are both resolved by the caller: the
// mode is what to do, and the origin is who asked for it. The elevation below
// needs the second, because the same Mode means different things depending on
// whether anyone chose it.
func (r *Runner) Run(ctx context.Context, command string, args []string, mode Mode, origin Origin, resolver ProjectDeclarationResolver) error {
	// Security gate 1: root guard.
	if os.Geteuid() == 0 {
		return fmt.Errorf("%w: refusing to auto-install as root", ErrForbidden)
	}

	matches, declaration, err := r.candidates(ctx, command, resolver)
	if err != nil {
		return err
	}

	// The one positional read of the candidate list, and it stays the only
	// one: a consumer added below reads match rather than indexing matches
	// again. That is not style: R3a's review instrument is that a positional
	// read below the narrowing site can be judged by its position instead of
	// by reasoning about what its author meant, and a read that goes through
	// match rather than around it is one fewer place to judge. The conflict
	// gate's len(matches) is the other read of the list and is a count rather
	// than a selection.
	//
	// Indexing without a length check is safe because candidates returns a
	// non-empty list on a nil error.
	match := matches[0]
	version := ""
	if declaration != nil {
		version = declaration.Version
	}

	// Project-declared tool: exec from the version-specific bin directory,
	// not from tools/current/. Install the declared version if needed.
	if declaration != nil {
		binDir := r.cfg.ToolBinDir(match.Recipe, version)
		binaryPath := filepath.Join(binDir, command)

		// If the declared version is already installed, exec directly.
		if _, statErr := os.Stat(binaryPath); statErr == nil {
			return r.execBinary(binaryPath, args)
		}

		// Declared version not installed -- fall through to the consent mode
		// below, which the declaration raises only where nothing set one.
	} else if match.Installed {
		// Nothing declared -- use the globally active version.
		binaryPath := filepath.Join(r.cfg.CurrentDir, command)
		return r.execBinary(binaryPath, args)
	}

	// The bounded elevation. It raises the mode and nothing else: the terminal
	// check below reads the mode rather than the declaration, so a declared
	// command a gate lowers back to confirm meets that check like any other --
	// what the declaration bypasses is the prompt it consented to, not every
	// question there is.
	//
	// The origin the elevation produces is what the record names, and it is
	// taken from here rather than decided again down there. This line is the
	// one place that knows a declaration turned a default into project; a
	// record applying the rule a second time, from the mode and the
	// declaration it can still see, would be a copy free to disagree with this
	// one. The parameter is the wrong value for it to read for the same
	// reason: on an elevated run the parameter still says default.
	//
	// The disclosure below does not read it, and that is the point of the
	// paragraph there rather than an oversight here: what it announces is
	// wider than what this line raised.
	effectiveMode, effectiveOrigin := elevate(mode, origin, declaration != nil)

	// The mode-lowering gates, from the table they are registered in -- however
	// many are registered, which is the point of the table and the reason this
	// no longer counts them. Each announces itself where it fires, which is R11;
	// the traversal stops at the first, because that is the one that *changed*
	// the mode and the ones after it would be reporting a mode they found
	// already lowered.
	//
	// The gate it names goes into the same record the origin above does, and
	// for the same reason: this is the site that walked the table, and a
	// reader recovering the answer from a second walk is where two copies
	// start disagreeing.
	subject := gateSubject{command: command, match: match, matches: matches}
	effectiveMode, loweredBy := r.lowerMode(effectiveMode, subject)

	// The terminal check. It asks whether *this* command needs a prompt, and
	// it asks here because here is the first place that question has an
	// answer: below the declaration lookup, below the two fast paths that
	// return without prompting, and below every gate that can lower a raised
	// mode back to confirm.
	//
	// There is deliberately no declaredness term in the predicate. The command
	// layer's check had one -- it asked whether the configuration declared
	// anything at all -- and both halves of that were wrong. A command nothing
	// declares skipped the check in a repository that declared something else,
	// and then met the prompt at a closed stdin. A command that is declared
	// could be lowered back to confirm by a gate down here, which the check up
	// there could not know. By this line the mode already carries everything a
	// declaration contributes, so asking again would be asking twice.
	if effectiveMode == ModeConfirm && !r.terminalAttached() {
		fmt.Fprintf(r.stderr, "%s\n", r.notInteractiveMessage(subject))
		return ErrNotInteractive
	}

	// The disclosure, and the rule R13a required to be written before the
	// control existed: an install `tsuku run` performs, whose recipe or whose
	// consent mode was determined by a project declaration, states before the
	// install begins the recipe, the version, the path of the file that
	// authorized it, and the recipe's source.
	//
	// The rule names `tsuku run` and so does this control, which is the
	// comparison AC51 makes. It is not that the install path needs no
	// disclosure -- it is that no consent mode governs it and no elevation
	// happens there, so R11a has nothing to be about. D5 carries the reasoning
	// and says plainly what the scope leaves uncovered.
	//
	// It is keyed on the declaration rather than on the elevation, and that is
	// a case rather than a nicety. R3b stops the multiple-provider gate firing
	// for a declared command, so a user in an explicitly configured auto who
	// runs an ambiguous command in a repository declaring one provider now
	// gets a silent install where they previously got a prompt. No elevation
	// happened, so a disclosure keyed on one says nothing; no gate changed the
	// mode, so no announcement above says anything either. A
	// repository-supplied file would have turned a prompt into a silent
	// install with nothing reporting it.
	//
	// Suggest is the one dispatch it is skipped for. Suggest installs nothing,
	// so a per-install disclosure has no install to attach to, and AC30
	// asserts its absence there. Confirm is not skipped and the disclosure
	// precedes the prompt rather than following it: the prompt is where the
	// person decides, and facts delivered after the decision are not
	// disclosure. A run that then declines has disclosed an install that did
	// not happen, which is the harmless direction to be wrong in.
	if declaration != nil && effectiveMode != ModeSuggest {
		r.discloseDeclaration(match, version, declaration.ConfigPath)
	}

	// Mode dispatch.
	switch effectiveMode {
	case ModeSuggest:
		// The instruction names what this run would have installed, which for
		// a declared command means the declared version. Without it a user who
		// followed the line got whatever `latest` resolved to, the declared
		// fast path went on missing the version it stats, and the next
		// `tsuku run` here printed this same line again. That loop closes for
		// an exact pin, which is what the version this prints is. It does not
		// close for a declaration of `latest` or a prefix, because no install
		// puts a binary where the fast path stats for those -- a defect older
		// than this line, which only made it visible.
		//
		// It is built the way the prompt below is built, from the recipe and
		// the version, rather than from the declaration's configuration key. The key carries a source component, and the run
		// path deliberately does not: the recipe name comes from the binary
		// index and the source the key names is never honored here. An
		// instruction carrying it would send a user to the install path, which
		// does honor it -- a different install from the one this run declined
		// to perform, reached through a line a repository-supplied file wrote.
		argument := match.Recipe
		if version != "" {
			argument += "@" + version
		}
		fmt.Fprintf(r.stdout, "Install with: tsuku install %s\n", argument)
		return ErrSuggestOnly

	case ModeConfirm:
		prompt := fmt.Sprintf("Install %s", match.Recipe)
		if version != "" {
			prompt += "@" + version
		}
		prompt += "? [y/N] "
		_, _ = fmt.Fprint(r.stdout, prompt)

		reader := r.ConsentReader
		if reader == nil {
			reader = os.Stdin
		}
		scanner := bufio.NewScanner(reader)
		if !scanner.Scan() {
			return ErrUserDeclined
		}
		answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if answer != "y" && answer != "yes" {
			return ErrUserDeclined
		}

	case ModeAuto:
		// Proceed silently. The record below is written for this dispatch and
		// for confirm alike, so "silently" means no prompt rather than no
		// trace.
	}

	// Install.
	if r.Installer == nil {
		return fmt.Errorf("autoinstall: Installer not configured")
	}
	if err := r.Installer.Install(ctx, match.Recipe, version); err != nil {
		return fmt.Errorf("autoinstall: install failed: %w", err)
	}

	// The record, on every install rather than on the auto ones (R12). The
	// guard that used to stand here was the defect: an install a gate diverted
	// to confirm left no trace at all, so the runs that ended somewhere other
	// than where the configuration pointed were exactly the runs with nothing
	// written about them.
	//
	// It is below the install rather than above it because what it records is
	// an install that happened. A failed install returns before this line, and
	// an entry for one would be a record of something the machine does not
	// have.
	//
	// Suggest never reaches here: its dispatch returns above without
	// installing. That is the reason this is unconditional rather than a
	// condition naming the two modes that install -- the modes that get here
	// are the ones that install, and restating the list would be a second
	// place for it to be wrong.
	writeAuditLog(r.cfg.HomeDir, match.Recipe, version, effectiveMode, effectiveOrigin, loweredBy)

	// Exec the installed binary.
	binaryPath := filepath.Join(r.cfg.CurrentDir, command)
	return r.execBinary(binaryPath, args)
}

// elevate applies the bounded elevation: what mode is in force for this
// command, and which origin the record names for it.
//
// A declaration raises the mode to auto only where the mode is the unset
// default, and only for a command it declared -- declared is false for every
// other command, because the narrowing above has already decided which
// declaration, if any, is this command's.
//
// The bound is the origin rather than the mode, and the mode would not do. The
// case for raising confirm is that confirm is a default nobody chose, and that
// reasoning does not survive someone choosing it -- so an explicitly set mode
// is honored as given, whichever it is. Confirm is also what the escalation
// restriction substitutes for an environment variable asking for auto that the
// persistent config did not corroborate, and raising every confirm would take
// that control's output and put it straight back to auto, which is the reverse
// of what it is for. An origin of default is the one state in which nobody has
// said anything.
//
// Where it raises, the origin becomes project: a declaration outranks default
// and nothing else, so project is recorded exactly where the mode would
// otherwise have been the unset default, and every other source is both
// honored and recorded as itself.
//
// There is no term for the mode, and none is wanted. The only mode an origin
// of default can carry is confirm, because a default is what nobody set and
// confirm is what nobody setting anything produces. A guard against a pair
// that cannot be resolved would state a second rule; what makes the absence
// safe rather than merely tidy is that a caller which *forgets* to resolve an
// origin has OriginUnset, not OriginDefault, and OriginUnset raises nothing.
//
// Scoped to forgetting deliberately. A caller that supplies OriginDefault
// explicitly and wrongly is not covered: elevate(ModeSuggest, OriginDefault,
// true) still returns ModeAuto. That pair is unresolvable rather than unsafe
// -- an origin of default asserts nothing set the mode, a mode of suggest
// asserts something did, and no guard here can tell which half is the lie.
// The absence is defensible for the case named, not for the wider one.
func elevate(mode Mode, origin Origin, declared bool) (Mode, Origin) {
	if declared && origin == OriginDefault {
		return ModeAuto, OriginProject
	}
	return mode, origin
}

// The stable identifiers the three mode-lowering gates announce themselves by.
//
// They are output, not internal labels, and three things read them: a user
// grepping their own terminal, the gate field of the audit entry, and every
// "no gate intervened" assertion in the test corpus. That last one is why they
// have to stay distinct. Those assertions establish their negative by the
// absence of these three strings rather than by enumerating the preconditions
// that would produce them -- an enumeration attempted twice while the criteria
// were written and incomplete both times, because the configuration-permission
// gate's precondition is filesystem state rather than anything about the
// recipe.
//
// So a later simplification collapsing the three into one generic line is not
// a simplification. It removes that seam, and every assertion resting on it
// goes on passing while measuring nothing.
const (
	gateConfigPermissions  = "config-permissions"
	gateRecipeVerification = "recipe-verification"
	gateMultipleProviders  = "multiple-providers"
)

// GateIdentifiers returns the identifiers above, in the order the gates run.
//
// It is exported for the assertions that establish the negative -- that no
// gate diverted the mode on a given run -- from outside this package, which is
// where the end-to-end demonstrations of the consent modes live. It reads the
// table rather than listing the constants so those assertions cover a gate
// added later without anyone going back to revisit them, which is the property
// AC30 asks for by name.
func GateIdentifiers() []string {
	ids := make([]string, 0, len(modeGates))
	for _, gate := range modeGates {
		ids = append(ids, gate.id)
	}
	return ids
}

// gateSubject is what a gate reads: the command as typed, the recipe that will
// actually be installed, and the candidate list it was selected from.
//
// The list is narrowed by the time a gate sees it where a declaration applies,
// which is what makes the multiple-provider gate's count mean what R3b says it
// means rather than what it counted before.
type gateSubject struct {
	command string
	match   index.BinaryMatch
	matches []index.BinaryMatch
}

// modeGate is one mode-lowering gate: the identifier it announces itself by,
// and the condition under which auto does not survive it.
type modeGate struct {
	id string

	// blocks reports the condition that fires this gate for the subject, or ""
	// where auto survives it.
	//
	// One function rather than a predicate beside a message, because the two
	// would be free to disagree: a gate that fires while naming a condition
	// other than the one that fired it is worse than a gate that says nothing,
	// since a user acts on what it named.
	blocks func(r *Runner, subject gateSubject) string
}

// The table is a package-level var rather than a constant because a test
// registers a fourth gate through it to observe that one entry reaches every
// site. Nothing in production writes it, and it deliberately is not a Runner
// field: these are security controls, and a field lets a wiring site build a
// Runner with no gates at all and no compile error to say so.
//
// modeGates registers the mode-lowering gates. Every site that needs to know
// about a gate reads this table, and in production there are three (the test
// corpus walks it too, for the assertions below): lowerMode, which lowers
// the mode and announces the gate that did it; autoBlockedBy, which decides
// whether the terminal check's message may name --mode=auto; and
// GateIdentifiers, which hands the identifiers to the assertions that
// establish no gate fired.
//
// Registration is the thing the table closes, and it is worth being explicit
// about what it is not. Sharing the *conditions* between the two sites that
// evaluate them -- lowerMode and autoBlockedBy -- was already done: it stops a
// gate and the hatch message drifting apart on what fires them. What it could
// not stop is a fourth gate added to the lowering path and not to the hatch
// computation, which leaves the message naming --mode=auto in a state that
// gate blocks -- a user following it verbatim arrives back at the same message
// having changed nothing, which is the defect R10 is about. A gate needs an
// identifier and a condition regardless, so there was almost nothing left to
// pay for closing that door here.
//
// The order is the order they run in, and both evaluating sites stop at the
// first gate that fires, so the condition either one reports is the one a user
// would hit first. Stopping is not an optimization: a gate that has not
// changed the mode has nothing to announce, because by the time it is reached
// the mode is already confirm.
//
// On the path that prints the hatch message a gate can be answered twice,
// including the recipe load behind RecipeHasVerification -- once by lowerMode
// and once by autoBlockedBy. Only where the mode arrived as auto: a mode that
// was already confirm skips lowerMode entirely. That path is about to end the
// run without installing anything, so the second answer is cheaper than the
// divergence caching it would invite.
var modeGates = []modeGate{
	{
		// The config file gates auto mode, so a file someone else can write
		// is a file that can turn auto on.
		//
		// cfg.ConfigFile rather than a path joined here, because the file this
		// has to guard is the one userconfig.Load actually reads, and that is
		// the field it reads. Two literals that agree today is not the same
		// property: if the config file ever moves, a joined path guards a file
		// nobody consults, while still reporting permissions in a message
		// naming config.toml.
		id: gateConfigPermissions,
		blocks: func(r *Runner, _ gateSubject) string {
			return configPermissionCondition(r.cfg.ConfigFile, os.Getuid())
		},
	},
	{
		// A nil RecipeHasVerification is treated as "unverified": the gate
		// fires rather than being silently skipped.
		id: gateRecipeVerification,
		blocks: func(r *Runner, subject gateSubject) string {
			if r.RecipeHasVerification != nil && r.RecipeHasVerification(subject.match.Recipe) {
				return ""
			}
			return fmt.Sprintf("%s carries no checksum or signature to verify", subject.match.Recipe)
		},
	},
	{
		// For a declared command the list holds one recipe, so this gate
		// cannot fire on a rival provider: the project already said which one
		// it meant, and prompting about a conflict it has settled would be
		// asking a question that has a written answer. It is still a count of
		// the narrowed list rather than an assertion about it -- the index's
		// primary key makes one recipe appear once per command, but matches
		// reaches this package from a caller, and internal/project guards the
		// same parameter for the same reason rather than trusting that.
		id: gateMultipleProviders,
		blocks: func(_ *Runner, subject gateSubject) string {
			if len(subject.matches) <= 1 {
				return ""
			}
			return fmt.Sprintf("more than one recipe provides %q", subject.command)
		},
	},
}

// lowerMode returns the mode that survives the gates, announcing the one that
// fired (R11), and naming that gate to the caller.
//
// Only auto is lowered, so a mode that is not auto is returned untouched and
// no gate is consulted -- which is why "starting from an effective consent
// mode of auto" is in AC21 rather than being a condition the criterion could
// have left out.
//
// The gate identifier is what the audit entry's gate field carries, which is
// R12a. It is returned rather than left inside this function because this is
// the one place that knows which gate fired: a later reader deciding it again
// from a second walk of the table is the divergence the table was built to
// remove. The identifier the record names and the identifier the warning above
// printed are therefore the same string by construction, not by agreement.
func (r *Runner) lowerMode(mode Mode, subject gateSubject) (Mode, string) {
	if mode != ModeAuto {
		return mode, ""
	}
	for _, gate := range modeGates {
		condition := gate.blocks(r, subject)
		if condition == "" {
			continue
		}
		fmt.Fprintf(r.stderr, "Warning: %s: %s; falling back to confirm mode\n", gate.id, condition)
		return ModeConfirm, gate.id
	}
	return mode, ""
}

// terminalAttached reports whether a prompt could be answered. A nil
// IsTerminal is not a terminal -- see the field's own doc for why that is the
// safe direction to default in.
func (r *Runner) terminalAttached() bool {
	return r.IsTerminal != nil && r.IsTerminal()
}

// notInteractiveMessage says why the run stopped, and names every escape hatch
// that works from the state it is printed in and no others (R10).
//
// --mode=auto is the only hatch there is, because it is the only source
// resolveMode honors unconditionally. TSUKU_AUTO_INSTALL_MODE=auto reads like
// its sibling and is deliberately absent: resolveMode ignores an env-supplied
// auto unless config.toml already says auto, so a user following it verbatim
// would arrive back at this message having changed nothing.
//
// The flag is named only where auto would survive the mode-lowering gates.
// Where one of them would put it back at confirm -- a recipe carrying no
// checksum is the ordinary way, and it is why this branch is not a corner
// case -- following it reaches this same message, so the message names the
// gate instead and does not spell the flag at all. Nothing here says
// "--mode=auto" except where --mode=auto works.
func (r *Runner) notInteractiveMessage(subject gateSubject) string {
	const opening = "tsuku: confirm mode requires a terminal"
	if blocked := r.autoBlockedBy(subject); blocked != "" {
		return opening + ", and auto mode is unavailable here: " + blocked
	}
	return opening + "; use --mode=auto for non-interactive use"
}

// autoBlockedBy names the condition under which a mode-lowering gate would put
// a mode of auto back at confirm for this command, or "" where none of them
// would.
//
// It iterates modeGates rather than restating them, which is what makes a
// fourth gate reach this message without anyone remembering to bring it here.
func (r *Runner) autoBlockedBy(subject gateSubject) string {
	for _, gate := range modeGates {
		if condition := gate.blocks(r, subject); condition != "" {
			return condition
		}
	}
	return ""
}

// DeclarationDisclosure is the stable identifier the elevation disclosure
// leads with, for the reason the gate identifiers have one: AC30 asserts this
// line's absence wherever a declaration determined nothing that installs, and
// an assertion against the whole stream cannot tell this line from a gate's.
const DeclarationDisclosure = "project-declaration"

// discloseDeclaration writes the line D5 requires before an install a project
// declaration determined. See the call site for the rule and for why suggest
// is the one dispatch it is skipped for.
//
// The recipe's source is named because the run path inherits the registration
// exposure in tsukumogami/tsuku#2552. That defect can put a recipe in the
// index under a bare name after registering a source the user never approved,
// and a bare declaration then matches it here with no source component
// anywhere in sight -- so a disclosure naming only the authorizing file would
// not say where the thing being installed came from.
// All four facts are stated, including the three that can be absent. A
// declaration carrying no version is ordinary rather than exotic -- `jq = {}`
// in a .tsuku.toml parses to one, and R20 passes it through verbatim -- and
// what the user needs told there is that the version is the installer's
// choice, which dropping the fact does not tell them. The same reasoning
// covers an absent path and an absent source: a line missing a fact still
// looks like a disclosure, so each says it is missing instead.
//
// It goes to stderr, while the confirm prompt it precedes goes to stdout.
// That is the package's existing split rather than a choice made here -- the
// gates announce on stderr, the prompt and the suggest instruction are the
// command's own output -- and it means a user piping stdout still sees the
// disclosure. What it costs is that the two are only interleaved on a
// terminal, which is where the person deciding is.
func (r *Runner) discloseDeclaration(match index.BinaryMatch, version, configPath string) {
	named := match.Recipe
	if version != "" {
		named += "@" + version
	} else {
		named += " at no declared version"
	}
	if configPath == "" {
		configPath = "an unrecorded file"
	}
	fmt.Fprintf(r.stderr, "%s: %s declares %s (recipe source: %s)\n",
		DeclarationDisclosure, configPath, named, recipeSource(match))
}

// recipeSource is the source the binary index recorded for a match: "registry"
// for a recipe the user's configured registries carry, "installed" for one
// that exists only because something installed it locally, which is the half
// of the pair #2552 produces.
//
// An empty source is reported rather than omitted. The field is filled in by
// the index, so empty means a caller built the match by hand, and a
// disclosure that quietly dropped its fourth fact would be a disclosure that
// no longer satisfies the rule while still looking like one.
func recipeSource(match index.BinaryMatch) string {
	if match.Source == "" {
		return "unknown"
	}
	return match.Source
}

// execBinary replaces the current process with the given binary.
func (r *Runner) execBinary(binary string, args []string) error {
	if r.Exec == nil {
		return fmt.Errorf("autoinstall: Exec function not configured")
	}
	execArgs := append([]string{binary}, args...)
	return r.Exec(binary, execArgs, os.Environ())
}

// configPermissionCondition reports why the configuration-permission gate
// fires for the file at path, or "" where it does not: the file has to carry
// no group or other bits at all and be owned by uid, and a file that does not
// exist is fine because there is nothing there to tamper with.
//
// It returns the condition rather than a bool because this gate has five
// distinct ways to fire and AC21 requires the line to say which. A constant
// string would report permissions for a file owned by somebody else, and
// permissions are what a user would then go and change -- so the wrong
// condition here is not a cosmetic failure, it is an instruction that does not
// work, which is the same defect R10 names for the escape hatches.
//
// The owner is a parameter rather than a call to os.Getuid inside, so that the
// branch which cannot be built without a second account -- the one the
// paragraph above is about -- is reachable from a test.
//
// An empty path is checked before the stat and not left to it. os.Stat("")
// fails with ENOENT, so IsNotExist reports true and the first branch below
// would return "no config file, which is fine" -- for a Runner that does not
// know where its config file is, which is not the same statement at all.
// Treating "I cannot tell" as "fine" is how a gate stops being one, and this
// is the direction the rest of the package defaults in: a nil IsTerminal is no
// terminal, a nil RecipeHasVerification is unverified.
func configPermissionCondition(path string, uid int) string {
	if path == "" {
		return "no path is configured for config.toml, so its permissions cannot be checked"
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		return fmt.Sprintf("config.toml cannot be read: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Sprintf("the permissions on config.toml are %#o, which grants access beyond its owner", perm)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "the ownership of config.toml cannot be determined"
	}
	if stat.Uid != uint32(uid) {
		return fmt.Sprintf("config.toml is owned by uid %d rather than by uid %d", stat.Uid, uid)
	}
	return ""
}

// writeAuditLog appends one NDJSON line to $TSUKU_HOME/audit.log.
//
// The action is "install" rather than the "auto-install" it said while this
// was an auto-only line. Now that confirm installs are recorded too, that
// spelling would be false on most of them, and a per-mode action string --
// "confirm-install" beside "auto-install" -- would only put the mode field's
// content in a second place free to disagree with it. The action says what
// happened; the three fields beside it say under what consent.
//
// The origin is written through Origin.String, so an origin no caller resolved
// appears as "unset" rather than as one of the five names R12 lists. That is
// the honest report of a wiring bug rather than a sixth origin: every
// production path into Run resolves one, and the corpus asserts every install
// it drives records one of the five. Substituting a plausible value here would
// make the log agree with R12 by naming a source nobody chose, which is the
// one failure a record has no way to survive.
//
// It stays best effort in both directions -- a marshal that fails and a file
// that cannot be opened both return silently. A run that installed what the
// user asked for should not fail because the log could not be appended to.
func writeAuditLog(homeDir, recipe, version string, mode Mode, origin Origin, loweredBy string) {
	logPath := filepath.Join(homeDir, "audit.log")

	entry := auditEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Action:    "install",
		Recipe:    recipe,
		Version:   version,
		Mode:      mode.String(),
		Origin:    origin.String(),
		Gate:      loweredBy,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return // best effort
	}
	data = append(data, '\n')

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return // best effort
	}
	defer f.Close()
	_, _ = f.Write(data)
}

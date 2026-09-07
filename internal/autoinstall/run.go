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
type auditEntry struct {
	Timestamp string `json:"ts"`
	Action    string `json:"action"`
	Recipe    string `json:"recipe"`
	Version   string `json:"version"`
	Mode      string `json:"mode"`
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

	// The bounded elevation. A declaration raises the mode to auto only where
	// the mode is the unset default, and only for the command it declared --
	// this branch is below the narrowing, so an undeclared command in a
	// project that declares something else has a nil declaration here and is
	// not raised.
	//
	// The bound is the origin rather than the mode, and the mode would not do.
	// The case for raising confirm is that confirm is a default nobody chose,
	// and that reasoning does not survive someone choosing it -- so an
	// explicitly set mode is honored as given, whichever it is. Confirm is
	// also what the escalation restriction substitutes for an environment
	// variable asking for auto that the persistent config did not corroborate,
	// and raising every confirm would take that control's output and put it
	// straight back to auto, which is the reverse of what it is for. An origin
	// of default is the one state in which nobody has said anything.
	//
	// It raises the mode and nothing else. The terminal check below reads the
	// mode rather than the declaration, so a declared command a gate lowers
	// back to confirm meets that check like any other -- what the declaration
	// bypasses is the prompt it consented to, not every question there is.
	effectiveMode := mode
	if declaration != nil && origin == OriginDefault {
		effectiveMode = ModeAuto
	}

	// Security gate 2: config permission check.
	// If the config file has permissive permissions, fall back to confirm
	// to prevent a tampered config from enabling auto mode.
	if effectiveMode == ModeAuto && !r.configPermissionGateOK() {
		fmt.Fprintf(r.stderr, "Warning: config file permissions are too open, falling back to confirm mode\n")
		effectiveMode = ModeConfirm
	}

	// Security gate 3 (auto mode only): verification gate.
	// If the recipe has no checksum or signature verification, fall back to confirm.
	if effectiveMode == ModeAuto && !r.verificationGateOK(match.Recipe) {
		effectiveMode = ModeConfirm
	}

	// Security gate 4 (auto mode only): conflict gate.
	// If multiple recipes provide this command, fall back to confirm.
	//
	// For a declared command the list holds one recipe, so this gate cannot
	// fire on a rival provider: the project already said which one it meant,
	// and prompting about a conflict it has settled would be asking a question
	// that has a written answer. It is still a count of the narrowed list
	// rather than an assertion about it -- the index's primary key makes one
	// recipe appear once per command, but matches reaches this package from a
	// caller, and internal/project guards the same parameter for the same
	// reason rather than trusting that.
	if effectiveMode == ModeAuto && !r.providerGateOK(matches) {
		effectiveMode = ModeConfirm
	}

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
		fmt.Fprintf(r.stderr, "%s\n", r.notInteractiveMessage(command, match, matches))
		return ErrNotInteractive
	}

	// Mode dispatch.
	switch effectiveMode {
	case ModeSuggest:
		// A declared command's instruction is built from the declaration
		// rather than from the match, through the same helper the refusal
		// uses. The configuration key carries the source and the declaration
		// carries the version, and an instruction naming the bare recipe would
		// install something the project did not ask for -- leaving the next
		// `tsuku run` in this directory printing this same line.
		argument := match.Recipe
		if declaration != nil {
			argument = installArgument(*declaration)
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
		// Proceed silently; audit log is written after install.
	}

	// Install.
	if r.Installer == nil {
		return fmt.Errorf("autoinstall: Installer not configured")
	}
	if err := r.Installer.Install(ctx, match.Recipe, version); err != nil {
		return fmt.Errorf("autoinstall: install failed: %w", err)
	}

	// Write audit log for auto-mode installs.
	if effectiveMode == ModeAuto {
		writeAuditLog(r.cfg.HomeDir, match.Recipe, version)
	}

	// Exec the installed binary.
	binaryPath := filepath.Join(r.cfg.CurrentDir, command)
	return r.execBinary(binaryPath, args)
}

// The three mode-lowering gates, each as a predicate reporting whether auto
// survives it.
//
// They are predicates rather than three conditions written inline because two
// callers need the same answer: the gates above, which lower the mode, and the
// terminal check's message, which names --mode=auto only where auto would
// still be auto by the time it got here. A message deciding that from a second
// copy of these conditions would go on printing the hatch the first time a
// gate changed, and what R10 requires is that the message be true, not that it
// once was.
//
// Two callers means each is answered twice on the path that prints the
// message, including the recipe load behind RecipeHasVerification. That is a
// path that is about to end the run without installing anything, so the second
// answer is cheaper than the divergence caching it would invite.

func (r *Runner) configPermissionGateOK() bool {
	return configPermissionsOK(filepath.Join(r.cfg.HomeDir, "config.toml"))
}

// A nil RecipeHasVerification is treated as "unverified": the gate fires
// rather than being silently skipped.
func (r *Runner) verificationGateOK(recipe string) bool {
	return r.RecipeHasVerification != nil && r.RecipeHasVerification(recipe)
}

func (r *Runner) providerGateOK(matches []index.BinaryMatch) bool {
	return len(matches) <= 1
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
func (r *Runner) notInteractiveMessage(command string, match index.BinaryMatch, matches []index.BinaryMatch) string {
	const opening = "tsuku: confirm mode requires a terminal"
	if blocked := r.autoBlockedBy(command, match, matches); blocked != "" {
		return opening + ", and auto mode is unavailable here: " + blocked
	}
	return opening + "; use --mode=auto for non-interactive use"
}

// autoBlockedBy names the mode-lowering gate that would put a mode of auto
// back at confirm for this command, or "" where none of them would.
//
// The order matches the order the gates run in, so the reason named is the one
// a user would hit first.
func (r *Runner) autoBlockedBy(command string, match index.BinaryMatch, matches []index.BinaryMatch) string {
	switch {
	case !r.configPermissionGateOK():
		return "the permissions on config.toml are too open"
	case !r.verificationGateOK(match.Recipe):
		return fmt.Sprintf("%s carries no checksum or signature to verify", match.Recipe)
	case !r.providerGateOK(matches):
		return fmt.Sprintf("more than one recipe provides %q", command)
	}
	return ""
}

// execBinary replaces the current process with the given binary.
func (r *Runner) execBinary(binary string, args []string) error {
	if r.Exec == nil {
		return fmt.Errorf("autoinstall: Exec function not configured")
	}
	execArgs := append([]string{binary}, args...)
	return r.Exec(binary, execArgs, os.Environ())
}

// configPermissionsOK checks that the config file is mode 0600 and owned
// by the current user. Returns true if the file doesn't exist (no config
// to tamper with).
func configPermissionsOK(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return true // no config file is fine
	}
	if err != nil {
		return false // can't stat, be cautious
	}
	// Check permissions: no group/other access.
	mode := info.Mode().Perm()
	if mode&0077 != 0 {
		return false
	}
	// Check ownership: must be owned by the current user.
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false // can't determine ownership, be cautious
	}
	return stat.Uid == uint32(os.Getuid())
}

// writeAuditLog appends one NDJSON line to $TSUKU_HOME/audit.log.
func writeAuditLog(homeDir, recipe, version string) {
	logPath := filepath.Join(homeDir, "audit.log")

	entry := auditEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Action:    "auto-install",
		Recipe:    recipe,
		Version:   version,
		Mode:      "auto",
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

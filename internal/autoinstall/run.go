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
func (r *Runner) Run(ctx context.Context, command string, args []string, mode Mode, resolver ProjectDeclarationResolver) error {
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

		// Declared version not installed -- fall through to install flow
		// with auto mode (project config is consent).
	} else if match.Installed {
		// Nothing declared -- use the globally active version.
		binaryPath := filepath.Join(r.cfg.CurrentDir, command)
		return r.execBinary(binaryPath, args)
	}

	// Project override: when the tool is declared in .tsuku.toml, escalate
	// the mode to auto so the TTY gate and interactive prompt are bypassed.
	effectiveMode := mode
	if declaration != nil {
		effectiveMode = ModeAuto
	}

	// Security gate 2: config permission check.
	// If the config file has permissive permissions, fall back to confirm
	// to prevent a tampered config from enabling auto mode.
	if effectiveMode == ModeAuto {
		configPath := filepath.Join(r.cfg.HomeDir, "config.toml")
		if !configPermissionsOK(configPath) {
			fmt.Fprintf(r.stderr, "Warning: config file permissions are too open, falling back to confirm mode\n")
			effectiveMode = ModeConfirm
		}
	}

	// Security gate 3 (auto mode only): verification gate.
	// If the recipe has no checksum or signature verification, fall back to confirm.
	// A nil RecipeHasVerification function is treated as "unverified" — the gate
	// fires rather than being silently skipped.
	if effectiveMode == ModeAuto {
		hasVerification := r.RecipeHasVerification != nil && r.RecipeHasVerification(match.Recipe)
		if !hasVerification {
			effectiveMode = ModeConfirm
		}
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
	if effectiveMode == ModeAuto && len(matches) > 1 {
		effectiveMode = ModeConfirm
	}

	// Mode dispatch.
	switch effectiveMode {
	case ModeSuggest:
		fmt.Fprintf(r.stdout, "Install with: tsuku install %s\n", match.Recipe)
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

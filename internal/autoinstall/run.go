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
	"time"

	"github.com/tsukumogami/tsuku/internal/index"
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

// Run executes the install-then-exec flow for a command.
//
// It looks up the command in the binary index, applies security gates and
// the consent mode, installs if needed, and hands off execution via
// syscall.Exec (or the injected ExecFunc).
//
// The resolver parameter provides project-pinned versions. Pass nil to
// use the latest version from the registry.
func (r *Runner) Run(ctx context.Context, command string, args []string, mode Mode, resolver ProjectVersionResolver) error {
	// Security gate 1: root guard.
	if os.Geteuid() == 0 {
		return fmt.Errorf("%w: refusing to auto-install as root", ErrForbidden)
	}

	// Look up command in the binary index.
	if r.Lookup == nil {
		return fmt.Errorf("autoinstall: Lookup function not configured")
	}
	matches, err := r.Lookup(ctx, command)
	if err != nil {
		if errors.Is(err, index.ErrIndexNotBuilt) {
			fmt.Fprintf(r.stderr, "Binary index not built. Run 'tsuku update-registry' to build it.\n")
			return ErrIndexNotBuilt
		}
		// StaleIndexWarning: results are still valid.
		var stale index.StaleIndexWarning
		if !errors.As(err, &stale) {
			return fmt.Errorf("autoinstall: lookup failed: %w", err)
		}
		fmt.Fprintf(r.stderr, "Warning: %v\n", err)
	}

	if len(matches) == 0 {
		return ErrNoMatch
	}

	// If the top match is already installed, exec immediately.
	if matches[0].Installed {
		binaryPath := filepath.Join(r.cfg.CurrentDir, command)
		return r.execBinary(binaryPath, args)
	}

	// Pick the best match. If there's only one, use it. If multiple, the
	// conflict gate may apply in auto mode.
	match := matches[0]

	// Resolve version from project config if available.
	version := ""
	if resolver != nil {
		v, ok, resolveErr := resolver.ProjectVersionFor(ctx, command)
		if resolveErr != nil {
			return fmt.Errorf("autoinstall: version resolution failed: %w", resolveErr)
		}
		if ok {
			version = v
		}
	}

	// Security gate 2: config permission check.
	// If the config file has permissive permissions, fall back to confirm
	// to prevent a tampered config from enabling auto mode.
	effectiveMode := mode
	if effectiveMode == ModeAuto {
		configPath := filepath.Join(r.cfg.HomeDir, "config.toml")
		if !configPermissionsOK(configPath) {
			fmt.Fprintf(r.stderr, "Warning: config file permissions are too open, falling back to confirm mode\n")
			effectiveMode = ModeConfirm
		}
	}

	// Security gate 3 (auto mode only): verification gate.
	// If the recipe has no checksum or signature verification, fall back to confirm.
	if effectiveMode == ModeAuto && r.RecipeHasVerification != nil {
		if !r.RecipeHasVerification(match.Recipe) {
			effectiveMode = ModeConfirm
		}
	}

	// Security gate 4 (auto mode only): conflict gate.
	// If multiple recipes provide this command, fall back to confirm.
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
		fmt.Fprint(r.stdout, prompt)

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
	mode := info.Mode().Perm()
	return mode&0077 == 0
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

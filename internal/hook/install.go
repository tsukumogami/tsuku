// Package hook implements tsuku hook install, uninstall, and status commands
// for registering shell hooks in bash, zsh, and fish.
package hook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tsukumogami/tsuku/internal/hooks"
)

// markerComment is the first line of the two-line marker block inserted into rc files.
const markerComment = "# tsuku hook"

// rcFileForShell returns the rc file path for the given shell name.
// The home directory is provided as a parameter to allow test isolation.
func rcFileForShell(shell, homeDir string) (string, error) {
	switch shell {
	case "bash":
		return filepath.Join(homeDir, ".bashrc"), nil
	case "zsh":
		return filepath.Join(homeDir, ".zshrc"), nil
	case "fish":
		return filepath.Join(homeDir, ".config", "fish", "conf.d", "tsuku.fish"), nil
	default:
		return "", fmt.Errorf("unsupported shell: %q", shell)
	}
}

// markerBlock returns the two-line block to insert into bash/zsh rc files.
// Uses ${TSUKU_HOME:-$HOME/.tsuku} so the line works whether or not
// TSUKU_HOME is exported. Matches the fallback pattern in $TSUKU_HOME/env.
func markerBlock(shell string) string {
	return markerComment + "\n" + `. "${TSUKU_HOME:-$HOME/.tsuku}/share/hooks/tsuku.` + shell + `"`
}

// Install writes the hook files to shareHooksDir and registers the hook for
// the given shell. homeDir is the user's home directory (used to locate rc
// files). For bash and zsh, a marker block is appended to the rc file
// atomically; if the marker is already present the file is not modified.
// For fish, the hook file is written directly to
// ~/.config/fish/conf.d/tsuku.fish.
func Install(shell, homeDir, shareHooksDir string) error {
	// Write all hook files to the share/hooks directory.
	if err := hooks.WriteHookFiles(shareHooksDir); err != nil {
		return fmt.Errorf("write hook files: %w", err)
	}

	switch shell {
	case "bash", "zsh":
		return installRCFile(shell, homeDir)
	case "fish":
		return installFish(homeDir, shareHooksDir)
	default:
		return fmt.Errorf("unsupported shell: %q", shell)
	}
}

// installRCFile appends the marker block to ~/.bashrc or ~/.zshrc if it is
// not already present. The write is atomic: content is written to a temp file
// in the same directory, then renamed into place.
func installRCFile(shell, homeDir string) error {
	rcFile, err := rcFileForShell(shell, homeDir)
	if err != nil {
		return err
	}

	// Read existing content, if any.
	existing, err := os.ReadFile(rcFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", rcFile, err)
	}

	// Idempotency: skip if the marker is already present.
	if strings.Contains(string(existing), markerComment) {
		return nil
	}

	// Append the marker block. Ensure the block starts on its own line.
	content := string(existing)
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += markerBlock(shell) + "\n"

	return atomicWrite(rcFile, []byte(content), 0644)
}

// installFish writes the hook file content directly to
// ~/.config/fish/conf.d/tsuku.fish. No marker block is needed because
// conf.d files are purpose-built extension points, not user-edited files.
func installFish(homeDir, shareHooksDir string) error {
	fishConfd := filepath.Join(homeDir, ".config", "fish", "conf.d")
	if err := os.MkdirAll(fishConfd, 0755); err != nil {
		return fmt.Errorf("create fish conf.d directory: %w", err)
	}

	hookFile := filepath.Join(shareHooksDir, "tsuku.fish")
	data, err := os.ReadFile(hookFile)
	if err != nil {
		return fmt.Errorf("read fish hook file: %w", err)
	}

	dest := filepath.Join(fishConfd, "tsuku.fish")
	return atomicWrite(dest, data, 0644)
}

// atomicWrite writes data to path atomically by writing to a temp file in the
// same directory and then renaming it into place. This prevents rc file
// corruption on interrupted writes.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create parent directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".tsuku-hook-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()

	// Clean up temp file on failure.
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpName, path, err)
	}
	success = true
	return nil
}

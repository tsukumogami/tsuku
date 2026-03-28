package hook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Uninstall removes the tsuku hook for the given shell. homeDir is the user's
// home directory. For bash and zsh, the two-line marker block is removed from
// the rc file. For fish, ~/.config/fish/conf.d/tsuku.fish is deleted.
// Uninstall is idempotent: if the hook is not installed, it returns nil.
func Uninstall(shell, homeDir string) error {
	switch shell {
	case "bash", "zsh":
		return uninstallRCFile(shell, homeDir)
	case "fish":
		return uninstallFish(homeDir)
	default:
		return fmt.Errorf("unsupported shell: %q", shell)
	}
}

// UninstallActivate removes the activation hook for the given shell.
// For bash and zsh, the activate marker block is removed from the rc file.
// For fish, ~/.config/fish/conf.d/tsuku-activate.fish is deleted.
// Idempotent: if the hook is not installed, it returns nil.
func UninstallActivate(shell, homeDir string) error {
	switch shell {
	case "bash", "zsh":
		return uninstallActivateRCFile(shell, homeDir)
	case "fish":
		return uninstallActivateFish(homeDir)
	default:
		return fmt.Errorf("unsupported shell: %q", shell)
	}
}

// uninstallRCFile removes the two-line marker block from ~/.bashrc or ~/.zshrc.
// The write is atomic.
func uninstallRCFile(shell, homeDir string) error {
	rcFile, err := rcFileForShell(shell, homeDir)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(rcFile)
	if os.IsNotExist(err) {
		return nil // Nothing to remove.
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", rcFile, err)
	}

	content := string(data)
	if !strings.Contains(content, markerComment) {
		return nil // Marker not present; nothing to do.
	}

	updated := removeMarkerBlock(content)
	return atomicWrite(rcFile, []byte(updated), 0644)
}

// removeMarkerBlock removes the two-line command-not-found marker block from content.
func removeMarkerBlock(content string) string {
	return removeMarkerBlockByComment(content, markerComment)
}

// removeMarkerBlockByComment removes a two-line marker block identified by
// the given comment string from content. It handles both Unix (\n) and
// Windows (\r\n) line endings.
func removeMarkerBlockByComment(content, comment string) string {
	lines := strings.Split(content, "\n")
	var result []string
	skipNext := false
	for i, line := range lines {
		if skipNext {
			skipNext = false
			continue
		}
		trimmed := strings.TrimRight(line, "\r")
		if trimmed == comment {
			// Also skip the following source line.
			if i+1 < len(lines) {
				skipNext = true
			}
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// uninstallActivateRCFile removes the activate marker block from the rc file.
func uninstallActivateRCFile(shell, homeDir string) error {
	rcFile, err := rcFileForShell(shell, homeDir)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(rcFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", rcFile, err)
	}

	content := string(data)
	if !strings.Contains(content, activateMarkerComment) {
		return nil
	}

	updated := removeMarkerBlockByComment(content, activateMarkerComment)
	return atomicWrite(rcFile, []byte(updated), 0644)
}

// uninstallFish removes ~/.config/fish/conf.d/tsuku.fish if it exists.
func uninstallFish(homeDir string) error {
	dest := filepath.Join(homeDir, ".config", "fish", "conf.d", "tsuku.fish")
	err := os.Remove(dest)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// uninstallActivateFish removes ~/.config/fish/conf.d/tsuku-activate.fish if it exists.
func uninstallActivateFish(homeDir string) error {
	dest := filepath.Join(homeDir, ".config", "fish", "conf.d", "tsuku-activate.fish")
	err := os.Remove(dest)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

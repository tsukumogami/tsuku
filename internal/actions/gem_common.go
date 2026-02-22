package actions

import (
	"fmt"
	"os"
	"path/filepath"
)

// gemWrapperTemplate is the bash wrapper script template for gem executables.
// It sets GEM_HOME/GEM_PATH for isolation and adds Ruby to PATH.
// Arguments: executable name, gem home relative path, ruby bin dir, executable name (for .gem suffix).
const gemWrapperTemplate = `#!/bin/bash
# tsuku wrapper for %s (sets GEM_HOME/GEM_PATH for isolated gem)
SCRIPT_PATH="${BASH_SOURCE[0]}"
# Resolve symlinks to get the actual script location
while [ -L "$SCRIPT_PATH" ]; do
    SCRIPT_DIR="$(cd -P "$(dirname "$SCRIPT_PATH")" && pwd)"
    SCRIPT_PATH="$(readlink "$SCRIPT_PATH")"
    [[ $SCRIPT_PATH != /* ]] && SCRIPT_PATH="$SCRIPT_DIR/$SCRIPT_PATH"
done
SCRIPT_DIR="$(cd -P "$(dirname "$SCRIPT_PATH")" && pwd)"
INSTALL_DIR="$(dirname "$SCRIPT_DIR")"
export GEM_HOME="$INSTALL_DIR/%s"
export GEM_PATH="$GEM_HOME"
# Add Ruby to PATH and explicitly call ruby (don't rely on shebang)
export PATH="%s:$PATH"
exec ruby "$SCRIPT_DIR/%s.gem" "$@"
`

// createGemWrapper creates a self-contained wrapper script for a gem executable.
// It copies the original script to <dstDir>/<exeName>.gem and creates a wrapper
// at <dstDir>/<exeName> that sets GEM_HOME/GEM_PATH/PATH before calling the original.
//
// gemHomeRel is the relative path from the install directory to the GEM_HOME.
// For gem_install direct: "." (gems at install root)
// For gem_exec bundler: "ruby/<ver>" (gems in versioned subdirectory)
//
// When srcScript is in dstDir, the original is renamed in place. When it's in a
// different directory (e.g., bundler's deep ruby/<ver>/bin/ path), the content is
// copied across directories.
func createGemWrapper(srcScript, dstDir, exeName, rubyBinDir, gemHomeRel string) error {
	gemPath := filepath.Join(dstDir, exeName+".gem")
	wrapperPath := filepath.Join(dstDir, exeName)

	sameDir := filepath.Dir(srcScript) == dstDir

	// Move or copy the original script to .gem suffix.
	// When same directory, rename moves the source out of the way (freeing wrapperPath).
	// When cross-directory, copy content and clean up any existing file at wrapperPath.
	if sameDir {
		if err := os.Rename(srcScript, gemPath); err != nil {
			return fmt.Errorf("failed to rename gem script: %w", err)
		}
	} else {
		content, err := os.ReadFile(srcScript)
		if err != nil {
			return fmt.Errorf("failed to read gem script %s: %w", srcScript, err)
		}
		if err := os.WriteFile(gemPath, content, 0755); err != nil {
			return fmt.Errorf("failed to write gem script: %w", err)
		}
		// Remove existing wrapper/symlink at target path
		os.Remove(wrapperPath)
	}

	// Write wrapper script
	wrapperContent := fmt.Sprintf(gemWrapperTemplate, exeName, gemHomeRel, rubyBinDir, exeName)
	if err := os.WriteFile(wrapperPath, []byte(wrapperContent), 0755); err != nil {
		// Restore original on failure
		if sameDir {
			_ = os.Rename(gemPath, srcScript)
		} else {
			_ = os.Remove(gemPath)
		}
		return fmt.Errorf("failed to create wrapper script for %s: %w", exeName, err)
	}

	return nil
}

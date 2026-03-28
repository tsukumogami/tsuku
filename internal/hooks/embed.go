// Package hooks provides shell hook files for bash, zsh, and fish that
// intercept command-not-found events and call tsuku run (which handles both
// project-aware auto-install and suggest fallback), as well as activation
// hooks that call tsuku hook-env on each prompt.
package hooks

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed tsuku.bash tsuku.zsh tsuku.fish tsuku-activate.bash tsuku-activate.zsh tsuku-activate.fish
var HookFiles embed.FS

// WriteHookFiles writes all three hook files from the embedded FS to the
// given directory with 0644 permissions. The directory must already exist.
func WriteHookFiles(shareHooksDir string) error {
	entries, err := fs.ReadDir(HookFiles, ".")
	if err != nil {
		return fmt.Errorf("read embedded hook files: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := fs.ReadFile(HookFiles, entry.Name())
		if err != nil {
			return fmt.Errorf("read embedded hook file %s: %w", entry.Name(), err)
		}
		dest := filepath.Join(shareHooksDir, entry.Name())
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return fmt.Errorf("write hook file %s: %w", entry.Name(), err)
		}
	}
	return nil
}

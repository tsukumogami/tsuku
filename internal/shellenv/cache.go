package shellenv

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RebuildShellCache reads all *.{shell} files from $TSUKU_HOME/share/shell.d/,
// concatenates them sorted alphabetically, and atomically writes
// .init-cache.{shell} to the same directory.
//
// Security hardening:
//   - Symlinks are rejected (only regular files are included)
//   - Files with a known content hash are verified; mismatches are excluded
//   - Files without a stored hash (legacy installs) are included without verification
//   - A file lock prevents concurrent cache rebuilds
//   - Cache files are written with restrictive permissions (0600)
//
// selection is optional. Omit it and nothing is excluded and no hash is
// verified, which is the behavior every caller had before version-keyed
// filenames landed. Pass one and a file recorded for an installed-but-inactive
// version is left out, so only the active version's fragment reaches the cache.
//
// If no matching files exist, any existing cache file is removed.
func RebuildShellCache(tsukuHome string, shell string, selection ...ShellDSelection) error {
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")

	// Ensure directory exists before acquiring lock
	if err := os.MkdirAll(shellDDir, 0700); err != nil {
		return fmt.Errorf("creating shell.d directory: %w", err)
	}

	// Acquire exclusive file lock to prevent concurrent cache rebuilds
	lockPath := filepath.Join(shellDDir, ".lock")
	unlock, err := acquireFileLock(lockPath)
	if err != nil {
		return fmt.Errorf("acquiring shell cache lock: %w", err)
	}
	defer unlock()

	sel := firstSelection(selection)

	// Read directory entries
	entries, err := os.ReadDir(shellDDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No shell.d directory means nothing to cache
			return nil
		}
		return fmt.Errorf("reading shell.d directory: %w", err)
	}

	// Collect matching files (*.{shell}), excluding the cache file and lock file
	suffix := "." + shell
	cacheFileName := ".init-cache" + suffix

	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if name == cacheFileName || name == ".lock" {
			continue
		}
		if strings.HasSuffix(name, suffix) {
			// A file state records for a version of a tool that is installed but
			// not active belongs to that other version; sourcing it would put the
			// wrong version's exports in the user's shell.
			if sel.excludes(shellDRelPath(name)) {
				continue
			}

			filePath := filepath.Join(shellDDir, name)

			// Symlink rejection: use Lstat to check the actual entry type
			info, err := os.Lstat(filePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: cannot stat %s, skipping: %v\n", name, err)
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 {
				fmt.Fprintf(os.Stderr, "Warning: %s is a symlink, excluding from shell cache\n", name)
				continue
			}
			if !info.Mode().IsRegular() {
				fmt.Fprintf(os.Stderr, "Warning: %s is not a regular file, excluding from shell cache\n", name)
				continue
			}

			files = append(files, name)
		}
	}

	cachePath := filepath.Join(shellDDir, cacheFileName)

	if len(files) == 0 {
		// No source files; remove stale cache if present
		os.Remove(cachePath)
		return nil
	}

	// Sort alphabetically. This is not only for reproducible output: the order
	// is a contract. Files are sourced in this order, and a tool's init script
	// often reads variables its recipe exported -- nvm.sh, for one, only honors
	// NVM_DIR when it is already set. The set_env action relies on that by
	// naming its files "00-env-<tool>@<version>.<shell>" so they sort ahead of
	// the tool's own entry. An init target must begin with a letter or an
	// underscore, so nothing legal sorts before "0". Changing this ordering
	// breaks those recipes.
	sort.Strings(files)

	// Concatenate file contents with hash verification and error isolation.
	// Each tool's content is wrapped in a brace group so runtime errors in one
	// tool's init script do not prevent others from loading, while still allowing
	// function definitions and variable exports to affect the current shell.
	var buf strings.Builder
	for _, name := range files {
		filePath := filepath.Join(shellDDir, name)
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}

		// Hash verification: if we have a stored hash for this file, verify it.
		// If no hash is stored for this file (legacy install), include it.
		if expectedHash := sel.hashFor(shellDRelPath(name)); expectedHash != "" {
			actualHash := sha256Hex(content)
			if actualHash != expectedHash {
				fmt.Fprintf(os.Stderr, "Warning: %s content hash mismatch (expected %s, got %s), excluding from shell cache\n",
					name, expectedHash[:12]+"...", actualHash[:12]+"...")
				continue
			}
		}

		// Derive the display name from the filename, dropping the shell suffix
		// and the version key ("nvm@0.40.6.bash" -> "nvm").
		toolName := DisplayName(name, shell)

		// Wrap in a brace group (not a subshell) so function definitions and
		// variable assignments propagate to the current shell. Stderr is
		// suppressed and non-zero exits are swallowed so one tool's runtime
		// failure does not prevent other tools' initialization.
		buf.WriteString("# tsuku: " + toolName + "\n")
		buf.WriteString("{ # begin " + toolName + "\n")
		contentStr := string(content)
		buf.WriteString(contentStr)
		// Ensure content ends with a newline before the closing brace
		if len(contentStr) > 0 && contentStr[len(contentStr)-1] != '\n' {
			buf.WriteByte('\n')
		}
		buf.WriteString("} 2>/dev/null || true\n")
	}

	// If all files were excluded by hash verification, remove the cache
	if buf.Len() == 0 {
		os.Remove(cachePath)
		return nil
	}

	// Atomic write: write to temp file, then rename
	tmpPath := cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(buf.String()), 0600); err != nil {
		return fmt.Errorf("writing temp cache: %w", err)
	}

	if err := os.Rename(tmpPath, cachePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming cache file: %w", err)
	}

	return nil
}

// sha256Hex computes the SHA-256 hex digest of the given data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

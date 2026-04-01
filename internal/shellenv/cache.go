package shellenv

import (
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
// If no matching files exist, any existing cache file is removed.
func RebuildShellCache(tsukuHome string, shell string) error {
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")

	// Read directory entries
	entries, err := os.ReadDir(shellDDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No shell.d directory means nothing to cache
			return nil
		}
		return fmt.Errorf("reading shell.d directory: %w", err)
	}

	// Collect matching files (*.{shell}), excluding the cache file itself
	suffix := "." + shell
	cacheFileName := ".init-cache" + suffix

	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		if name == cacheFileName {
			continue
		}
		if strings.HasSuffix(name, suffix) {
			files = append(files, name)
		}
	}

	cachePath := filepath.Join(shellDDir, cacheFileName)

	if len(files) == 0 {
		// No source files; remove stale cache if present
		os.Remove(cachePath)
		return nil
	}

	// Sort alphabetically for deterministic output
	sort.Strings(files)

	// Concatenate file contents
	var buf strings.Builder
	for _, name := range files {
		content, err := os.ReadFile(filepath.Join(shellDDir, name))
		if err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}
		buf.Write(content)
		// Ensure each file's content ends with a newline
		if len(content) > 0 && content[len(content)-1] != '\n' {
			buf.WriteByte('\n')
		}
	}

	// Atomic write: write to temp file, then rename
	tmpPath := cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(buf.String()), 0644); err != nil {
		return fmt.Errorf("writing temp cache: %w", err)
	}

	if err := os.Rename(tmpPath, cachePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming cache file: %w", err)
	}

	return nil
}

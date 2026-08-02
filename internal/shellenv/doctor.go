package shellenv

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ShellDCheckResult holds the results of a shell.d health check.
type ShellDCheckResult struct {
	// ActiveScripts lists tool names that have shell.d files, keyed by shell.
	// e.g., {"bash": ["starship", "zoxide"], "zsh": ["starship"]}
	ActiveScripts map[string][]string

	// CacheStale is true if the cache doesn't match the current concatenation
	// of shell.d files for any shell.
	CacheStale map[string]bool

	// HashMismatches lists files where the on-disk content doesn't match the
	// stored content hash. Key is the relative path, value describes the issue.
	HashMismatches []string

	// Symlinks lists shell.d entries that are symlinks (a security concern).
	Symlinks []string

	// SyntaxErrors lists files that fail shell syntax checking.
	SyntaxErrors []ShellSyntaxError
}

// ShellSyntaxError pairs a filename with its syntax error message.
type ShellSyntaxError struct {
	File    string
	Message string
}

// HasIssues returns true if any check found problems.
func (r *ShellDCheckResult) HasIssues() bool {
	for _, stale := range r.CacheStale {
		if stale {
			return true
		}
	}
	return len(r.HashMismatches) > 0 || len(r.Symlinks) > 0 || len(r.SyntaxErrors) > 0
}

// CheckShellD runs health checks on the shell.d directory under tsukuHome.
//
// selection is optional and carries the same meaning it has in
// RebuildShellCache: omit it and no file is excluded and no hash is verified.
// The two must agree on what belongs in the cache, or doctor reports a stale
// cache that --fix rebuilds into the same state it just called stale.
func CheckShellD(tsukuHome string, selection ...ShellDSelection) *ShellDCheckResult {
	result := &ShellDCheckResult{
		ActiveScripts: make(map[string][]string),
		CacheStale:    make(map[string]bool),
	}

	sel := firstSelection(selection)

	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")

	entries, err := os.ReadDir(shellDDir)
	if err != nil {
		// No shell.d directory -- nothing to check
		return result
	}

	// Classify files by shell type
	shellFiles := make(map[string][]string) // shell -> sorted filenames
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}

		// A fragment recorded for an installed-but-inactive version is not part
		// of the cache, so it is neither an active script nor a hash to check.
		if sel.excludes(shellDRelPath(name)) {
			continue
		}

		// Detect symlinks
		filePath := filepath.Join(shellDDir, name)
		info, err := os.Lstat(filePath)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			result.Symlinks = append(result.Symlinks, name)
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}

		// Categorize by extension
		for _, shell := range []string{"bash", "zsh"} {
			suffix := "." + shell
			if strings.HasSuffix(name, suffix) {
				result.ActiveScripts[shell] = append(result.ActiveScripts[shell], DisplayName(name, shell))
				shellFiles[shell] = append(shellFiles[shell], name)
			}
		}

		// Hash verification
		if expectedHash := sel.hashFor(shellDRelPath(name)); expectedHash != "" {
			content, err := os.ReadFile(filePath)
			if err == nil {
				actualHash := sha256Hex(content)
				if actualHash != expectedHash {
					result.HashMismatches = append(result.HashMismatches, name)
				}
			}
		}

		// Syntax check
		if strings.HasSuffix(name, ".bash") {
			if err := checkShellSyntax(filePath, "bash"); err != nil {
				result.SyntaxErrors = append(result.SyntaxErrors, ShellSyntaxError{
					File:    name,
					Message: err.Error(),
				})
			}
		} else if strings.HasSuffix(name, ".zsh") {
			if err := checkShellSyntax(filePath, "zsh"); err != nil {
				result.SyntaxErrors = append(result.SyntaxErrors, ShellSyntaxError{
					File:    name,
					Message: err.Error(),
				})
			}
		}
	}

	// Sort active scripts for deterministic output
	for shell := range result.ActiveScripts {
		sort.Strings(result.ActiveScripts[shell])
	}

	// Check cache freshness for each shell
	for shell, files := range shellFiles {
		sort.Strings(files)
		result.CacheStale[shell] = isCacheStale(shellDDir, shell, files, sel)
	}

	return result
}

// isCacheStale checks whether the cache file matches the expected content
// from concatenating the given files (with error-isolation wrapping).
//
// It has to reproduce RebuildShellCache exactly, including the files that
// builder drops. Skipping a hash-mismatched file here is what makes
// `doctor --fix` converge: before, the rebuild excluded the file and the
// re-check expected it back, so the cache was reported stale forever.
func isCacheStale(shellDDir, shell string, files []string, sel ShellDSelection) bool {
	cachePath := filepath.Join(shellDDir, ".init-cache."+shell)
	cacheContent, err := os.ReadFile(cachePath)

	// Rebuild what the cache should look like
	var buf strings.Builder
	included := 0
	for _, name := range files {
		content, readErr := os.ReadFile(filepath.Join(shellDDir, name))
		if readErr != nil {
			return true // Can't read a source file -- consider stale
		}
		if expectedHash := sel.hashFor(shellDRelPath(name)); expectedHash != "" {
			if sha256Hex(content) != expectedHash {
				continue
			}
		}
		included++
		toolName := DisplayName(name, shell)
		buf.WriteString("# tsuku: " + toolName + "\n")
		buf.WriteString("{ # begin " + toolName + "\n")
		contentStr := string(content)
		buf.WriteString(contentStr)
		if len(contentStr) > 0 && contentStr[len(contentStr)-1] != '\n' {
			buf.WriteByte('\n')
		}
		buf.WriteString("} 2>/dev/null || true\n")
	}

	if err != nil {
		// The builder removes the cache when nothing is left to include, so a
		// missing cache is correct in that case rather than stale.
		return included > 0
	}

	return string(cacheContent) != buf.String()
}

// checkShellSyntax runs `<shell> -n <file>` to check for syntax errors.
// Returns nil if the syntax is valid or the shell isn't available.
func checkShellSyntax(filePath, shell string) error {
	shellPath, err := exec.LookPath(shell)
	if err != nil {
		// Shell not available -- skip check
		return nil
	}

	cmd := exec.Command(shellPath, "-n", filePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = fmt.Sprintf("%s -n exited with: %v", shell, err)
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

package actions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstallLibrariesAction implements library file installation with symlink preservation
type InstallLibrariesAction struct{}

// Name returns the action name
func (a *InstallLibrariesAction) Name() string {
	return "install_libraries"
}

// Execute copies library files matching glob patterns to the installation directory
//
// Parameters:
//   - patterns (required): List of glob patterns to match library files
//     e.g., ["lib/*.so*", "lib/*.dylib"]
//
// The action preserves symlinks (copies as symlinks, not dereferenced).
// This is critical for library versioning (e.g., libyaml.so.2 -> libyaml.so.2.0.9).
func (a *InstallLibrariesAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get patterns list (required)
	patternsRaw, ok := params["patterns"]
	if !ok {
		return fmt.Errorf("install_libraries action requires 'patterns' parameter")
	}

	// Parse patterns list
	patterns, err := a.parsePatterns(patternsRaw)
	if err != nil {
		return fmt.Errorf("failed to parse patterns: %w", err)
	}

	if len(patterns) == 0 {
		return fmt.Errorf("patterns list cannot be empty")
	}

	// Validate patterns for security
	for _, pattern := range patterns {
		if err := a.validatePattern(pattern); err != nil {
			return err
		}
	}

	// Collect all matching files
	var matches []string
	for _, pattern := range patterns {
		fullPattern := filepath.Join(ctx.WorkDir, pattern)
		m, err := filepath.Glob(fullPattern)
		if err != nil {
			return fmt.Errorf("invalid glob pattern '%s': %w", pattern, err)
		}
		matches = append(matches, m...)
	}

	if len(matches) == 0 {
		return fmt.Errorf("no files matched patterns: %v", patterns)
	}

	// Copy each matched file, preserving symlinks
	fmt.Printf("   Installing %d library file(s)\n", len(matches))

	for _, srcPath := range matches {
		// Calculate relative path from WorkDir
		relPath, err := filepath.Rel(ctx.WorkDir, srcPath)
		if err != nil {
			return fmt.Errorf("failed to compute relative path for %s: %w", srcPath, err)
		}

		// Destination path in InstallDir
		destPath := filepath.Join(ctx.InstallDir, relPath)

		// Ensure destination directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", relPath, err)
		}

		// Check if source is a symlink (use Lstat to not follow the link)
		info, err := os.Lstat(srcPath)
		if err != nil {
			return fmt.Errorf("failed to stat %s: %w", srcPath, err)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			// Copy as symlink
			if err := CopySymlink(srcPath, destPath); err != nil {
				return fmt.Errorf("failed to copy symlink %s: %w", relPath, err)
			}
			fmt.Printf("   ✓ Installed symlink: %s\n", relPath)
		} else {
			// Copy as regular file
			if err := CopyFile(srcPath, destPath, info.Mode()); err != nil {
				return fmt.Errorf("failed to copy file %s: %w", relPath, err)
			}
			fmt.Printf("   ✓ Installed: %s\n", relPath)
		}
	}

	return nil
}

// parsePatterns parses the patterns parameter from TOML
func (a *InstallLibrariesAction) parsePatterns(raw interface{}) ([]string, error) {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("patterns must be an array")
	}

	var result []string
	for i, item := range arr {
		pattern, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("pattern %d: must be a string", i)
		}
		result = append(result, pattern)
	}

	return result, nil
}

// validatePattern validates that a pattern doesn't contain directory traversal
func (a *InstallLibrariesAction) validatePattern(pattern string) error {
	// Check for directory traversal patterns
	if strings.Contains(pattern, "..") {
		return fmt.Errorf("pattern cannot contain '..': %s", pattern)
	}

	// Check for absolute paths
	if filepath.IsAbs(pattern) {
		return fmt.Errorf("pattern must be relative, not absolute: %s", pattern)
	}

	return nil
}

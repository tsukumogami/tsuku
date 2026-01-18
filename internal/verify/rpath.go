package verify

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Security limits for RPATH processing
const (
	// MaxRpathEntries is the maximum number of RPATH entries allowed per binary
	MaxRpathEntries = 100

	// MaxPathLength is the maximum length of any path (matches Linux PATH_MAX)
	MaxPathLength = 4096
)

// LC_RPATH is the Mach-O load command for runtime search paths.
// This constant is not exported by Go's standard library.
const lcRpath macho.LoadCmd = 0x8000001c

// ExtractRpaths extracts RPATH entries from an ELF or Mach-O binary.
// For ELF, it uses DT_RUNPATH (preferred) with DT_RPATH fallback.
// For Mach-O, it parses LC_RPATH load commands.
// Returns an empty slice if the binary has no RPATH entries.
func ExtractRpaths(path string) (rpaths []string, err error) {
	// Panic recovery for robustness against malformed input
	defer func() {
		if r := recover(); r != nil {
			err = &ValidationError{
				Category: ErrCorrupted,
				Path:     path,
				Message:  fmt.Sprintf("parser panic: %v", r),
			}
		}
	}()

	// Read magic bytes for format detection
	magic, err := readMagicForRpath(path)
	if err != nil {
		return nil, fmt.Errorf("read magic: %w", err)
	}

	format := detectFormatForRpath(magic)
	switch format {
	case "elf":
		return extractELFRpaths(path)
	case "macho":
		return extractMachORpaths(path)
	case "fat":
		return extractFatRpaths(path)
	default:
		// Non-binary files have no RPATH - return empty (not an error)
		return nil, nil
	}
}

// extractELFRpaths extracts RPATH entries from an ELF binary.
// Prefers DT_RUNPATH over DT_RPATH (per modern ELF semantics).
func extractELFRpaths(path string) ([]string, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open ELF: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Try DT_RUNPATH first (preferred, takes precedence in modern linkers)
	runpaths, err := f.DynString(elf.DT_RUNPATH)
	if err == nil && len(runpaths) > 0 {
		return parseRpathString(runpaths[0], path)
	}

	// Fall back to DT_RPATH
	rpaths, err := f.DynString(elf.DT_RPATH)
	if err == nil && len(rpaths) > 0 {
		return parseRpathString(rpaths[0], path)
	}

	// No RPATH/RUNPATH is normal - return empty slice
	return nil, nil
}

// parseRpathString parses a colon-separated RPATH string into individual paths.
// Enforces the RPATH limit and path length limits.
func parseRpathString(rpathStr string, binaryPath string) ([]string, error) {
	if rpathStr == "" {
		return nil, nil
	}

	parts := strings.Split(rpathStr, ":")
	if len(parts) > MaxRpathEntries {
		return nil, &ValidationError{
			Category: ErrRpathLimitExceeded,
			Path:     binaryPath,
			Message:  fmt.Sprintf("binary has %d RPATH entries (limit: %d)", len(parts), MaxRpathEntries),
		}
	}

	var rpaths []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len(p) > MaxPathLength {
			return nil, &ValidationError{
				Category: ErrPathLengthExceeded,
				Path:     binaryPath,
				Message:  fmt.Sprintf("RPATH entry exceeds %d characters", MaxPathLength),
			}
		}
		rpaths = append(rpaths, p)
	}
	return rpaths, nil
}

// extractMachORpaths extracts RPATH entries from a Mach-O binary.
func extractMachORpaths(path string) ([]string, error) {
	f, err := macho.Open(path)
	if err != nil {
		// Check if this is a fat binary
		if isFatBinaryForRpath(path) {
			return extractFatRpaths(path)
		}
		return nil, fmt.Errorf("open Mach-O: %w", err)
	}
	defer func() { _ = f.Close() }()

	return extractMachORpathsFromFile(f, path)
}

// extractMachORpathsFromFile extracts RPATH entries from an open macho.File.
func extractMachORpathsFromFile(f *macho.File, binaryPath string) ([]string, error) {
	var rpaths []string

	for _, load := range f.Loads {
		raw := load.Raw()
		if len(raw) < 8 {
			continue
		}

		// Parse the load command header (cmd uint32, cmdsize uint32)
		cmd := f.ByteOrder.Uint32(raw[0:4])
		if macho.LoadCmd(cmd) != lcRpath {
			continue
		}

		// LC_RPATH structure: cmd(4) + cmdsize(4) + path_offset(4)
		if len(raw) < 12 {
			continue
		}
		pathOffset := f.ByteOrder.Uint32(raw[8:12])
		if int(pathOffset) >= len(raw) {
			continue
		}

		// Path is a null-terminated string starting at pathOffset
		pathBytes := raw[pathOffset:]
		if idx := bytes.IndexByte(pathBytes, 0); idx >= 0 {
			pathBytes = pathBytes[:idx]
		}
		rpathEntry := string(pathBytes)

		if len(rpathEntry) > MaxPathLength {
			return nil, &ValidationError{
				Category: ErrPathLengthExceeded,
				Path:     binaryPath,
				Message:  fmt.Sprintf("RPATH entry exceeds %d characters", MaxPathLength),
			}
		}

		rpaths = append(rpaths, rpathEntry)

		if len(rpaths) > MaxRpathEntries {
			return nil, &ValidationError{
				Category: ErrRpathLimitExceeded,
				Path:     binaryPath,
				Message:  fmt.Sprintf("binary has more than %d RPATH entries", MaxRpathEntries),
			}
		}
	}

	return rpaths, nil
}

// extractFatRpaths extracts RPATH entries from a fat/universal binary.
func extractFatRpaths(path string) ([]string, error) {
	ff, err := macho.OpenFat(path)
	if err != nil {
		return nil, fmt.Errorf("open fat binary: %w", err)
	}
	defer func() { _ = ff.Close() }()

	// RPATH should be the same across all architectures,
	// so just extract from the first slice
	if len(ff.Arches) > 0 {
		return extractMachORpathsFromFile(ff.Arches[0].File, path)
	}
	return nil, nil
}

// isFatBinaryForRpath checks if a file is a fat/universal binary.
func isFatBinaryForRpath(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	magic := make([]byte, 4)
	_, err = f.Read(magic)
	if err != nil {
		return false
	}

	return bytes.Equal(magic, fatMagic)
}

// readMagicForRpath reads the first 8 bytes of a file for format detection.
func readMagicForRpath(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	magic := make([]byte, 8)
	n, err := f.Read(magic)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return magic[:n], nil
}

// detectFormatForRpath determines the binary format from magic bytes.
func detectFormatForRpath(magic []byte) string {
	if len(magic) < 4 {
		return ""
	}

	switch {
	case bytes.HasPrefix(magic, elfMagic):
		return "elf"
	case bytes.Equal(magic[:4], machO32) || bytes.Equal(magic[:4], machO32Rev) ||
		bytes.Equal(magic[:4], machO64) || bytes.Equal(magic[:4], machO64Rev):
		return "macho"
	case bytes.Equal(magic[:4], fatMagic):
		return "fat"
	default:
		return ""
	}
}

// ExpandPathVariables expands runtime path variables in a dependency path.
// Supports:
//   - $ORIGIN, ${ORIGIN}: directory containing the binary (ELF)
//   - @rpath: tries each RPATH entry in order, returns first match
//   - @loader_path: directory containing the binary (Mach-O)
//   - @executable_path: directory containing the main executable (Mach-O)
//
// The allowedPrefix parameter specifies the canonical path prefix that expanded
// paths must resolve to (e.g., "$TSUKU_HOME/tools/"). Pass empty string to skip
// this validation.
//
// Returns the expanded path, or an error if:
//   - The path contains unexpanded variables after expansion
//   - The expanded path is outside the allowed directory
//   - The path exceeds length limits
func ExpandPathVariables(dep, binaryPath string, rpaths []string, allowedPrefix string) (string, error) {
	if len(dep) > MaxPathLength {
		return "", &ValidationError{
			Category: ErrPathLengthExceeded,
			Path:     dep,
			Message:  fmt.Sprintf("dependency path exceeds %d characters", MaxPathLength),
		}
	}

	// Get the directory containing the binary
	binaryDir := filepath.Dir(binaryPath)

	var expanded string

	switch {
	case strings.HasPrefix(dep, "$ORIGIN/") || strings.HasPrefix(dep, "${ORIGIN}/"):
		// ELF $ORIGIN - replace with binary's directory
		if suffix, ok := strings.CutPrefix(dep, "${ORIGIN}/"); ok {
			expanded = filepath.Join(binaryDir, suffix)
		} else if suffix, ok := strings.CutPrefix(dep, "$ORIGIN/"); ok {
			expanded = filepath.Join(binaryDir, suffix)
		}

	case dep == "$ORIGIN" || dep == "${ORIGIN}":
		// Just $ORIGIN without path component
		expanded = binaryDir

	case strings.HasPrefix(dep, "@rpath/"):
		// Mach-O @rpath - try each RPATH in order
		suffix, _ := strings.CutPrefix(dep, "@rpath/")
		for _, rpath := range rpaths {
			// Expand any @loader_path in the RPATH itself
			expandedRpath := rpath
			if loaderSuffix, ok := strings.CutPrefix(rpath, "@loader_path/"); ok {
				expandedRpath = filepath.Join(binaryDir, loaderSuffix)
			} else if rpath == "@loader_path" {
				expandedRpath = binaryDir
			} else if execSuffix, ok := strings.CutPrefix(rpath, "@executable_path/"); ok {
				// For @executable_path in RPATH, use binary dir as approximation
				expandedRpath = filepath.Join(binaryDir, execSuffix)
			} else if rpath == "@executable_path" {
				expandedRpath = binaryDir
			}

			candidate := filepath.Join(expandedRpath, suffix)
			candidate = filepath.Clean(candidate)

			// Check if this candidate exists
			if _, err := os.Stat(candidate); err == nil {
				expanded = candidate
				break
			}
		}

		// If no RPATH matched, return the first candidate (for error reporting)
		if expanded == "" {
			if len(rpaths) > 0 {
				firstRpath := rpaths[0]
				if loaderSuffix, ok := strings.CutPrefix(firstRpath, "@loader_path/"); ok {
					firstRpath = filepath.Join(binaryDir, loaderSuffix)
				} else if firstRpath == "@loader_path" {
					firstRpath = binaryDir
				}
				expanded = filepath.Join(firstRpath, suffix)
			} else {
				// No RPATHs at all - can't expand
				return "", &ValidationError{
					Category: ErrUnexpandedVariable,
					Path:     dep,
					Message:  "@rpath variable cannot be expanded (no RPATH entries in binary)",
				}
			}
		}

	case strings.HasPrefix(dep, "@loader_path/"):
		// Mach-O @loader_path - same as binary's directory
		expanded = filepath.Join(binaryDir, strings.TrimPrefix(dep, "@loader_path/"))

	case dep == "@loader_path":
		expanded = binaryDir

	case strings.HasPrefix(dep, "@executable_path/"):
		// Mach-O @executable_path - for libraries, use the binary's directory
		// (In a real scenario, this should be the main executable's directory,
		// but for library validation, we use the loading binary's directory)
		expanded = filepath.Join(binaryDir, strings.TrimPrefix(dep, "@executable_path/"))

	case dep == "@executable_path":
		expanded = binaryDir

	default:
		// No path variable - return as-is after cleaning
		expanded = dep
	}

	// Apply path normalization
	expanded = filepath.Clean(expanded)

	// Check for unexpanded variables after expansion
	if containsPathVariable(expanded) {
		return "", &ValidationError{
			Category: ErrUnexpandedVariable,
			Path:     expanded,
			Message:  "path contains unexpanded variable after expansion",
		}
	}

	// Check path length after expansion
	if len(expanded) > MaxPathLength {
		return "", &ValidationError{
			Category: ErrPathLengthExceeded,
			Path:     expanded,
			Message:  fmt.Sprintf("expanded path exceeds %d characters", MaxPathLength),
		}
	}

	// Try to resolve symlinks (optional - don't fail if path doesn't exist)
	if resolved, err := filepath.EvalSymlinks(expanded); err == nil {
		expanded = resolved
	}

	// Validate canonical path prefix if specified
	if allowedPrefix != "" {
		// Clean the allowed prefix for consistent comparison
		cleanAllowed := filepath.Clean(allowedPrefix)
		if !strings.HasPrefix(expanded, cleanAllowed) {
			return "", &ValidationError{
				Category: ErrPathOutsideAllowed,
				Path:     expanded,
				Message:  "expanded path resolves outside allowed directories",
			}
		}
	}

	return expanded, nil
}

// containsPathVariable checks if a path contains any unexpanded path variables.
func containsPathVariable(path string) bool {
	// Check for ELF-style variables
	if strings.Contains(path, "$ORIGIN") || strings.Contains(path, "${ORIGIN}") {
		return true
	}

	// Check for Mach-O-style variables
	if strings.Contains(path, "@rpath") ||
		strings.Contains(path, "@loader_path") ||
		strings.Contains(path, "@executable_path") {
		return true
	}

	// Check for any remaining $ or @ that might indicate unexpanded variables
	// But be careful not to flag normal path characters
	for i := 0; i < len(path); i++ {
		if path[i] == '$' && i+1 < len(path) {
			next := path[i+1]
			// $X or ${X} pattern indicates a variable
			if (next >= 'A' && next <= 'Z') || (next >= 'a' && next <= 'z') || next == '{' {
				return true
			}
		}
		if path[i] == '@' && i+1 < len(path) {
			next := path[i+1]
			// @word pattern indicates a Mach-O variable
			if next >= 'a' && next <= 'z' {
				return true
			}
		}
	}

	return false
}

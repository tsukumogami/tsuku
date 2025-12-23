package actions

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// SetRpathAction implements RPATH modification for relocatable library loading
type SetRpathAction struct{ BaseAction }

// IsDeterministic returns true because set_rpath produces identical results.
func (SetRpathAction) IsDeterministic() bool { return true }

// Name returns the action name
func (a *SetRpathAction) Name() string {
	return "set_rpath"
}

// Execute modifies the RPATH of binaries for library resolution
//
// Parameters:
//   - binaries (required): List of binary paths to modify (relative to work_dir)
//   - rpath (optional): RPATH value to set (default: "$ORIGIN/../lib")
//   - create_wrapper (optional): Create wrapper script on failure (default: true)
func (a *SetRpathAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get binaries list (required)
	binaries, ok := GetStringSlice(params, "binaries")
	if !ok || len(binaries) == 0 {
		return fmt.Errorf("set_rpath action requires 'binaries' parameter")
	}

	// Build vars for variable substitution
	vars := GetStandardVars(ctx.Version, ctx.InstallDir, ctx.WorkDir, ctx.LibsDir)

	// Get rpath (defaults to $ORIGIN/../lib per design doc)
	rpath, _ := GetString(params, "rpath")
	if rpath == "" {
		rpath = "$ORIGIN/../lib"
	}

	// Expand variables in rpath
	rpath = ExpandVars(rpath, vars)

	// Validate RPATH value for security
	if err := validateRpath(rpath, ctx.LibsDir); err != nil {
		return fmt.Errorf("invalid rpath value: %w", err)
	}

	// Get create_wrapper flag (defaults to true)
	createWrapper := true
	if val, ok := GetBool(params, "create_wrapper"); ok {
		createWrapper = val
	}

	fmt.Printf("   Setting RPATH: %s\n", rpath)

	for _, binary := range binaries {
		binary = ExpandVars(binary, vars)
		binaryPath := filepath.Join(ctx.WorkDir, binary)

		// Security: Validate that the binary path stays within WorkDir
		if err := validatePathWithinDir(binaryPath, ctx.WorkDir); err != nil {
			return fmt.Errorf("invalid binary path %q: %w", binary, err)
		}

		// Check if binary exists
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			return fmt.Errorf("binary not found: %s", binaryPath)
		}

		// Detect binary format
		format, err := detectBinaryFormat(binaryPath)
		if err != nil {
			return fmt.Errorf("failed to detect binary format for %s: %w", binary, err)
		}

		var setErr error
		switch format {
		case "elf":
			setErr = setRpathLinux(ctx, binaryPath, rpath)
		case "macho":
			setErr = setRpathMacOS(binaryPath, rpath)
		default:
			return fmt.Errorf("unsupported binary format for %s: %s", binary, format)
		}

		if setErr != nil {
			if createWrapper {
				fmt.Printf("   Warning: RPATH modification failed for %s, creating wrapper script\n", binary)
				if wrapErr := createLibraryWrapper(binaryPath, rpath); wrapErr != nil {
					return fmt.Errorf("failed to create wrapper for %s: %w (original error: %v)", binary, wrapErr, setErr)
				}
				continue
			}
			return fmt.Errorf("failed to set RPATH for %s: %w", binary, setErr)
		}

		fmt.Printf("   Set RPATH for %s\n", binary)
	}

	fmt.Printf("   RPATH modification complete\n")
	return nil
}

// detectBinaryFormat detects whether a file is ELF (Linux) or Mach-O (macOS)
func detectBinaryFormat(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Read magic bytes
	magic := make([]byte, 4)
	if _, err := f.Read(magic); err != nil {
		return "", err
	}

	// ELF magic: 0x7f 'E' 'L' 'F'
	if bytes.Equal(magic, []byte{0x7f, 'E', 'L', 'F'}) {
		return "elf", nil
	}

	// Mach-O magic (32-bit): 0xfeedface or 0xcefaedfe (little-endian)
	// Mach-O magic (64-bit): 0xfeedfacf or 0xcffaedfe (little-endian)
	// Fat binary magic: 0xcafebabe or 0xbebafeca (little-endian)
	if bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xce}) || // 32-bit big-endian
		bytes.Equal(magic, []byte{0xce, 0xfa, 0xed, 0xfe}) || // 32-bit little-endian
		bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xcf}) || // 64-bit big-endian
		bytes.Equal(magic, []byte{0xcf, 0xfa, 0xed, 0xfe}) || // 64-bit little-endian
		bytes.Equal(magic, []byte{0xca, 0xfe, 0xba, 0xbe}) || // Fat binary big-endian
		bytes.Equal(magic, []byte{0xbe, 0xba, 0xfe, 0xca}) { // Fat binary little-endian
		return "macho", nil
	}

	return "unknown", nil
}

// setRpathLinux uses patchelf to modify RPATH on Linux binaries
func setRpathLinux(ctx *ExecutionContext, binaryPath, rpath string) error {
	// Find patchelf - check ExecPaths first (for installed dependencies), then fall back to PATH
	patchelfPath := ""
	for _, p := range ctx.ExecPaths {
		candidatePath := filepath.Join(p, "patchelf")
		if _, err := os.Stat(candidatePath); err == nil {
			patchelfPath = candidatePath
			break
		}
	}
	if patchelfPath == "" {
		// Fall back to system PATH
		var err error
		patchelfPath, err = exec.LookPath("patchelf")
		if err != nil {
			return fmt.Errorf("patchelf not found: install with 'apt install patchelf' or 'yum install patchelf'")
		}
	}

	// Homebrew bottles often have read-only files; make writable before patching
	info, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to stat binary: %w", err)
	}
	originalMode := info.Mode()
	if originalMode&0200 == 0 {
		if err := os.Chmod(binaryPath, originalMode|0200); err != nil {
			return fmt.Errorf("failed to make binary writable: %w", err)
		}
		// Restore original mode after patching (best-effort cleanup)
		defer func() { _ = os.Chmod(binaryPath, originalMode) }()
	}

	// First, remove existing RPATH/RUNPATH (security requirement)
	// Using --remove-rpath removes both RPATH and RUNPATH
	removeCmd := exec.Command(patchelfPath, "--remove-rpath", binaryPath)
	if output, err := removeCmd.CombinedOutput(); err != nil {
		// Some binaries don't have RPATH, which is fine
		if !strings.Contains(string(output), "cannot find") {
			// Log but continue - binary might not have RPATH
			fmt.Printf("   Note: Could not remove existing RPATH: %s\n", strings.TrimSpace(string(output)))
		}
	}

	// Set new RPATH using --force-rpath to set DT_RPATH instead of DT_RUNPATH
	// DT_RPATH takes precedence over LD_LIBRARY_PATH, providing better security
	// DT_RUNPATH (patchelf default) is overridden by LD_LIBRARY_PATH
	setCmd := exec.Command(patchelfPath, "--force-rpath", "--set-rpath", rpath, binaryPath)
	if output, err := setCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("patchelf --set-rpath failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	return nil
}

// setRpathMacOS uses install_name_tool to modify RPATH on macOS binaries
func setRpathMacOS(binaryPath, rpath string) error {
	// Convert $ORIGIN to @executable_path for macOS
	macRpath := strings.ReplaceAll(rpath, "$ORIGIN", "@executable_path")

	// Check if install_name_tool is available
	installNameTool, err := exec.LookPath("install_name_tool")
	if err != nil {
		return fmt.Errorf("install_name_tool not found: should be available on macOS")
	}

	// First, get existing rpaths using otool
	otool, err := exec.LookPath("otool")
	if err != nil {
		return fmt.Errorf("otool not found: should be available on macOS")
	}

	// Get existing rpaths
	otoolCmd := exec.Command(otool, "-l", binaryPath)
	output, err := otoolCmd.Output()
	if err != nil {
		return fmt.Errorf("otool failed: %w", err)
	}

	// Parse and remove existing rpaths (security requirement)
	existingRpaths := parseRpathsFromOtool(string(output))
	for _, oldRpath := range existingRpaths {
		deleteCmd := exec.Command(installNameTool, "-delete_rpath", oldRpath, binaryPath)
		if err := deleteCmd.Run(); err != nil {
			// Ignore errors - rpath might not exist
			fmt.Printf("   Note: Could not delete rpath %s\n", oldRpath)
		}
	}

	// Add new RPATH
	addCmd := exec.Command(installNameTool, "-add_rpath", macRpath, binaryPath)
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("install_name_tool -add_rpath failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	// Re-sign the binary with ad-hoc signature (required on Apple Silicon)
	if runtime.GOARCH == "arm64" || needsCodesign() {
		codesign, err := exec.LookPath("codesign")
		if err == nil {
			signCmd := exec.Command(codesign, "-f", "-s", "-", binaryPath)
			if output, err := signCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("codesign failed: %s: %w", strings.TrimSpace(string(output)), err)
			}
		}
	}

	return nil
}

// parseRpathsFromOtool extracts RPATH entries from otool -l output
func parseRpathsFromOtool(output string) []string {
	var rpaths []string
	lines := strings.Split(output, "\n")
	inRpathSection := false

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "cmd LC_RPATH" {
			inRpathSection = true
			continue
		}
		if inRpathSection && strings.HasPrefix(line, "path ") {
			// Extract path value: "path /some/path (offset XX)"
			pathLine := strings.TrimPrefix(line, "path ")
			if idx := strings.Index(pathLine, " (offset"); idx != -1 {
				rpaths = append(rpaths, pathLine[:idx])
			} else {
				rpaths = append(rpaths, pathLine)
			}
			inRpathSection = false
		}
		// Skip cmd line content - only 3 lines per LC_RPATH
		if inRpathSection && i > 0 && strings.HasPrefix(lines[i-1], "cmdsize") {
			inRpathSection = false
		}
	}

	return rpaths
}

// needsCodesign checks if the current macOS system requires codesigning
// (Apple Silicon Macs require signed binaries)
func needsCodesign() bool {
	// Check if we're on macOS
	if runtime.GOOS != "darwin" {
		return false
	}

	// Check for Apple Silicon by looking at the processor
	cmd := exec.Command("uname", "-m")
	output, err := cmd.Output()
	if err != nil {
		return true // Default to codesign if we can't detect
	}

	arch := strings.TrimSpace(string(output))
	return arch == "arm64"
}

// createLibraryWrapper creates a wrapper script that sets LD_LIBRARY_PATH/DYLD_LIBRARY_PATH
// This is used as a fallback when RPATH modification fails (e.g., signed binaries)
func createLibraryWrapper(binaryPath, _ string) error {
	// Security: Validate binary filename doesn't contain shell metacharacters
	baseName := filepath.Base(binaryPath)
	if err := validateBinaryName(baseName); err != nil {
		return fmt.Errorf("unsafe binary name for wrapper: %w", err)
	}

	// Security: Check that binaryPath is not a symlink (prevent symlink attacks)
	fileInfo, err := os.Lstat(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to stat binary: %w", err)
	}
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("binary is a symlink, refusing to create wrapper")
	}

	// Rename original binary
	origBinary := binaryPath + ".orig"
	if err := os.Rename(binaryPath, origBinary); err != nil {
		return fmt.Errorf("failed to rename binary: %w", err)
	}

	// Determine library path variable based on OS
	libPathVar := "LD_LIBRARY_PATH"
	if runtime.GOOS == "darwin" {
		libPathVar = "DYLD_LIBRARY_PATH"
	}

	// Create wrapper script
	// The script calculates paths at runtime using $0 to find its location
	// Using single quotes around the binary name to prevent shell injection
	origBaseName := baseName + ".orig"
	wrapper := fmt.Sprintf(`#!/bin/sh
# Wrapper script for library path configuration
# Generated by tsuku set_rpath action

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LIB_DIR="$(cd "$SCRIPT_DIR/../lib" 2>/dev/null && pwd || echo "$SCRIPT_DIR/../lib")"

export %s="$LIB_DIR${%s:+:$%s}"
exec "$SCRIPT_DIR"/'%s' "$@"
`, libPathVar, libPathVar, libPathVar, origBaseName)

	if err := os.WriteFile(binaryPath, []byte(wrapper), 0755); err != nil {
		// Try to restore original binary (best effort, ignore error)
		_ = os.Rename(origBinary, binaryPath)
		return fmt.Errorf("failed to write wrapper script: %w", err)
	}

	return nil
}

// validatePathWithinDir ensures that a path stays within the given directory
// This prevents path traversal attacks (e.g., "../../../etc/passwd")
func validatePathWithinDir(targetPath, baseDir string) error {
	// Clean and resolve to absolute paths
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("failed to resolve target path: %w", err)
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to resolve base directory: %w", err)
	}

	// Ensure the target starts with the base directory
	relPath, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return fmt.Errorf("failed to compute relative path: %w", err)
	}

	// Check if the relative path escapes the base directory
	if strings.HasPrefix(relPath, "..") {
		return fmt.Errorf("path escapes base directory")
	}

	return nil
}

// validateRpath validates that an RPATH value is safe
// This prevents RPATH injection attacks while allowing tsuku dependency paths
func validateRpath(rpath string, libsDir string) error {
	// Allow empty rpath (will use default)
	if rpath == "" {
		return nil
	}

	// Split colon-separated paths for validation
	paths := strings.Split(rpath, ":")

	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}

		// Note: We don't block ".." in paths because it's commonly used in RPATH
		// (e.g., "$ORIGIN/../lib"). The real security check is ensuring absolute
		// paths stay within $TSUKU_HOME/libs/.

		// Check if it's an absolute path
		if filepath.IsAbs(path) {
			// Absolute paths are only allowed within $TSUKU_HOME/libs/
			// This allows dependency libraries while preventing arbitrary system paths
			if libsDir != "" {
				// Clean paths for comparison
				cleanPath := filepath.Clean(path)
				cleanLibsDir := filepath.Clean(libsDir)

				// Check if path is within libsDir
				if !strings.HasPrefix(cleanPath, cleanLibsDir+string(filepath.Separator)) &&
					cleanPath != cleanLibsDir {
					return fmt.Errorf("absolute RPATH must be within tsuku libs directory (%s): %s", cleanLibsDir, path)
				}
			} else {
				// If libsDir is not set, reject absolute paths for safety
				return fmt.Errorf("RPATH must be relative using $ORIGIN or @executable_path: %s", path)
			}
		} else {
			// Relative paths must use platform-specific prefixes
			validPrefixes := []string{"$ORIGIN", "@executable_path", "@loader_path", "@rpath"}
			hasValidPrefix := false
			for _, prefix := range validPrefixes {
				if strings.HasPrefix(path, prefix) {
					hasValidPrefix = true
					break
				}
			}
			if !hasValidPrefix {
				return fmt.Errorf("relative RPATH must start with $ORIGIN, @executable_path, @loader_path, or @rpath: %s", path)
			}
		}
	}

	return nil
}

// validateBinaryName checks that a binary name is safe to use in a shell script
func validateBinaryName(name string) error {
	// Binary names should only contain alphanumeric, dash, underscore, and dot
	// This prevents shell injection in wrapper scripts
	validName := regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	if !validName.MatchString(name) {
		return fmt.Errorf("binary name contains unsafe characters")
	}
	return nil
}

package actions

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// HomebrewRelocateAction replaces Homebrew placeholders in extracted bottles.
// This handles both text files (direct replacement) and binaries (RPATH fixup).
type HomebrewRelocateAction struct{ BaseAction }

// IsDeterministic returns true because homebrew_relocate produces identical results.
func (HomebrewRelocateAction) IsDeterministic() bool { return true }

// Dependencies returns patchelf as a Linux-only install-time dependency.
// Patchelf is needed for ELF RPATH fixup on Linux; macOS uses install_name_tool (system-provided).
// TODO(#644): This dependency should be automatically inherited by composite actions like homebrew.
// Currently duplicated in HomebrewAction due to dependency resolution happening before decomposition.
func (HomebrewRelocateAction) Dependencies() ActionDeps {
	return ActionDeps{
		LinuxInstallTime: []string{"patchelf"},
	}
}

// Name returns the action name
func (a *HomebrewRelocateAction) Name() string { return "homebrew_relocate" }

// Execute replaces Homebrew placeholders in the work directory
//
// Parameters:
//   - formula (required): Homebrew formula name (for logging)
//
// The action:
// 1. Walks all files in the work directory
// 2. For text files: replaces @@HOMEBREW_PREFIX@@ and @@HOMEBREW_CELLAR@@
// 3. For binary files: uses patchelf/install_name_tool to fix RPATH
func (a *HomebrewRelocateAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get formula name (optional, for logging)
	formula, _ := GetString(params, "formula")
	if formula == "" {
		formula = "bottle"
	}

	// Determine install path for placeholder replacement
	// For libraries, use the final library installation path ($TSUKU_HOME/libs/recipename-version)
	// Note: Use recipe name (not formula name) since that's where install_binaries puts the library
	// For tools, use ToolInstallDir or InstallDir
	var installPath string
	if ctx.Recipe != nil && ctx.Recipe.Metadata.Type == "library" {
		// Library: use final library installation path
		// If LibsDir is not set in context, compute it from TSUKU_HOME
		libsDir := ctx.LibsDir
		if libsDir == "" {
			// Fallback: compute from TSUKU_HOME environment variable or default
			tsukuHome := os.Getenv("TSUKU_HOME")
			if tsukuHome == "" {
				// Default to ~/.tsuku
				homeDir, err := os.UserHomeDir()
				if err == nil {
					tsukuHome = filepath.Join(homeDir, ".tsuku")
				}
			}
			libsDir = filepath.Join(tsukuHome, "libs")
		}
		// IMPORTANT: Use recipe name, not formula name!
		// The actual installation goes to libs/recipename-version
		recipeName := ctx.Recipe.Metadata.Name
		installPath = filepath.Join(libsDir, recipeName+"-"+ctx.Version)
		fmt.Printf("   Relocating placeholders: %s (library, recipe: %s)\n", formula, recipeName)
	} else {
		// Tool: use ToolInstallDir or InstallDir
		installPath = ctx.ToolInstallDir
		if installPath == "" {
			installPath = ctx.InstallDir
		}
		fmt.Printf("   Relocating placeholders: %s\n", formula)
	}

	fmt.Printf("   Debug: installPath=%s\n", installPath)

	// For libraries, we need to handle @@HOMEBREW_CELLAR@@ and @@HOMEBREW_PREFIX@@ differently:
	// - @@HOMEBREW_CELLAR@@ should be replaced with the libs directory (e.g., /root/.tsuku/libs)
	// - @@HOMEBREW_PREFIX@@ should be replaced with the full library path (e.g., /root/.tsuku/libs/curl-8.17.0)
	// For tools, both are the same (the tool install directory).
	cellarPath := installPath
	if ctx.Recipe != nil && ctx.Recipe.Metadata.Type == "library" {
		// For libraries, cellar path is the parent directory
		cellarPath = filepath.Dir(installPath)
	}

	// Relocate placeholders in files
	if err := a.relocatePlaceholders(ctx, installPath, cellarPath, formula); err != nil {
		return fmt.Errorf("failed to relocate placeholders: %w", err)
	}

	fmt.Printf("   Relocation complete: %s\n", formula)

	return nil
}

// Note: homebrewPlaceholders is defined in homebrew.go

// relocatePlaceholders replaces Homebrew placeholders in all files
// For text files: direct replacement with install path
// For binary files: use patchelf/install_name_tool to reset RPATH
// prefixPath is used for @@HOMEBREW_PREFIX@@, cellarPath for @@HOMEBREW_CELLAR@@
// formula is the Homebrew formula name (may differ from recipe name, e.g., curl vs libcurl)
func (a *HomebrewRelocateAction) relocatePlaceholders(ctx *ExecutionContext, prefixPath, cellarPath, formula string) error {
	dir := ctx.WorkDir
	prefixReplacement := []byte(prefixPath)
	cellarReplacement := []byte(cellarPath)

	// Detect bottle build paths (e.g., /tmp/action-validator-XXXXXXXX/.install/FORMULA/VERSION)
	// by scanning for the pattern in all files. We'll collect unique prefixes to replace.
	bottlePrefixes := make(map[string]bool)

	// Collect binaries that need RPATH fixup
	var binariesToFix []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and symlinks
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		// Read file content
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		// Check if file contains any placeholder or bottle build path
		hasPlaceholder := false
		for _, placeholder := range homebrewPlaceholders {
			if bytes.Contains(content, placeholder) {
				hasPlaceholder = true
				break
			}
		}

		// Also check for bottle build paths (e.g., /tmp/action-validator-XXXXXXXX/.install/)
		hasBottlePath := bytes.Contains(content, []byte("/tmp/action-validator-")) ||
			bytes.Contains(content, []byte("@@HOMEBREW"))

		if !hasPlaceholder && !hasBottlePath {
			return nil
		}

		// Collect bottle prefixes from this file for later replacement
		if hasBottlePath {
			// Debug: Log which files contain bottle paths
			if strings.HasSuffix(path, "curl-config") {
				contentStr := string(content)
				hasMarker := strings.Contains(contentStr, "/tmp/action-validator-")
				hasPrefix := strings.Contains(contentStr, "prefix=")
				fmt.Printf("   Debug: Scanning %s (size: %d, has /tmp marker: %v, has prefix: %v)\n",
					filepath.Base(path), len(content), hasMarker, hasPrefix)

				// Show what prefix contains
				if hasPrefix {
					idx := strings.Index(contentStr, "prefix=")
					if idx >= 0 && idx+60 < len(contentStr) {
						sample := contentStr[idx : idx+60]
						fmt.Printf("   Debug: prefix line: %s\n", sample)
					}
				}
			}
			a.extractBottlePrefixes(content, bottlePrefixes)
		}

		// Determine if binary or text file
		isBinary := a.isBinaryFile(content)

		if isBinary {
			// Binary files: collect for RPATH fixup using patchelf/install_name_tool
			binariesToFix = append(binariesToFix, path)
		} else {
			// Text files: replace placeholders with appropriate paths
			newContent := content

			// Replace Homebrew placeholders with correct paths:
			// @@HOMEBREW_PREFIX@@ → prefixPath (e.g., /root/.tsuku/libs/libcurl-8.17.0)
			newContent = bytes.ReplaceAll(newContent, []byte("@@HOMEBREW_PREFIX@@"), prefixReplacement)

			// For @@HOMEBREW_CELLAR@@, bottles contain formula-specific patterns like:
			// @@HOMEBREW_CELLAR@@/curl/8.17.0 (using FORMULA name, not recipe name)
			// We need to replace with recipe-based path: /root/.tsuku/libs/libcurl-8.17.0
			// So we replace the ENTIRE pattern @@HOMEBREW_CELLAR@@/formula/version with the install path
			// This handles cases where formula name != recipe name (e.g., curl vs libcurl)
			cellarFormulaPattern := fmt.Sprintf("@@HOMEBREW_CELLAR@@/%s/%s", formula, ctx.Version)
			newContent = bytes.ReplaceAll(newContent, []byte(cellarFormulaPattern), prefixReplacement)

			// Also replace bare @@HOMEBREW_CELLAR@@ for any other occurrences
			newContent = bytes.ReplaceAll(newContent, []byte("@@HOMEBREW_CELLAR@@"), cellarReplacement)

			// Replace bottle build paths
			// We replace the entire bottle-specific path (e.g., /tmp/action-validator-XXX/.install/curl/8.17.0)
			// with the prefix path. This is done using a simple pattern match and replace.
			for prefix := range bottlePrefixes {
				// Replace paths like /tmp/action-validator-XXX/.install/FORMULA/VERSION
				// with the installation path
				newContent = bytes.ReplaceAll(newContent, []byte(prefix), prefixReplacement)
			}

			// Homebrew bottles often have read-only files; make writable before writing
			originalMode := info.Mode()
			if originalMode&0200 == 0 {
				if err := os.Chmod(path, originalMode|0200); err != nil {
					return fmt.Errorf("failed to make %s writable: %w", path, err)
				}
			}

			if err := os.WriteFile(path, newContent, originalMode); err != nil {
				return fmt.Errorf("failed to write %s: %w", path, err)
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Debug: Show detected bottle prefixes
	if len(bottlePrefixes) > 0 {
		fmt.Printf("   Debug: Found %d bottle path(s) to relocate\n", len(bottlePrefixes))
		for prefix := range bottlePrefixes {
			if len(prefix) > 60 {
				fmt.Printf("     %s...\n", prefix[:60])
			} else {
				fmt.Printf("     %s\n", prefix)
			}
		}
	}

	// Fix RPATH on binary files using patchelf/install_name_tool
	for _, binaryPath := range binariesToFix {
		if err := a.fixBinaryRpath(ctx, binaryPath, prefixPath); err != nil {
			return fmt.Errorf("failed to fix RPATH for %s: %w", binaryPath, err)
		}
	}

	return nil
}

// fixBinaryRpath uses patchelf or install_name_tool to set a proper RPATH
// This replaces the Homebrew placeholder RPATH with a working path
func (a *HomebrewRelocateAction) fixBinaryRpath(ctx *ExecutionContext, binaryPath, installPath string) error {
	// Detect binary format
	f, err := os.Open(binaryPath)
	if err != nil {
		return err
	}

	magic := make([]byte, 4)
	_, err = f.Read(magic)
	f.Close()
	if err != nil {
		return err
	}

	// Check if it's an ELF binary
	if bytes.Equal(magic, []byte{0x7f, 'E', 'L', 'F'}) {
		return a.fixElfRpath(ctx, binaryPath, installPath)
	}

	// Check if it's a Mach-O binary
	if bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xce}) || // 32-bit big-endian
		bytes.Equal(magic, []byte{0xce, 0xfa, 0xed, 0xfe}) || // 32-bit little-endian
		bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xcf}) || // 64-bit big-endian
		bytes.Equal(magic, []byte{0xcf, 0xfa, 0xed, 0xfe}) || // 64-bit little-endian
		bytes.Equal(magic, []byte{0xca, 0xfe, 0xba, 0xbe}) || // Fat binary big-endian
		bytes.Equal(magic, []byte{0xbe, 0xba, 0xfe, 0xca}) { // Fat binary little-endian
		return a.fixMachoRpath(binaryPath, installPath)
	}

	// Not a recognized binary format, skip silently
	return nil
}

// fixElfRpath uses patchelf to set RPATH on Linux ELF binaries
func (a *HomebrewRelocateAction) fixElfRpath(ctx *ExecutionContext, binaryPath, installPath string) error {
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
			// Patchelf is declared as a dependency but may not be in PATH yet during bootstrap.
			// Gracefully degrade - the dependency declaration ensures it gets installed for future use.
			fmt.Printf("   Warning: patchelf not found, skipping RPATH fix for %s\n", filepath.Base(binaryPath))
			return nil
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

	// Remove existing RPATH first (contains placeholders)
	removeCmd := exec.Command(patchelfPath, "--remove-rpath", binaryPath)
	if output, err := removeCmd.CombinedOutput(); err != nil {
		// Some binaries might not have RPATH, which is fine
		if !strings.Contains(string(output), "cannot find") {
			// Log but continue
			fmt.Printf("   Note: Could not remove existing RPATH from %s\n", filepath.Base(binaryPath))
		}
	}

	// For shared libraries, set RPATH to $ORIGIN so they can find sibling libraries
	// For executables, RPATH would typically be $ORIGIN/../lib
	// Since Homebrew bottles are libraries, use $ORIGIN
	newRpath := "$ORIGIN"

	// Check if there's a lib subdirectory (common patterns for homebrew bottles):
	// 1. lib/ as sibling to binary (e.g., bin/tool and bin/lib/)
	// 2. lib/ one level up from binary (e.g., bin/tool and lib/)
	libDir := filepath.Join(filepath.Dir(binaryPath), "lib")
	if _, err := os.Stat(libDir); err != nil {
		// Try one level up (common for bin/tool + lib/ structure)
		libDir = filepath.Join(filepath.Dir(filepath.Dir(binaryPath)), "lib")
	}

	if _, err := os.Stat(libDir); err == nil {
		// Binary is not in lib/, might need to point to lib/
		relPath, _ := filepath.Rel(filepath.Dir(binaryPath), libDir)
		if relPath != "" && relPath != "." {
			newRpath = "$ORIGIN/" + relPath
		}
	}

	setCmd := exec.Command(patchelfPath, "--force-rpath", "--set-rpath", newRpath, binaryPath)
	if output, err := setCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("patchelf --set-rpath failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	// Fix the ELF interpreter if it contains Homebrew placeholders
	// Homebrew bottles on Linux have interpreter set to @@HOMEBREW_PREFIX@@/lib/ld.so
	// which needs to be changed to the system loader
	if err := a.fixElfInterpreter(patchelfPath, binaryPath); err != nil {
		// Log but don't fail - some binaries (shared libs) don't have interpreters
		fmt.Printf("   Note: Could not fix interpreter for %s: %v\n", filepath.Base(binaryPath), err)
	}

	return nil
}

// fixElfInterpreter fixes the ELF interpreter path if it contains Homebrew placeholders
func (a *HomebrewRelocateAction) fixElfInterpreter(patchelf, binaryPath string) error {
	// Read current interpreter
	printCmd := exec.Command(patchelf, "--print-interpreter", binaryPath)
	output, err := printCmd.CombinedOutput()
	if err != nil {
		// Shared libraries don't have interpreters, this is expected
		return nil
	}

	currentInterp := strings.TrimSpace(string(output))
	if !strings.Contains(currentInterp, "@@HOMEBREW") && !strings.Contains(currentInterp, "HOMEBREW_PREFIX") {
		// Interpreter doesn't contain placeholders, no fix needed
		return nil
	}

	// Determine system interpreter based on architecture
	systemInterp := "/lib64/ld-linux-x86-64.so.2"
	if runtime.GOARCH == "arm64" {
		systemInterp = "/lib/ld-linux-aarch64.so.1"
	}

	setCmd := exec.Command(patchelf, "--set-interpreter", systemInterp, binaryPath)
	if output, err := setCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("patchelf --set-interpreter failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	return nil
}

// fixMachoRpath uses install_name_tool to fix RPATH on macOS Mach-O binaries
func (a *HomebrewRelocateAction) fixMachoRpath(binaryPath, installPath string) error {
	installNameTool, err := exec.LookPath("install_name_tool")
	if err != nil {
		fmt.Printf("   Warning: install_name_tool not found, skipping RPATH fix for %s\n", filepath.Base(binaryPath))
		return nil
	}

	otool, err := exec.LookPath("otool")
	if err != nil {
		fmt.Printf("   Warning: otool not found, skipping RPATH fix for %s\n", filepath.Base(binaryPath))
		return nil
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

	// Get existing rpaths that contain placeholders
	otoolCmd := exec.Command(otool, "-l", binaryPath)
	output, err := otoolCmd.Output()
	if err != nil {
		return fmt.Errorf("otool failed: %w", err)
	}

	// Parse and delete rpaths containing HOMEBREW placeholders
	lines := strings.Split(string(output), "\n")
	inRpathSection := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "cmd LC_RPATH" {
			inRpathSection = true
			continue
		}
		if inRpathSection && strings.HasPrefix(line, "path ") {
			pathLine := strings.TrimPrefix(line, "path ")
			if idx := strings.Index(pathLine, " (offset"); idx != -1 {
				pathLine = pathLine[:idx]
			}
			// Delete if it contains placeholder
			if strings.Contains(pathLine, "HOMEBREW") {
				deleteCmd := exec.Command(installNameTool, "-delete_rpath", pathLine, binaryPath)
				_ = deleteCmd.Run() // Ignore errors
			}
			inRpathSection = false
		}
	}

	// Add new RPATH
	// Check if there's a lib subdirectory (common patterns for homebrew bottles):
	// 1. lib/ as sibling to binary (e.g., bin/tool and bin/lib/)
	// 2. lib/ one level up from binary (e.g., bin/tool and lib/)
	newRpath := "@loader_path"
	libDir := filepath.Join(filepath.Dir(binaryPath), "lib")
	if _, err := os.Stat(libDir); err != nil {
		// Try one level up (common for bin/tool + lib/ structure)
		libDir = filepath.Join(filepath.Dir(filepath.Dir(binaryPath)), "lib")
	}

	if _, err := os.Stat(libDir); err == nil {
		// Binary is not in lib/, might need to point to lib/
		relPath, _ := filepath.Rel(filepath.Dir(binaryPath), libDir)
		if relPath != "" && relPath != "." {
			newRpath = "@loader_path/" + relPath
		}
	}

	addCmd := exec.Command(installNameTool, "-add_rpath", newRpath, binaryPath)
	if output, err := addCmd.CombinedOutput(); err != nil {
		// Ignore "would duplicate" errors
		if !strings.Contains(string(output), "would duplicate") {
			return fmt.Errorf("install_name_tool -add_rpath failed: %s: %w", strings.TrimSpace(string(output)), err)
		}
	}

	// Fix install_name for shared libraries
	// Shared libraries have an embedded install_name that homebrew sets to the cellar path
	// We need to change it to @rpath/libname.dylib so it can be found via RPATH
	if strings.HasSuffix(binaryPath, ".dylib") || strings.Contains(binaryPath, ".dylib.") {
		basename := filepath.Base(binaryPath)
		newInstallName := "@rpath/" + basename

		idCmd := exec.Command(installNameTool, "-id", newInstallName, binaryPath)
		if output, err := idCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("install_name_tool -id failed: %s: %w", strings.TrimSpace(string(output)), err)
		}
	}

	// Fix library references in binaries
	// Binaries have explicit references to libraries that need to be updated
	// to use @rpath instead of absolute homebrew paths
	otoolLibCmd := exec.Command(otool, "-L", binaryPath)
	libOutput, err := otoolLibCmd.Output()
	if err == nil {
		libLines := strings.Split(string(libOutput), "\n")
		for _, line := range libLines[1:] { // Skip first line (the binary itself)
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Extract library path (format: "	/path/to/lib.dylib (compatibility version...)")
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			libPath := parts[0]

			// Change library references that contain HOMEBREW placeholders
			if strings.Contains(libPath, "HOMEBREW") || strings.Contains(libPath, "@@") {
				// Extract basename and use @rpath
				libBasename := filepath.Base(libPath)
				newLibRef := "@rpath/" + libBasename

				changeCmd := exec.Command(installNameTool, "-change", libPath, newLibRef, binaryPath)
				_ = changeCmd.Run() // Ignore errors - not all references need changing
			}
		}
	}

	// Re-sign the binary (required on Apple Silicon)
	if runtime.GOARCH == "arm64" {
		codesign, err := exec.LookPath("codesign")
		if err == nil {
			signCmd := exec.Command(codesign, "-f", "-s", "-", binaryPath)
			_ = signCmd.Run() // Best effort
		}
	}

	return nil
}

// isBinaryFile detects if content is binary (contains null bytes in first 8KB)
func (a *HomebrewRelocateAction) isBinaryFile(content []byte) bool {
	// Check first 8KB for null bytes
	checkLen := 8192
	if len(content) < checkLen {
		checkLen = len(content)
	}

	for i := 0; i < checkLen; i++ {
		if content[i] == 0 {
			return true
		}
	}

	return false
}

// extractBottlePrefixes scans content for Homebrew bottle build paths and adds them to the map.
// Bottle paths follow the pattern: /tmp/action-validator-XXXXXXXX/.install/FORMULA/VERSION
// We need to extract the full path to replace it with the actual installation path.
func (a *HomebrewRelocateAction) extractBottlePrefixes(content []byte, prefixes map[string]bool) {
	contentStr := string(content)

	// Look for /tmp/action-validator-XXXXXXXX/.install/FORMULA/VERSION patterns
	searchPos := 0
	marker := "/tmp/action-validator-"
	foundCount := 0

	for {
		// Find next occurrence of /tmp/action-validator-
		idx := strings.Index(contentStr[searchPos:], marker)
		if idx == -1 {
			break
		}

		foundCount++

		// Adjust to absolute position
		absIdx := searchPos + idx

		// Extract path starting from /tmp/action-validator-
		remaining := contentStr[absIdx:]

		// Find the end of the path (whitespace, quote, newline, etc.)
		endIdx := strings.IndexAny(remaining, " \t\n\r'\"<>;:|")
		if endIdx == -1 {
			endIdx = len(remaining)
		}

		pathStr := remaining[:endIdx]

		// Debug: Show what we found
		fmt.Printf("   Debug: Found candidate path #%d: %s (has .install: %v)\n",
			foundCount, pathStr, strings.Contains(pathStr, "/.install/"))

		// Only add if it looks like a valid bottle path (contains /.install/)
		if strings.Contains(pathStr, "/.install/") {
			prefixes[pathStr] = true
		}

		// Move search position past this occurrence
		searchPos = absIdx + len(marker)
	}
}

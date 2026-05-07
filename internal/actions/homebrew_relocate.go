package actions

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/tsukumogami/tsuku/internal/progress"
	"github.com/tsukumogami/tsuku/internal/version"
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
	reporter := ctx.GetReporter()
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
		reporter.Status(fmt.Sprintf("   Relocating placeholders: %s (library, recipe: %s)", formula, recipeName))
	} else {
		// Tool: use ToolInstallDir or InstallDir
		installPath = ctx.ToolInstallDir
		if installPath == "" {
			installPath = ctx.InstallDir
		}
		reporter.Status(fmt.Sprintf("   Relocating placeholders: %s", formula))
	}

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
	if err := a.relocatePlaceholders(ctx, installPath, cellarPath, formula, reporter); err != nil {
		return fmt.Errorf("failed to relocate placeholders: %w", err)
	}

	// SONAME completeness scan: walk every binary in the bottle, parse its
	// NEEDED SONAMES, classify each against the SONAME index, and produce
	// a local []chainEntry of under-declared deps to auto-include. Runs
	// before the chain walks so the auto-included entries can be unioned
	// into the chain along with ctx.Dependencies.RuntimeDependencies.
	//
	// The scanner does NOT mutate ctx.Dependencies. Auto-included entries
	// live in Execute scope only. When ctx.SonameIndex is nil (production
	// call sites that haven't been plumbed yet) the scanner is a no-op
	// and the chain walk degrades to RuntimeDependencies-only — recipes
	// behave exactly as they did pre-Issue 6.
	scanResult, err := runSonameCompletenessScan(ctx, reporter)
	if err != nil {
		return fmt.Errorf("SONAME completeness scan failed: %w", err)
	}

	// macOS RPATH chain: emit @loader_path-relative entries for each
	// runtime dependency the recipe declared (and each auto-included entry
	// the SONAME scan produced). Runs after relocatePlaceholders (which
	// performs the per-binary fixMachoRpath pass that wipes stale HOMEBREW
	// rpaths) so the new entries survive the wipe.
	//
	// The chain function fires for any recipe Type — the Type == "library"
	// gate that used to live here was lifted as part of Issue 3. Whether
	// the chain emits anything is now driven by the union of declared and
	// auto-included entries being non-empty.
	if runtime.GOOS == "darwin" && (len(ctx.Dependencies.RuntimeDependencies) > 0 || len(scanResult.AutoInclude) > 0) {
		if err := a.fixDylibRpathChain(ctx, installPath, scanResult.AutoInclude, reporter); err != nil {
			return fmt.Errorf("failed to fix dylib RPATH chain: %w", err)
		}
	}

	// Linux RPATH chain: the ELF mirror of fixDylibRpathChain. Emits one
	// $ORIGIN-relative RPATH entry per declared (or auto-included) runtime
	// dependency, written via patchelf --force-rpath --set-rpath (DT_RPATH,
	// not DT_RUNPATH — DT_RUNPATH has subtle resolution differences that
	// break some tools' shared-library lookups, e.g. wget's libunistring).
	// Runs after the per-binary fixElfRpath pass executed by
	// relocatePlaceholders so the chain entries can be appended to the
	// rpath that pass installed.
	if runtime.GOOS == "linux" && (len(ctx.Dependencies.RuntimeDependencies) > 0 || len(scanResult.AutoInclude) > 0) {
		if err := a.fixElfRpathChain(ctx, installPath, scanResult.AutoInclude, reporter); err != nil {
			return fmt.Errorf("failed to fix ELF RPATH chain: %w", err)
		}
	}

	// Library install-time chain. Kept as a separate helper from
	// fixDylibRpathChain because the source field differs: the legacy
	// helper walks ctx.Dependencies.InstallTime (populated from
	// metadata.dependencies, e.g. libevent declares dependencies =
	// ["openssl"]), while fixDylibRpathChain reads
	// ctx.Dependencies.RuntimeDependencies (populated from
	// metadata.runtime_dependencies). Library recipes that use the legacy
	// dependencies field do not flow through the new chain, so this helper
	// remains the install-time entry point for them.
	//
	// As of Issue 4 the helper emits @loader_path-relative RPATHs (computed
	// via the same computeChainRpaths machinery as fixDylibRpathChain),
	// which makes installs portable across $TSUKU_HOME locations.
	if runtime.GOOS == "darwin" && ctx.Recipe != nil && ctx.Recipe.Metadata.Type == "library" {
		if err := a.fixLibraryInstallTimeChain(ctx, installPath, reporter); err != nil {
			return fmt.Errorf("failed to fix library install-time chain: %w", err)
		}
	}

	reporter.Status(fmt.Sprintf("   Relocation complete: %s", formula))

	return nil
}

// Note: homebrewPlaceholders is defined in homebrew.go

// relocatePlaceholders replaces Homebrew placeholders in all files
// For text files: direct replacement with install path
// For binary files: use patchelf/install_name_tool to reset RPATH
// prefixPath is used for @@HOMEBREW_PREFIX@@, cellarPath for @@HOMEBREW_CELLAR@@
// formula is the Homebrew formula name (may differ from recipe name, e.g., curl vs libcurl)
func (a *HomebrewRelocateAction) relocatePlaceholders(ctx *ExecutionContext, prefixPath, cellarPath, formula string, reporter progress.Reporter) error {
	dir := ctx.WorkDir
	prefixReplacement := []byte(prefixPath)
	cellarReplacement := []byte(cellarPath)

	// Detect bottle build paths (e.g., /tmp/action-validator-XXXXXXXX/.install/FORMULA/VERSION)
	// by scanning for the pattern in all files. We'll collect mappings of full path -> prefix.
	// This allows us to replace the prefix while preserving any suffix after the version.
	bottlePrefixes := make(map[string]string)

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
			// We replace the bottle prefix with the install path, preserving any suffix.
			// e.g., /tmp/action-validator-XXX/.install/pod/1.16.2/libexec/bin/pod
			//    -> /root/.tsuku/tools/cocoapods-1.16.2/libexec/bin/pod
			for fullPath, bottlePrefix := range bottlePrefixes {
				// Extract suffix (everything after the bottle prefix)
				suffix := fullPath[len(bottlePrefix):]

				// Security: validate suffix doesn't contain traversal attempts
				if strings.Contains(suffix, "..") {
					// Skip paths with traversal attempts - defense in depth
					continue
				}

				// Construct replacement: install path + preserved suffix
				replacement := prefixPath + suffix
				newContent = bytes.ReplaceAll(newContent, []byte(fullPath), []byte(replacement))
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

	// Fix RPATH on binary files using patchelf/install_name_tool
	for _, binaryPath := range binariesToFix {
		if err := a.fixBinaryRpath(ctx, binaryPath, prefixPath, reporter); err != nil {
			return fmt.Errorf("failed to fix RPATH for %s: %w", binaryPath, err)
		}
	}

	return nil
}

// fixBinaryRpath uses patchelf or install_name_tool to set a proper RPATH
// This replaces the Homebrew placeholder RPATH with a working path
func (a *HomebrewRelocateAction) fixBinaryRpath(ctx *ExecutionContext, binaryPath, installPath string, reporter progress.Reporter) error {
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
		return a.fixElfRpath(ctx, binaryPath, installPath, reporter)
	}

	// Check if it's a Mach-O binary
	if bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xce}) || // 32-bit big-endian
		bytes.Equal(magic, []byte{0xce, 0xfa, 0xed, 0xfe}) || // 32-bit little-endian
		bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xcf}) || // 64-bit big-endian
		bytes.Equal(magic, []byte{0xcf, 0xfa, 0xed, 0xfe}) || // 64-bit little-endian
		bytes.Equal(magic, []byte{0xca, 0xfe, 0xba, 0xbe}) || // Fat binary big-endian
		bytes.Equal(magic, []byte{0xbe, 0xba, 0xfe, 0xca}) { // Fat binary little-endian
		return a.fixMachoRpath(binaryPath, installPath, reporter)
	}

	// Not a recognized binary format, skip silently
	return nil
}

// findPatchelf locates the patchelf binary by checking (in order):
//  1. ctx.ExecPaths (dependency bin dirs added during plan execution)
//  2. System PATH
//  3. $TSUKU_HOME/tools/patchelf-*/bin/patchelf (glob for any installed version)
//  4. $TSUKU_HOME/tools/current/patchelf (current symlink)
//
// Returns the path to patchelf, or an error if not found anywhere.
func (a *HomebrewRelocateAction) findPatchelf(ctx *ExecutionContext) (string, error) {
	// 1. Check ExecPaths (dependency bin dirs from earlier plan steps)
	for _, p := range ctx.ExecPaths {
		candidate := filepath.Join(p, "patchelf")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// 2. Check system PATH
	if p, err := exec.LookPath("patchelf"); err == nil {
		return p, nil
	}

	// 3-4. Check $TSUKU_HOME tools directory (glob and current symlink)
	if p, err := a.findPatchelfInToolsDir(ctx.ToolsDir, ctx.CurrentDir); err == nil {
		return p, nil
	}

	return "", fmt.Errorf("patchelf not found: checked ExecPaths, system PATH, %s/patchelf-*/bin/, and %s/", ctx.ToolsDir, ctx.CurrentDir)
}

// findPatchelfInToolsDir searches for patchelf in the tsuku tools directory.
// It first globs for versioned install dirs, then checks the current symlink dir.
func (a *HomebrewRelocateAction) findPatchelfInToolsDir(toolsDir, currentDir string) (string, error) {
	// Glob $TSUKU_HOME/tools/patchelf-*/bin/patchelf
	if toolsDir != "" {
		matches, err := filepath.Glob(filepath.Join(toolsDir, "patchelf-*", "bin", "patchelf"))
		if err == nil && len(matches) > 0 {
			// Use the last match (highest version due to lexicographic sort)
			return matches[len(matches)-1], nil
		}
	}

	// Check $TSUKU_HOME/tools/current/patchelf
	if currentDir != "" {
		candidate := filepath.Join(currentDir, "patchelf")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("patchelf not found in tools directory")
}

// fixElfRpath uses patchelf to set RPATH on Linux ELF binaries
func (a *HomebrewRelocateAction) fixElfRpath(ctx *ExecutionContext, binaryPath, installPath string, reporter progress.Reporter) error {
	// Find patchelf using multi-location discovery
	patchelfPath, err := a.findPatchelf(ctx)
	if err != nil {
		return fmt.Errorf("cannot fix RPATH for %s: %w", filepath.Base(binaryPath), err)
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
			reporter.Log("   Note: Could not remove existing RPATH from %s", filepath.Base(binaryPath))
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
		reporter.Log("   Note: Could not fix interpreter for %s: %v", filepath.Base(binaryPath), err)
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
func (a *HomebrewRelocateAction) fixMachoRpath(binaryPath, installPath string, reporter progress.Reporter) error {
	installNameTool, err := exec.LookPath("install_name_tool")
	if err != nil {
		reporter.Warn("   install_name_tool not found, skipping RPATH fix for %s", filepath.Base(binaryPath))
		return nil
	}

	otool, err := exec.LookPath("otool")
	if err != nil {
		reporter.Warn("   otool not found, skipping RPATH fix for %s", filepath.Base(binaryPath))
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

// chainEntry describes a single runtime dependency contribution to the
// RPATH chain: the dep's recipe name and resolved version. The version is
// looked up from ctx.Dependencies.Runtime; entries with no resolved version
// fall back to "latest", matching the existing behavior of the legacy
// install-time chain when versions weren't pinned.
type chainEntry struct {
	name    string
	version string
}

// resolveRuntimeDepVersion returns the on-disk version of dep `name` for
// chain emission. Resolution order:
//
//  1. ctx.Dependencies.Runtime[name] if it looks like a real version
//     (non-empty and not the unresolved sentinel "latest"). This preserves
//     the resolved version when the dep was carried through the runtime
//     graph by the executor.
//  2. Glob ctx.LibsDir/<name>-* and pick the highest installed version.
//     Used when Runtime[name] is empty (auto-included deps that arrived as
//     transitive runtime deps under another name) or the unresolved
//     sentinel "latest" (declared deps where the executor hadn't pinned a
//     version by chain-emit time). The tiebreaker uses semver-aware
//     comparison via internal/version.SortVersionsDescending so multiple
//     installed siblings produce a deterministic pick.
//  3. Otherwise: ("", false). Callers must NOT emit an RPATH for an
//     unresolved dep — historically the chain walk fell back to "latest",
//     but $TSUKU_HOME/libs/<name>-latest does not exist on disk and the
//     resulting RPATH made the binary fail at runtime with "cannot open
//     shared object file."
func resolveRuntimeDepVersion(ctx *ExecutionContext, name string) (string, bool) {
	if ctx == nil || name == "" {
		return "", false
	}
	if ctx.Dependencies.Runtime != nil {
		if v := ctx.Dependencies.Runtime[name]; v != "" && v != "latest" {
			return v, true
		}
	}
	if ctx.LibsDir == "" {
		return "", false
	}
	matches, _ := filepath.Glob(filepath.Join(ctx.LibsDir, name+"-*"))
	if len(matches) == 0 {
		return "", false
	}
	versions := make([]string, 0, len(matches))
	for _, m := range matches {
		v := strings.TrimPrefix(filepath.Base(m), name+"-")
		if v == "" {
			continue
		}
		versions = append(versions, v)
	}
	if len(versions) == 0 {
		return "", false
	}
	sorted := version.SortVersionsDescending(versions)
	return sorted[0], true
}

// buildChainEntries returns the union of author-declared chain entries (from
// ctx.Dependencies.RuntimeDependencies, preserving the recipe's declared
// order) and the auto-included entries the SONAME completeness scan
// produced. Declared entries come first; auto-included entries follow,
// preserving scanner order. Entries that share a recipe name with an
// already-emitted entry are dropped (declared wins over auto-included on
// name collision; the declared version is what the recipe author asked
// for).
//
// Each declared entry's version is resolved via resolveRuntimeDepVersion:
// the executor's resolved Runtime map takes precedence, then a glob over
// ctx.LibsDir/<name>-* picks the highest-installed sibling. A declared dep
// that resolves to neither (no executor pin, no on-disk install) is
// skipped with a warning rather than emitted as a known-bad "-latest"
// path. Author-supplied extra entries are trusted: they already carry a
// resolved version from the SONAME scan's resolveAutoIncludeVersion path
// (which uses the same resolver).
//
// Splitting this helper out from fixDylibRpathChain / fixElfRpathChain
// keeps the union semantics in one place: both chain walks consume the
// same union and a future bug fix to the merge rule won't have to be
// applied twice.
func buildChainEntries(ctx *ExecutionContext, extra []chainEntry, reporter progress.Reporter) []chainEntry {
	if ctx == nil {
		return nil
	}
	entries := make([]chainEntry, 0, len(ctx.Dependencies.RuntimeDependencies)+len(extra))
	seen := make(map[string]bool, len(ctx.Dependencies.RuntimeDependencies)+len(extra))
	for _, name := range ctx.Dependencies.RuntimeDependencies {
		if seen[name] {
			continue
		}
		seen[name] = true
		v, ok := resolveRuntimeDepVersion(ctx, name)
		if !ok {
			if reporter != nil {
				reporter.Warn("   runtime dep %q has no resolved version and no installed sibling under %s; skipping RPATH chain entry",
					name, ctx.LibsDir)
			}
			continue
		}
		entries = append(entries, chainEntry{name: name, version: v})
	}
	for _, e := range extra {
		if e.name == "" || seen[e.name] {
			continue
		}
		seen[e.name] = true
		entries = append(entries, e)
	}
	return entries
}

// fixDylibRpathChain adds @loader_path-relative RPATH entries to Mach-O
// binaries (both bin/ and lib/) for each entry in
// ctx.Dependencies.RuntimeDependencies, unioned with the auto-included
// slice produced by the SONAME completeness scan (extra). Author-declared
// entries appear first in the emitted RPATH order; auto-included entries
// follow.
//
// Path form: relative to the binary's directory (loaderDir), computed via
// filepath.Rel after EvalSymlinks on both ends. Each computed entry is
// post-checked: filepath.Join(loaderDir, relPath) must resolve back inside
// ctx.LibsDir. A failed check fails the install with a clear error before
// any install_name_tool -add_rpath invocation for that entry.
//
// Order: this runs after relocatePlaceholders (which performs the
// per-binary fixMachoRpath pass that wipes HOMEBREW rpaths via
// install_name_tool -delete_rpath), so the new entries survive the wipe.
func (a *HomebrewRelocateAction) fixDylibRpathChain(ctx *ExecutionContext, installPath string, extra []chainEntry, reporter progress.Reporter) error {
	// Only run on macOS
	if runtime.GOOS != "darwin" {
		return nil
	}

	entries := buildChainEntries(ctx, extra, reporter)

	if len(entries) == 0 {
		// No-op when no runtime dependencies were declared and the scanner
		// produced no auto-includes.
		return nil
	}

	// Collect Mach-O binaries to patch: dylibs in lib/ (library and tool
	// recipes can both ship dylibs) and executables in bin/ (tool case).
	var binaries []string
	for _, sub := range []string{"lib", "bin"} {
		root := filepath.Join(ctx.WorkDir, sub)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			if a.isMachOBinary(path) {
				binaries = append(binaries, path)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to walk %s directory: %w", sub, err)
		}
	}

	if len(binaries) == 0 {
		return nil
	}

	reporter.Log("   Chaining RPATHs for %d Mach-O file(s) with %d runtime dependency(ies)",
		len(binaries), len(entries))

	installNameTool, err := exec.LookPath("install_name_tool")
	if err != nil {
		reporter.Warn("   install_name_tool not found, skipping dylib RPATH chain")
		return nil
	}

	for _, binaryPath := range binaries {
		if err := a.addChainEntriesToMachO(installNameTool, binaryPath, entries, ctx, installPath, reporter); err != nil {
			return err
		}
	}

	return nil
}

// addChainEntriesToMachO computes one @loader_path-relative RPATH per chain
// entry for binaryPath, runs the defense-in-depth post-check, and applies
// them via install_name_tool -add_rpath. The post-check fails the install
// before the patch invocation if filepath.Join(loaderDir, relPath) escapes
// ctx.LibsDir.
//
// The loader dir used for path computation is the binary's eventual install
// location (rebased from ctx.WorkDir onto installPath), not its current
// location in ctx.WorkDir. install_binaries moves the bottle into installPath
// after this action runs; an RPATH computed from the WorkDir layout would
// point at a directory that doesn't exist once the binary lands in its
// final home (see futureLoaderDir for details).
func (a *HomebrewRelocateAction) addChainEntriesToMachO(
	installNameTool, binaryPath string,
	entries []chainEntry,
	ctx *ExecutionContext,
	installPath string,
	reporter progress.Reporter,
) error {
	loaderDir := futureLoaderDir(ctx.WorkDir, installPath, binaryPath)

	// Compute and validate every entry BEFORE any patching, so a single
	// escaping entry fails the install before any install_name_tool runs.
	rpaths, err := computeChainRpaths(loaderDir, ctx.LibsDir, entries, filepath.Base(binaryPath))
	if err != nil {
		return err
	}

	// Make file writable for the duration of the patch
	info, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", binaryPath, err)
	}
	originalMode := info.Mode()
	if originalMode&0200 == 0 {
		if err := os.Chmod(binaryPath, originalMode|0200); err != nil {
			return fmt.Errorf("failed to make %s writable: %w", binaryPath, err)
		}
		defer func() { _ = os.Chmod(binaryPath, originalMode) }()
	}

	for _, rp := range rpaths {
		cmd := exec.Command(installNameTool, "-add_rpath", rp, binaryPath)
		output, err := cmd.CombinedOutput()
		if err != nil && !strings.Contains(string(output), "would duplicate") {
			reporter.Warn("   failed to add RPATH %s to %s: %s",
				rp, filepath.Base(binaryPath), strings.TrimSpace(string(output)))
		}
	}

	// Re-sign the binary (required on macOS after modification, especially
	// on Apple Silicon).
	if codesign, err := exec.LookPath("codesign"); err == nil {
		signCmd := exec.Command(codesign, "-f", "-s", "-", binaryPath)
		_ = signCmd.Run() // Best effort
	}

	return nil
}

// fixElfRpathChain is the ELF mirror of fixDylibRpathChain. It walks
// ctx.WorkDir/{bin,lib} for ELF binaries and, for each one, appends one
// $ORIGIN-relative RPATH entry per declared runtime dependency (and per
// auto-included entry from the SONAME completeness scan), pointing at
// $TSUKU_HOME/libs/<dep>-<version>/lib. Author-declared entries appear
// first in the resulting RPATH; auto-included entries follow.
//
// Path form: relative to the binary's directory (loaderDir), computed via
// filepath.Rel after EvalSymlinks on both ends — the same machinery
// fixDylibRpathChain uses. Each computed entry is post-checked:
// filepath.Join(loaderDir, relPath) must resolve back inside ctx.LibsDir.
// A failed check fails the install with a clear error before any patchelf
// invocation for that entry.
//
// Patchelf primitive: --force-rpath --set-rpath '<colon-joined>' (writes
// DT_RPATH). --add-rpath would write DT_RUNPATH instead, but DT_RUNPATH
// has subtle resolution differences that break some shared-library
// lookups (e.g., wget's libunistring). Because --set-rpath replaces the
// entire RPATH rather than appending, this helper first reads the
// existing RPATH (set by the per-binary fixElfRpath pass that ran via
// relocatePlaceholders) and concatenates the new chain entries onto it.
//
// Order: this runs after relocatePlaceholders (which performs the
// per-binary fixElfRpath pass that wipes stale HOMEBREW rpaths and sets
// the $ORIGIN anchor), so the chain entries are appended to a clean
// baseline rather than racing the wipe.
func (a *HomebrewRelocateAction) fixElfRpathChain(ctx *ExecutionContext, installPath string, extra []chainEntry, reporter progress.Reporter) error {
	// Only run on Linux
	if runtime.GOOS != "linux" {
		return nil
	}

	entries := buildChainEntries(ctx, extra, reporter)

	if len(entries) == 0 {
		// No-op when no runtime dependencies were declared and the scanner
		// produced no auto-includes.
		return nil
	}

	// Collect ELF binaries to patch: shared libraries in lib/ (library and
	// tool recipes can both ship .so files) and executables in bin/ (tool
	// case). Mirrors the {bin,lib} walk fixDylibRpathChain performs.
	var binaries []string
	for _, sub := range []string{"lib", "bin"} {
		root := filepath.Join(ctx.WorkDir, sub)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			if a.isELFBinary(path) {
				binaries = append(binaries, path)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to walk %s directory: %w", sub, err)
		}
	}

	if len(binaries) == 0 {
		return nil
	}

	reporter.Log("   Chaining RPATHs for %d ELF file(s) with %d runtime dependency(ies)",
		len(binaries), len(entries))

	patchelfPath, err := a.findPatchelf(ctx)
	if err != nil {
		// Without patchelf the chain cannot be applied; surface a clear error
		// so the install fails loudly rather than producing under-linked
		// binaries that look fine until they run.
		return fmt.Errorf("ELF RPATH chain requires patchelf: %w", err)
	}

	for _, binaryPath := range binaries {
		if err := a.addChainEntriesToELF(patchelfPath, binaryPath, entries, ctx, installPath, reporter); err != nil {
			return err
		}
	}

	return nil
}

// addChainEntriesToELF computes one $ORIGIN-relative RPATH per chain entry
// for binaryPath, runs the defense-in-depth post-check, then appends those
// entries onto whatever RPATH patchelf currently reports for the binary
// and writes the combined string back via
// patchelf --force-rpath --set-rpath '<colon-joined>'.
//
// The append-not-replace shape preserves the per-binary $ORIGIN baseline
// installed by the earlier fixElfRpath pass (so a tool's bin/ binary that
// was set to $ORIGIN/../lib still finds its own bottle-shipped libs after
// the chain entries are added). Duplicate entries are collapsed.
//
// The post-check fails the install before the patch invocation if
// filepath.Join(loaderDir, relPath) escapes ctx.LibsDir. Same defense-in-
// depth contract as the macOS chain (see addChainEntriesToMachO).
//
// The loader dir used for path computation is the binary's eventual install
// location (rebased from ctx.WorkDir onto installPath), not its current
// location in ctx.WorkDir; install_binaries moves the bottle into
// installPath after this action runs. See futureLoaderDir for details.
func (a *HomebrewRelocateAction) addChainEntriesToELF(
	patchelfPath, binaryPath string,
	entries []chainEntry,
	ctx *ExecutionContext,
	installPath string,
	reporter progress.Reporter,
) error {
	loaderDir := futureLoaderDir(ctx.WorkDir, installPath, binaryPath)

	// Compute and validate every entry BEFORE any patching, so a single
	// escaping entry fails the install before patchelf runs.
	chainRpaths, err := computeChainRpathsWithPrefix(loaderDir, ctx.LibsDir, entries, filepath.Base(binaryPath), "$ORIGIN")
	if err != nil {
		return err
	}

	// Read the existing RPATH (set by the per-binary fixElfRpath pass).
	// patchelf --print-rpath returns an empty line if no rpath is set, which
	// is fine — the chain entries become the entire rpath in that case.
	printCmd := exec.Command(patchelfPath, "--print-rpath", binaryPath)
	printOutput, err := printCmd.Output()
	if err != nil {
		// --print-rpath fails on binaries patchelf can't parse (e.g., very
		// small static archives). Treat the existing rpath as empty and let
		// the --set-rpath below either succeed or fail with a clear error.
		printOutput = nil
	}
	existingRpath := strings.TrimSpace(string(printOutput))

	// Build the combined RPATH: existing entries first (so the per-binary
	// $ORIGIN baseline keeps priority), chain entries appended, dedup-aware.
	seen := make(map[string]bool)
	var combined []string
	if existingRpath != "" {
		for _, p := range strings.Split(existingRpath, ":") {
			p = strings.TrimSpace(p)
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			combined = append(combined, p)
		}
	}
	for _, p := range chainRpaths {
		if seen[p] {
			continue
		}
		seen[p] = true
		combined = append(combined, p)
	}

	if len(combined) == 0 {
		// Nothing to write (no existing rpath, no chain entries — this
		// should not happen because we early-returned on len(entries) == 0,
		// but defensively skip the patchelf call if it does).
		return nil
	}

	newRpath := strings.Join(combined, ":")

	// Make file writable for the duration of the patch
	info, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", binaryPath, err)
	}
	originalMode := info.Mode()
	if originalMode&0200 == 0 {
		if err := os.Chmod(binaryPath, originalMode|0200); err != nil {
			return fmt.Errorf("failed to make %s writable: %w", binaryPath, err)
		}
		defer func() { _ = os.Chmod(binaryPath, originalMode) }()
	}

	setCmd := exec.Command(patchelfPath, "--force-rpath", "--set-rpath", newRpath, binaryPath)
	if output, err := setCmd.CombinedOutput(); err != nil {
		// Some files in lib/ are static archives or non-dynamic objects that
		// patchelf rejects with "not an ELF executable" or "no dynamic
		// section". Skip those quietly; they have no rpath to chain into.
		out := string(output)
		if strings.Contains(out, "not an ELF") || strings.Contains(out, "no dynamic section") {
			reporter.Log("   skipping %s: %s", filepath.Base(binaryPath), strings.TrimSpace(out))
			return nil
		}
		return fmt.Errorf("patchelf --set-rpath failed for %s: %s: %w",
			filepath.Base(binaryPath), strings.TrimSpace(out), err)
	}

	return nil
}

// isELFBinary reports whether path looks like an ELF binary by reading its
// magic bytes (\x7fELF). Used to filter the chain walk to actual binaries
// (so a script or text file in bin/ doesn't fail patchelf).
func (a *HomebrewRelocateAction) isELFBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	magic := make([]byte, 4)
	if _, err := f.Read(magic); err != nil {
		return false
	}
	return bytes.Equal(magic, []byte{0x7f, 'E', 'L', 'F'})
}

// futureLoaderDir returns the directory the binary will occupy after the
// install completes — the same directory the runtime linker will use to
// resolve $ORIGIN / @loader_path at load time. This is NOT
// filepath.Dir(binaryPath): the bottle currently lives in ctx.WorkDir
// (e.g., /tmp/action-validator-XXX/bin/tmux) and only moves to
// $TSUKU_HOME/tools/<recipe>-<version>/bin/tmux when install_binaries
// runs, after the relocate phase.
//
// Computing the chain RPATH from the WorkDir location produces the wrong
// number of "..": the WorkDir-to-LibsDir hop and the install-location-to-
// LibsDir hop have different depths. The WorkDir-relative path "happens
// to work" inside /tmp because WorkDir and LibsDir are often siblings
// there, but it breaks once the binary lands in its real home and tries
// to resolve $ORIGIN / @loader_path against the install location.
//
// Rebasing strips ctx.WorkDir from the front of binaryPath (preserving
// any sub-directory like "bin/" or "lib/") and joins the remainder onto
// installPath, then takes the directory of the result. The rebased
// directory does not need to exist on disk yet — computeChainRpaths uses
// EvalSymlinks only on the LibsDir side (which does exist by the time
// the chain runs) and falls back to the raw path for the loader side.
//
// If binaryPath is not under ctx.WorkDir (defensive: should not happen in
// production because the chain walk lists binaries from inside WorkDir),
// the function falls back to filepath.Dir(binaryPath) so the install does
// not panic on an unexpected layout.
func futureLoaderDir(workDir, installPath, binaryPath string) string {
	rel, err := filepath.Rel(workDir, binaryPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.Dir(binaryPath)
	}
	return filepath.Dir(filepath.Join(installPath, rel))
}

// computeChainRpaths is the path-computation half of the dylib/ELF RPATH
// chain. It is split out from addChainEntriesToMachO (and the ELF
// counterpart) so the loader-relative computation and the defense-in-depth
// post-check are unit-testable without requiring Mach-O / ELF binaries or
// install_name_tool / patchelf.
//
// For each entry, it computes a relative path from loaderDir to
// $LibsDir/<name>-<version>/lib via filepath.Rel over EvalSymlinks on both
// ends, then verifies filepath.Join(loaderDir, relPath) resolves back
// inside libsDir. If any entry escapes libsDir, the function returns an
// error (and emits no RPATHs) so the caller can fail the install before
// patching anything.
//
// labelForError is the file-name used in the error message ("which binary
// did the bad entry come from?"); pass filepath.Base(binaryPath).
//
// Default prefix is "@loader_path" (macOS). Pass "$ORIGIN" via
// computeChainRpathsWithPrefix for the ELF chain.
func computeChainRpaths(loaderDir, libsDir string, entries []chainEntry, labelForError string) ([]string, error) {
	return computeChainRpathsWithPrefix(loaderDir, libsDir, entries, labelForError, "@loader_path")
}

// computeChainRpathsWithPrefix is the platform-parameterized form of
// computeChainRpaths. macOS calls in via the default-prefix wrapper above
// ("@loader_path"); Linux passes "$ORIGIN" so the same Rel-over-EvalSymlinks
// machinery and the same defense-in-depth post-check apply on both
// platforms. The prefix is purely cosmetic — patchelf and install_name_tool
// each have their own anchor token, but the relative path attached to the
// anchor is computed identically.
func computeChainRpathsWithPrefix(loaderDir, libsDir string, entries []chainEntry, labelForError, anchorPrefix string) ([]string, error) {
	loaderDirReal, err := filepath.EvalSymlinks(loaderDir)
	if err != nil {
		// EvalSymlinks fails if a path component doesn't exist; for the
		// loader dir (which contains the binary) this should not happen
		// in production, but in tests with synthetic dirs it's harmless
		// to fall back to the raw path.
		loaderDirReal = loaderDir
	}

	// libsDirClean is the post-check anchor: filepath.Join(loaderDir,
	// relPath) must resolve back inside libsDir. We compare in the
	// unresolved-path space (matching how the runtime linker will resolve
	// @loader_path relative to the binary's actual on-disk location)
	// rather than the EvalSymlinks-resolved space. Rel-over-EvalSymlinks
	// is what defends against symlink trickery in the underlying paths;
	// this post-check defends against filepath.Clean collapsing an
	// attacker-controlled name segment upward and out of the libs root.
	libsDirClean := filepath.Clean(libsDir)
	libsDirPrefix := libsDirClean + string(os.PathSeparator)

	rpaths := make([]string, 0, len(entries))
	for _, e := range entries {
		depLibDir := filepath.Join(libsDir, fmt.Sprintf("%s-%s", e.name, e.version), "lib")
		depLibDirReal, err := filepath.EvalSymlinks(depLibDir)
		if err != nil {
			// The dep may not yet be installed at relocate time on the
			// host; fall back to the constructed path. The post-check
			// below still verifies the join stays inside libs/.
			depLibDirReal = depLibDir
		}

		relPath, err := filepath.Rel(loaderDirReal, depLibDirReal)
		if err != nil {
			return nil, fmt.Errorf("failed to compute @loader_path-relative path from %s to %s: %w",
				loaderDirReal, depLibDirReal, err)
		}

		// Defense-in-depth: filepath.Join(loaderDir, relPath) must resolve
		// back inside libsDir. A failed check rejects the entry before
		// any install_name_tool / patchelf invocation.
		resolvedBack := filepath.Clean(filepath.Join(loaderDir, relPath))
		if !strings.HasPrefix(resolvedBack+string(os.PathSeparator), libsDirPrefix) && resolvedBack != libsDirClean {
			return nil, fmt.Errorf("rpath chain entry %q for %s escapes libs dir: resolved path %q is not inside %q",
				e.name, labelForError, resolvedBack, libsDirClean)
		}

		rpaths = append(rpaths, anchorPrefix+"/"+filepath.ToSlash(relPath))
	}

	return rpaths, nil
}

// isMachOBinary reports whether path looks like a Mach-O binary by reading
// its magic bytes. Used to filter the chain walk to actual binaries (so a
// dylib-named text file or stub doesn't fail install_name_tool).
func (a *HomebrewRelocateAction) isMachOBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	magic := make([]byte, 4)
	if _, err := f.Read(magic); err != nil {
		return false
	}

	return bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xce}) || // 32-bit big-endian
		bytes.Equal(magic, []byte{0xce, 0xfa, 0xed, 0xfe}) || // 32-bit little-endian
		bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xcf}) || // 64-bit big-endian
		bytes.Equal(magic, []byte{0xcf, 0xfa, 0xed, 0xfe}) || // 64-bit little-endian
		bytes.Equal(magic, []byte{0xca, 0xfe, 0xba, 0xbe}) || // Fat big-endian
		bytes.Equal(magic, []byte{0xbe, 0xba, 0xfe, 0xca}) // Fat little-endian
}

// fixLibraryInstallTimeChain is the install-time chain for Type == "library"
// recipes. It walks ctx.Dependencies.InstallTime (populated from the legacy
// metadata.dependencies field) and adds one @loader_path-relative RPATH
// entry per declared dep to each .dylib in the library's lib/ directory.
//
// Path form: relative to the dylib's directory (loaderDir), computed via
// filepath.Rel over EvalSymlinks on both ends — same machinery as
// fixDylibRpathChain (computeChainRpaths). Each computed entry is post-
// checked: filepath.Join(loaderDir, relPath) must resolve back inside
// ctx.LibsDir. A failed check fails the install with a clear error before
// any install_name_tool -add_rpath invocation for that entry.
//
// A trailing @loader_path entry is added so the dylib resolves sibling
// libraries shipped in the same library bottle (e.g., libevent's
// libevent_core.dylib references its own libevent.dylib via @rpath).
//
// This helper is kept separate from fixDylibRpathChain because the source
// field differs: this one reads InstallTime (legacy `dependencies` field),
// the other reads RuntimeDependencies (newer `runtime_dependencies` field).
// They merge once every library recipe has been migrated to declare its
// chain via runtime_dependencies — until then, both code paths are needed.
func (a *HomebrewRelocateAction) fixLibraryInstallTimeChain(ctx *ExecutionContext, installPath string, reporter progress.Reporter) error {
	// Only run on macOS
	if runtime.GOOS != "darwin" {
		return nil
	}

	// Find all .dylib files in the library
	libDir := filepath.Join(ctx.WorkDir, "lib")
	if _, err := os.Stat(libDir); os.IsNotExist(err) {
		// No lib directory, nothing to do
		return nil
	}

	var dylibFiles []string
	err := filepath.Walk(libDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if strings.HasSuffix(path, ".dylib") {
			dylibFiles = append(dylibFiles, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk lib directory: %w", err)
	}

	if len(dylibFiles) == 0 {
		return nil
	}

	// Build chain entries from InstallTime, sorted by name for deterministic
	// RPATH ordering. (Map iteration in Go is non-deterministic; without the
	// sort, the emitted RPATHs would shuffle across runs and golden-fixture
	// diffs would be noisy.)
	entries := make([]chainEntry, 0, len(ctx.Dependencies.InstallTime))
	for depName, depVersion := range ctx.Dependencies.InstallTime {
		entries = append(entries, chainEntry{name: depName, version: depVersion})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})

	if len(entries) == 0 {
		// No dependencies, nothing to add
		return nil
	}

	reporter.Log("   Fixing dylib RPATHs for %d .dylib file(s) with %d dependencies", len(dylibFiles), len(entries))

	installNameTool, err := exec.LookPath("install_name_tool")
	if err != nil {
		reporter.Warn("   install_name_tool not found, skipping dylib RPATH fixes")
		return nil
	}

	// For each .dylib file, compute the @loader_path-relative chain entries
	// and apply them. The path computation runs BEFORE any patching so a
	// single escaping entry fails the install before any install_name_tool
	// -add_rpath runs for that dylib.
	//
	// The loader dir used for the relative-path computation is the dylib's
	// eventual install location (rebased from ctx.WorkDir onto installPath),
	// not its current location in ctx.WorkDir. install_binaries moves the
	// bottle into installPath after this action runs; the WorkDir-relative
	// computation "happens to work" because both WorkDir and LibsDir are
	// commonly siblings under /tmp during install, but the relative path
	// it produces is wrong once the dylib lands in its real home.
	for _, dylibPath := range dylibFiles {
		loaderDir := futureLoaderDir(ctx.WorkDir, installPath, dylibPath)
		rpaths, err := computeChainRpaths(loaderDir, ctx.LibsDir, entries, filepath.Base(dylibPath))
		if err != nil {
			return err
		}

		// Make file writable for the duration of the patch
		info, err := os.Stat(dylibPath)
		if err != nil {
			continue
		}
		originalMode := info.Mode()
		if originalMode&0200 == 0 {
			if err := os.Chmod(dylibPath, originalMode|0200); err != nil {
				continue
			}
			defer func(p string, m os.FileMode) {
				_ = os.Chmod(p, m)
			}(dylibPath, originalMode)
		}

		// Add @loader_path-relative RPATH for each dependency
		for _, rp := range rpaths {
			rpathCmd := exec.Command(installNameTool, "-add_rpath", rp, dylibPath)
			output, err := rpathCmd.CombinedOutput()
			if err != nil && !strings.Contains(string(output), "would duplicate") {
				reporter.Warn("   failed to add RPATH %s to %s: %s",
					rp, filepath.Base(dylibPath), strings.TrimSpace(string(output)))
			}
		}

		// Also add @loader_path so the dylib finds sibling libs in the same
		// directory (e.g., libevent_core.dylib referencing libevent.dylib).
		loaderPathCmd := exec.Command(installNameTool, "-add_rpath", "@loader_path", dylibPath)
		output, err := loaderPathCmd.CombinedOutput()
		if err != nil && !strings.Contains(string(output), "would duplicate") {
			reporter.Warn("   failed to add @loader_path RPATH to %s: %s",
				filepath.Base(dylibPath), strings.TrimSpace(string(output)))
		}

		// Re-sign the binary (required on macOS after modification)
		if codesign, err := exec.LookPath("codesign"); err == nil {
			signCmd := exec.Command(codesign, "-f", "-s", "-", dylibPath)
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

// extractBottlePrefixes scans content for Homebrew bottle build paths and extracts them.
// Bottle paths follow the pattern: /tmp/action-validator-XXXXXXXX/.install/FORMULA/VERSION[/suffix]
// Returns a map from full path (including suffix) to the bottle prefix (up to VERSION).
// This allows the caller to replace only the prefix portion while preserving any suffix.
func (a *HomebrewRelocateAction) extractBottlePrefixes(content []byte, prefixMap map[string]string) {
	contentStr := string(content)

	// Look for /tmp/action-validator-XXXXXXXX/.install/FORMULA/VERSION patterns
	searchPos := 0
	marker := "/tmp/action-validator-"

	for {
		// Find next occurrence of /tmp/action-validator-
		idx := strings.Index(contentStr[searchPos:], marker)
		if idx == -1 {
			break
		}

		// Adjust to absolute position
		absIdx := searchPos + idx

		// Extract path starting from /tmp/action-validator-
		remaining := contentStr[absIdx:]

		// Find the end of the path (whitespace, quote, newline, etc.)
		endIdx := strings.IndexAny(remaining, " \t\n\r'\"<>;:|")
		if endIdx == -1 {
			endIdx = len(remaining)
		}

		fullPath := remaining[:endIdx]

		// Only process if it looks like a valid bottle path (contains /.install/)
		installIdx := strings.Index(fullPath, "/.install/")
		if installIdx == -1 {
			searchPos = absIdx + len(marker)
			continue
		}

		// Parse the path after /.install/ to find FORMULA/VERSION boundary
		// Format: /tmp/action-validator-XXX/.install/FORMULA/VERSION[/suffix]
		afterInstall := fullPath[installIdx+len("/.install/"):]
		parts := strings.SplitN(afterInstall, "/", 3) // formula, version, rest

		if len(parts) < 2 {
			// Not enough components (need at least formula/version)
			searchPos = absIdx + len(marker)
			continue
		}

		// Construct the prefix: everything up to and including version
		bottlePrefix := fullPath[:installIdx] + "/.install/" + parts[0] + "/" + parts[1]

		// Store mapping from full path to prefix
		prefixMap[fullPath] = bottlePrefix

		// Move search position past this occurrence
		searchPos = absIdx + len(marker)
	}
}

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
	installPath := ctx.ToolInstallDir
	if installPath == "" {
		installPath = ctx.InstallDir
	}

	fmt.Printf("   Relocating placeholders: %s\n", formula)

	// Relocate placeholders in files
	if err := a.relocatePlaceholders(ctx.WorkDir, installPath); err != nil {
		return fmt.Errorf("failed to relocate placeholders: %w", err)
	}

	fmt.Printf("   Relocation complete: %s\n", formula)

	return nil
}

// Note: homebrewPlaceholders is defined in homebrew.go

// relocatePlaceholders replaces Homebrew placeholders in all files
// For text files: direct replacement with install path
// For binary files: use patchelf/install_name_tool to reset RPATH
func (a *HomebrewRelocateAction) relocatePlaceholders(dir, installPath string) error {
	replacement := []byte(installPath)

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

		// Check if file contains any placeholder
		hasPlaceholder := false
		for _, placeholder := range homebrewPlaceholders {
			if bytes.Contains(content, placeholder) {
				hasPlaceholder = true
				break
			}
		}

		if !hasPlaceholder {
			return nil
		}

		// Determine if binary or text file
		isBinary := a.isBinaryFile(content)

		if isBinary {
			// Binary files: collect for RPATH fixup using patchelf/install_name_tool
			binariesToFix = append(binariesToFix, path)
		} else {
			// Text files: simple replacement with install path
			newContent := content
			for _, placeholder := range homebrewPlaceholders {
				newContent = bytes.ReplaceAll(newContent, placeholder, replacement)
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
		if err := a.fixBinaryRpath(binaryPath, installPath); err != nil {
			return fmt.Errorf("failed to fix RPATH for %s: %w", binaryPath, err)
		}
	}

	return nil
}

// fixBinaryRpath uses patchelf or install_name_tool to set a proper RPATH
// This replaces the Homebrew placeholder RPATH with a working path
func (a *HomebrewRelocateAction) fixBinaryRpath(binaryPath, installPath string) error {
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
		return a.fixElfRpath(binaryPath, installPath)
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
func (a *HomebrewRelocateAction) fixElfRpath(binaryPath, installPath string) error {
	patchelf, err := exec.LookPath("patchelf")
	if err != nil {
		// patchelf not available - try to proceed without it
		// The binary may still work if its dependencies are system libraries
		fmt.Printf("   Warning: patchelf not found, skipping RPATH fix for %s\n", filepath.Base(binaryPath))
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

	// Remove existing RPATH first (contains placeholders)
	removeCmd := exec.Command(patchelf, "--remove-rpath", binaryPath)
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

	setCmd := exec.Command(patchelf, "--force-rpath", "--set-rpath", newRpath, binaryPath)
	if output, err := setCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("patchelf --set-rpath failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	// Fix the ELF interpreter if it contains Homebrew placeholders
	// Homebrew bottles on Linux have interpreter set to @@HOMEBREW_PREFIX@@/lib/ld.so
	// which needs to be changed to the system loader
	if err := a.fixElfInterpreter(patchelf, binaryPath); err != nil {
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

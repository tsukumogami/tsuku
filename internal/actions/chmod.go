package actions

import (
	"fmt"
	"os"
	"path/filepath"
)

// ChmodAction implements making files executable
type ChmodAction struct{ BaseAction }

// Preflight validates parameters without side effects.
func (a *ChmodAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	// ERROR: Missing files parameter
	files, hasFiles := GetStringSlice(params, "files")
	if !hasFiles {
		result.AddError("chmod action requires 'files' parameter")
	} else if len(files) == 0 {
		result.AddError("chmod action requires at least one file in 'files' array")
	}

	// WARNING: Overly permissive mode
	if modeStr, hasMode := GetString(params, "mode"); hasMode {
		var mode int
		if _, err := fmt.Sscanf(modeStr, "%o", &mode); err == nil {
			// Check for world-writable bits (o+w = 002)
			if mode&0002 != 0 {
				result.AddWarning("mode grants world-write permissions; consider more restrictive permissions")
			}
		}
	}

	return result
}

// IsDeterministic returns true because chmod produces identical results.
func (ChmodAction) IsDeterministic() bool { return true }

// Name returns the action name
func (a *ChmodAction) Name() string {
	return "chmod"
}

// Execute makes files executable
//
// Parameters:
//   - files (required): List of files to make executable
//   - mode (optional): File mode (default: "0755")
func (a *ChmodAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get files list (required)
	files, ok := GetStringSlice(params, "files")
	if !ok {
		return fmt.Errorf("chmod action requires 'files' parameter")
	}

	// Get mode (defaults to 0755)
	modeStr, _ := GetString(params, "mode")
	if modeStr == "" {
		modeStr = "0755"
	}

	// Parse mode (supports octal strings like "0755")
	var mode os.FileMode
	if _, err := fmt.Sscanf(modeStr, "%o", &mode); err != nil {
		return fmt.Errorf("invalid mode: %s", modeStr)
	}

	// Build vars for variable substitution
	vars := GetStandardVars(ctx.Version, ctx.InstallDir, ctx.WorkDir, ctx.LibsDir)

	fmt.Printf("   Making executable: %v\n", files)

	for _, file := range files {
		file = ExpandVars(file, vars)
		filePath := filepath.Join(ctx.WorkDir, file)

		if err := os.Chmod(filePath, mode); err != nil {
			return fmt.Errorf("failed to chmod %s: %w", file, err)
		}
	}

	fmt.Printf("   ✓ Made %d file(s) executable\n", len(files))
	return nil
}

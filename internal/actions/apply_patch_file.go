package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ApplyPatchFileAction applies a patch using the system patch command.
// This is a primitive action that works with local files or inline data.
// For URL-based patches with checksum validation, use the apply_patch composite.
type ApplyPatchFileAction struct{}

// Name returns the action name.
func (a *ApplyPatchFileAction) Name() string {
	return "apply_patch_file"
}

// Execute applies a patch to the source code.
//
// Parameters:
//   - file (optional): Path to local patch file (mutually exclusive with data)
//   - data (optional): Inline patch content (mutually exclusive with file)
//   - strip (optional): Strip level for patch -p flag (default: 1)
//   - subdir (optional): Subdirectory to apply patch in (relative to work dir)
func (a *ApplyPatchFileAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	file, _ := GetString(params, "file")
	data, _ := GetString(params, "data")

	// Must have either file or data
	if file == "" && data == "" {
		return fmt.Errorf("apply_patch_file: either 'file' or 'data' parameter is required")
	}
	if file != "" && data != "" {
		return fmt.Errorf("apply_patch_file: cannot specify both 'file' and 'data'")
	}

	// Get strip level (default 1)
	strip := 1
	if s, ok := params["strip"].(int); ok {
		strip = s
	} else if s, ok := params["strip"].(float64); ok {
		strip = int(s)
	}

	// Get subdir (optional)
	subdir, _ := GetString(params, "subdir")
	if subdir != "" {
		// Security: disallow path traversal
		if strings.Contains(subdir, "..") || filepath.IsAbs(subdir) {
			return fmt.Errorf("apply_patch_file: invalid subdir path '%s'", subdir)
		}
	}

	// Determine patch content
	var patchContent string
	if file != "" {
		// Read patch from file
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("apply_patch_file: failed to read patch file: %w", err)
		}
		patchContent = string(content)
		ctx.Log().Debug("apply_patch_file: read patch from file", "file", file)
	} else {
		patchContent = data
		ctx.Log().Debug("apply_patch_file: using inline patch data")
	}

	// Apply patch
	workDir := ctx.WorkDir
	if subdir != "" {
		workDir = filepath.Join(ctx.WorkDir, subdir)
		// Verify subdir exists
		if _, err := os.Stat(workDir); os.IsNotExist(err) {
			return fmt.Errorf("apply_patch_file: subdir '%s' does not exist", subdir)
		}
	}

	if err := applyPatchContent(workDir, patchContent, strip); err != nil {
		return fmt.Errorf("apply_patch_file: failed to apply patch: %w", err)
	}

	ctx.Log().Debug("apply_patch_file: successfully applied patch", "strip", strip)
	return nil
}

// applyPatchContent applies patch content using the patch command.
func applyPatchContent(workDir, patchContent string, strip int) error {
	// Check if patch command is available
	patchPath, err := exec.LookPath("patch")
	if err != nil {
		return fmt.Errorf("patch command not found: please install patch utility")
	}

	// Create patch command
	cmd := exec.Command(patchPath, "-p", fmt.Sprintf("%d", strip), "--batch")
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(patchContent)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("patch failed: %v\noutput: %s", err, string(output))
	}

	return nil
}

func init() {
	Register(&ApplyPatchFileAction{})
}

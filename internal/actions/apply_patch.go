package actions

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ApplyPatchAction applies a patch file using the system patch command.
// Supports both URL-based patches and inline patch data.
type ApplyPatchAction struct{}

// Name returns the action name.
func (a *ApplyPatchAction) Name() string {
	return "apply_patch"
}

// Execute applies a patch to the source code.
//
// Parameters:
//   - url (optional): URL to download patch from (mutually exclusive with data)
//   - data (optional): Inline patch content (mutually exclusive with url)
//   - strip (optional): Strip level for patch -p flag (default: 1)
//   - subdir (optional): Subdirectory to apply patch in (relative to work dir)
func (a *ApplyPatchAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	url, _ := GetString(params, "url")
	data, _ := GetString(params, "data")

	// Must have either URL or data
	if url == "" && data == "" {
		return fmt.Errorf("apply_patch: either 'url' or 'data' parameter is required")
	}
	if url != "" && data != "" {
		return fmt.Errorf("apply_patch: cannot specify both 'url' and 'data'")
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
			return fmt.Errorf("apply_patch: invalid subdir path '%s'", subdir)
		}
	}

	// Determine patch content
	var patchContent string
	if url != "" {
		// Download patch from URL
		if !strings.HasPrefix(url, "https://") {
			return fmt.Errorf("apply_patch: url must use https")
		}

		content, err := downloadPatch(url)
		if err != nil {
			return fmt.Errorf("apply_patch: failed to download patch: %w", err)
		}
		patchContent = content
		ctx.Log().Debug("apply_patch: downloaded patch from URL", "url", url)
	} else {
		patchContent = data
		ctx.Log().Debug("apply_patch: using inline patch data")
	}

	// Apply patch
	workDir := ctx.WorkDir
	if subdir != "" {
		workDir = filepath.Join(ctx.WorkDir, subdir)
		// Verify subdir exists
		if _, err := os.Stat(workDir); os.IsNotExist(err) {
			return fmt.Errorf("apply_patch: subdir '%s' does not exist", subdir)
		}
	}

	if err := applyPatch(workDir, patchContent, strip); err != nil {
		return fmt.Errorf("apply_patch: failed to apply patch: %w", err)
	}

	ctx.Log().Debug("apply_patch: successfully applied patch", "strip", strip)
	return nil
}

// downloadPatch downloads patch content from a URL.
func downloadPatch(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// applyPatch applies patch content using the patch command.
func applyPatch(workDir, patchContent string, strip int) error {
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
	Register(&ApplyPatchAction{})
}

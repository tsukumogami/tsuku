package actions

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

// ApplyPatchAction applies a patch file using the system patch command.
// This is a composite action that decomposes into download + apply_patch_file
// for URL-based patches (with checksum validation), or directly to apply_patch_file
// for inline patch data.
type ApplyPatchAction struct{}

// Name returns the action name.
func (a *ApplyPatchAction) Name() string {
	return "apply_patch"
}

// Execute applies a patch to the source code.
// This method provides backwards compatibility for direct execution.
// For plan generation with checksum validation, use Decompose instead.
//
// Parameters:
//   - url (optional): URL to download patch from (mutually exclusive with data)
//   - data (optional): Inline patch content (mutually exclusive with url)
//   - sha256 (optional): Expected SHA256 checksum for URL patches
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

	// Get common parameters
	strip := 1
	if s, ok := params["strip"].(int); ok {
		strip = s
	} else if s, ok := params["strip"].(float64); ok {
		strip = int(s)
	}
	subdir, _ := GetString(params, "subdir")

	if url != "" {
		// URL-based patch: download then apply
		if !strings.HasPrefix(url, "https://") {
			return fmt.Errorf("apply_patch: url must use https")
		}

		// Download patch
		content, err := downloadPatch(url)
		if err != nil {
			return fmt.Errorf("apply_patch: failed to download patch: %w", err)
		}

		// Verify checksum if provided
		sha256, hasSHA := GetString(params, "sha256")
		if hasSHA {
			actualChecksum := computeSHA256String(content)
			if actualChecksum != sha256 {
				return fmt.Errorf("apply_patch: checksum mismatch: expected %s, got %s", sha256, actualChecksum)
			}
			ctx.Log().Debug("apply_patch: checksum verified", "sha256", sha256)
		}

		// Delegate to apply_patch_file
		fileAction := &ApplyPatchFileAction{}
		return fileAction.Execute(ctx, map[string]interface{}{
			"data":   content,
			"strip":  strip,
			"subdir": subdir,
		})
	}

	// Inline data: delegate directly to apply_patch_file
	fileAction := &ApplyPatchFileAction{}
	return fileAction.Execute(ctx, map[string]interface{}{
		"data":   data,
		"strip":  strip,
		"subdir": subdir,
	})
}

// Decompose returns the primitive steps for apply_patch action.
// For URL-based patches, this decomposes to download + apply_patch_file,
// enabling checksum validation through the download action.
// For inline data, this returns a single apply_patch_file step.
func (a *ApplyPatchAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
	url, _ := GetString(params, "url")
	data, _ := GetString(params, "data")

	// Validate parameters
	if url == "" && data == "" {
		return nil, fmt.Errorf("either 'url' or 'data' parameter is required")
	}
	if url != "" && data != "" {
		return nil, fmt.Errorf("cannot specify both 'url' and 'data'")
	}

	// Get common parameters
	strip := 1
	if s, ok := params["strip"].(int); ok {
		strip = s
	} else if s, ok := params["strip"].(float64); ok {
		strip = int(s)
	}
	subdir, _ := GetString(params, "subdir")

	// Validate subdir if present
	if subdir != "" {
		if strings.Contains(subdir, "..") || filepath.IsAbs(subdir) {
			return nil, fmt.Errorf("invalid subdir path '%s'", subdir)
		}
	}

	if url != "" {
		// URL-based patch: decompose to download + apply_patch_file
		if !strings.HasPrefix(url, "https://") {
			return nil, fmt.Errorf("url must use https")
		}

		// Generate a unique filename for the patch
		patchFilename := "patch.diff"
		if idx := strings.LastIndex(url, "/"); idx >= 0 {
			patchFilename = url[idx+1:]
		}

		// Compute checksum if Downloader is available
		var checksum string
		var size int64
		sha256Param, hasSHA := GetString(params, "sha256")

		if hasSHA {
			// Use provided checksum
			checksum = sha256Param
		} else if ctx.Downloader != nil {
			// Download to compute checksum
			result, err := ctx.Downloader.Download(ctx.Context, url)
			if err != nil {
				return nil, fmt.Errorf("failed to download patch for checksum computation: %w", err)
			}
			checksum = result.Checksum
			size = result.Size
		}

		// Build steps: download patch file, then apply it
		steps := []Step{
			{
				Action: "download",
				Params: map[string]interface{}{
					"url":  url,
					"dest": patchFilename,
				},
				Checksum: checksum,
				Size:     size,
			},
			{
				Action: "apply_patch_file",
				Params: map[string]interface{}{
					"file":   patchFilename,
					"strip":  strip,
					"subdir": subdir,
				},
			},
		}

		return steps, nil
	}

	// Inline data: single apply_patch_file step
	steps := []Step{
		{
			Action: "apply_patch_file",
			Params: map[string]interface{}{
				"data":   data,
				"strip":  strip,
				"subdir": subdir,
			},
		},
	}

	return steps, nil
}

// computeSHA256String computes SHA256 hash of a string.
func computeSHA256String(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
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

func init() {
	Register(&ApplyPatchAction{})
}

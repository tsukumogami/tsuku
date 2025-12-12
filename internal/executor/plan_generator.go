package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/actions"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/validate"
	"github.com/tsukumogami/tsuku/internal/version"
)

// PlanConfig configures plan generation behavior.
type PlanConfig struct {
	// OS overrides the target operating system (default: runtime.GOOS)
	OS string
	// Arch overrides the target architecture (default: runtime.GOARCH)
	Arch string
	// RecipeSource indicates where the recipe came from ("registry" or file path)
	RecipeSource string
	// OnWarning is called when a non-evaluable step is encountered
	OnWarning func(action string, message string)
	// Downloader is used for checksum computation (if nil, a default is created)
	Downloader *validate.PreDownloader
}

// GeneratePlan evaluates a recipe and produces an installation plan.
// The plan captures fully-resolved URLs, computed checksums, and all steps
// needed to reproduce the installation.
func (e *Executor) GeneratePlan(ctx context.Context, cfg PlanConfig) (*InstallationPlan, error) {
	// Apply defaults
	targetOS := cfg.OS
	if targetOS == "" {
		targetOS = runtime.GOOS
	}
	targetArch := cfg.Arch
	if targetArch == "" {
		targetArch = runtime.GOARCH
	}
	recipeSource := cfg.RecipeSource
	if recipeSource == "" {
		recipeSource = "unknown"
	}

	// Create version resolver
	resolver := version.New()

	// Resolve version from recipe
	versionInfo, err := e.resolveVersionWith(ctx, resolver)
	if err != nil {
		return nil, fmt.Errorf("version resolution failed: %w", err)
	}

	// Store version for later use
	e.version = versionInfo.Version

	// Compute recipe hash
	recipeHash, err := computeRecipeHash(e.recipe)
	if err != nil {
		return nil, fmt.Errorf("failed to compute recipe hash: %w", err)
	}

	// Create downloader for checksum computation
	downloader := cfg.Downloader
	if downloader == nil {
		downloader = validate.NewPreDownloader()
	}

	// Build variable map for template expansion
	vars := map[string]string{
		"version":     versionInfo.Version,
		"version_tag": versionInfo.Tag,
		"os":          targetOS,
		"arch":        targetArch,
	}

	// Process each step
	var steps []ResolvedStep
	for _, step := range e.recipe.Steps {
		// Check conditional execution against target platform
		if !shouldExecuteForPlatform(step.When, targetOS, targetArch) {
			continue
		}

		// Resolve the step
		resolved, err := e.resolveStep(ctx, step, vars, downloader, cfg.OnWarning)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve step %s: %w", step.Action, err)
		}

		steps = append(steps, *resolved)
	}

	return &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          e.recipe.Metadata.Name,
		Version:       versionInfo.Version,
		Platform: Platform{
			OS:   targetOS,
			Arch: targetArch,
		},
		GeneratedAt:  time.Now().UTC(),
		RecipeHash:   recipeHash,
		RecipeSource: recipeSource,
		Steps:        steps,
	}, nil
}

// computeRecipeHash computes SHA256 hash of the recipe's TOML content.
func computeRecipeHash(r interface{ ToTOML() ([]byte, error) }) (string, error) {
	data, err := r.ToTOML()
	if err != nil {
		return "", fmt.Errorf("failed to serialize recipe: %w", err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// shouldExecuteForPlatform checks if a step should execute for the given platform.
func shouldExecuteForPlatform(when map[string]string, targetOS, targetArch string) bool {
	if len(when) == 0 {
		return true
	}

	// Check OS condition
	if osCondition, ok := when["os"]; ok {
		if osCondition != targetOS {
			return false
		}
	}

	// Check arch condition
	if archCondition, ok := when["arch"]; ok {
		if archCondition != targetArch {
			return false
		}
	}

	// Check package_manager condition (always true for plan generation)
	// Package manager conditions are runtime checks, not plan-time checks

	return true
}

// resolveStep resolves a single recipe step into a ResolvedStep.
func (e *Executor) resolveStep(
	ctx context.Context,
	step recipe.Step,
	vars map[string]string,
	downloader *validate.PreDownloader,
	onWarning func(string, string),
) (*ResolvedStep, error) {
	// Check evaluability
	evaluable := IsActionEvaluable(step.Action)

	// Emit warning for non-evaluable actions
	if !evaluable && onWarning != nil {
		onWarning(step.Action, fmt.Sprintf("action '%s' cannot be deterministically reproduced", step.Action))
	}

	// Expand templates in all string parameters
	expandedParams := expandParams(step.Params, vars)

	// Create resolved step
	resolved := &ResolvedStep{
		Action:    step.Action,
		Params:    expandedParams,
		Evaluable: evaluable,
	}

	// For download actions, compute checksum
	if isDownloadAction(step.Action) {
		url, err := extractDownloadURL(step.Action, expandedParams, vars)
		if err != nil {
			return nil, fmt.Errorf("failed to extract download URL: %w", err)
		}

		if url != "" {
			resolved.URL = url

			// Download to compute checksum
			result, err := downloader.Download(ctx, url)
			if err != nil {
				return nil, fmt.Errorf("failed to download for checksum: %w", err)
			}
			defer func() { _ = result.Cleanup() }()

			resolved.Checksum = result.Checksum
			resolved.Size = result.Size
		}
	}

	return resolved, nil
}

// isDownloadAction returns true if the action involves downloading files.
func isDownloadAction(action string) bool {
	switch action {
	case "download", "download_archive", "github_archive", "github_file", "hashicorp_release", "homebrew_bottle":
		return true
	default:
		return false
	}
}

// extractDownloadURL extracts the download URL from action parameters.
func extractDownloadURL(action string, params map[string]interface{}, vars map[string]string) (string, error) {
	switch action {
	case "download", "download_archive":
		// Direct URL in params
		url, ok := params["url"].(string)
		if !ok {
			return "", fmt.Errorf("missing 'url' parameter")
		}
		return url, nil

	case "github_archive", "github_file":
		// Construct URL from repo and asset_pattern or file
		repo, ok := params["repo"].(string)
		if !ok {
			return "", fmt.Errorf("missing 'repo' parameter")
		}

		// Get version from vars
		ver := vars["version"]

		// Determine asset name
		var assetName string
		if pattern, ok := params["asset_pattern"].(string); ok {
			assetName = pattern
		} else if file, ok := params["file"].(string); ok {
			assetName = file
		} else {
			return "", fmt.Errorf("missing 'asset_pattern' or 'file' parameter")
		}

		// Build GitHub release download URL
		// Format: https://github.com/{repo}/releases/download/{tag}/{asset}
		url := fmt.Sprintf("https://github.com/%s/releases/download/v%s/%s", repo, ver, assetName)
		return url, nil

	case "hashicorp_release":
		// Construct HashiCorp release URL
		product, ok := params["product"].(string)
		if !ok {
			return "", fmt.Errorf("missing 'product' parameter")
		}

		ver := vars["version"]
		os := vars["os"]
		arch := vars["arch"]

		// Format: https://releases.hashicorp.com/{product}/{version}/{product}_{version}_{os}_{arch}.zip
		url := fmt.Sprintf("https://releases.hashicorp.com/%s/%s/%s_%s_%s_%s.zip",
			product, ver, product, ver, os, arch)
		return url, nil

	case "homebrew_bottle":
		// Homebrew bottle URLs are complex and depend on formula
		// For now, return empty to skip checksum (bottles often have upstream checksums)
		return "", nil

	default:
		return "", nil
	}
}

// expandParams recursively expands template variables in parameters.
func expandParams(params map[string]interface{}, vars map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range params {
		result[k] = expandValue(v, vars)
	}
	return result
}

// expandValue expands template variables in a value.
func expandValue(v interface{}, vars map[string]string) interface{} {
	switch val := v.(type) {
	case string:
		return expandVarsInString(val, vars)
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = expandValue(item, vars)
		}
		return result
	case map[string]interface{}:
		return expandParams(val, vars)
	default:
		return v
	}
}

// expandVarsInString replaces {var} placeholders in a string.
func expandVarsInString(s string, vars map[string]string) string {
	result := s
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
	}
	return result
}

// GetStandardPlanVars returns the standard variable map for plan generation.
// This can be used by callers to understand what variables are available.
func GetStandardPlanVars(version, versionTag, os, arch string) map[string]string {
	return map[string]string{
		"version":     version,
		"version_tag": versionTag,
		"os":          os,
		"arch":        arch,
	}
}

// ApplyOSMapping applies OS mapping from params to the vars map.
func ApplyOSMapping(vars map[string]string, params map[string]interface{}) {
	if osMapping, ok := params["os_mapping"].(map[string]interface{}); ok {
		if mappedOS, ok := osMapping[vars["os"]].(string); ok {
			vars["os"] = mappedOS
		}
	}
}

// ApplyArchMapping applies arch mapping from params to the vars map.
func ApplyArchMapping(vars map[string]string, params map[string]interface{}) {
	if archMapping, ok := params["arch_mapping"].(map[string]interface{}); ok {
		if mappedArch, ok := archMapping[vars["arch"]].(string); ok {
			vars["arch"] = mappedArch
		}
	}
}

// Ensure actions package is imported for compatibility
var _ = actions.GetStandardVars

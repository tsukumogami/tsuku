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
	"github.com/tsukumogami/tsuku/internal/platform"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/version"
)

// PlanConfig configures plan generation behavior.
type PlanConfig struct {
	// OS overrides the target operating system (default: runtime.GOOS)
	OS string
	// Arch overrides the target architecture (default: runtime.GOARCH)
	Arch string
	// LinuxFamily specifies the target Linux distribution family (debian, rhel, arch, alpine, suse).
	// Only used when OS is "linux". If empty on Linux, DetectFamily() is called.
	LinuxFamily string
	// RecipeSource indicates where the recipe came from ("registry" or file path)
	RecipeSource string
	// OnWarning is called when a non-evaluable step is encountered
	OnWarning func(action string, message string)
	// Downloader is used for checksum computation during plan generation.
	// Required: callers must provide a downloader that implements actions.Downloader.
	// Use validate.NewPreDownloaderAdapter(validate.NewPreDownloader()) to create one.
	Downloader actions.Downloader
	// DownloadCache is used to cache downloaded files for later use in container validation.
	// If nil, downloads are not cached.
	DownloadCache *actions.DownloadCache
	// AutoAcceptEvalDeps controls whether eval-time dependencies are installed automatically.
	// When true, missing deps are installed without prompting (equivalent to --install-deps flag).
	AutoAcceptEvalDeps bool
	// OnEvalDepsNeeded is called when eval-time dependencies are missing.
	// The callback receives the list of missing dependencies and the auto-accept flag.
	// It should install the dependencies and return nil on success.
	// If nil and deps are missing, plan generation fails with an error.
	OnEvalDepsNeeded func(deps []string, autoAccept bool) error
	// RecipeLoader loads recipes for dependency resolution.
	// If nil, plans will not include dependency installation steps.
	// When set, plans become self-contained by including steps for all dependencies.
	RecipeLoader actions.RecipeLoader
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

	// Detect linux_family if on Linux and not provided
	linuxFamily := cfg.LinuxFamily
	if targetOS == "linux" && linuxFamily == "" {
		detectedFamily, err := platform.DetectFamily()
		if err != nil {
			// Don't fail plan generation if family detection fails
			// Just log a warning and continue without family filtering
			if cfg.OnWarning != nil {
				cfg.OnWarning("platform", fmt.Sprintf("linux_family detection failed: %v", err))
			}
		} else {
			linuxFamily = detectedFamily
		}
	}

	// Construct target for step filtering
	// This target is used to filter steps by both explicit When clauses and implicit action constraints
	target := platform.Target{
		Platform:    targetOS + "/" + targetArch,
		LinuxFamily: linuxFamily,
	}

	// Create version resolver
	resolver := version.New()

	// Resolve version from recipe
	versionInfo, err := e.resolveVersionWith(ctx, resolver)
	if err != nil {
		// Fall back to "dev" version for recipes without proper version sources
		// This matches the behavior in Execute() for backward compatibility
		if cfg.OnWarning != nil {
			cfg.OnWarning("version", fmt.Sprintf("version resolution failed: %v, using 'dev'", err))
		}
		versionInfo = &version.VersionInfo{
			Version: "dev",
			Tag:     "dev",
		}
	}

	// Store version for later use
	e.version = versionInfo.Version

	// Compute recipe hash
	recipeHash, err := computeRecipeHash(e.recipe)
	if err != nil {
		return nil, fmt.Errorf("failed to compute recipe hash: %w", err)
	}

	// Get downloader for checksum computation
	// Callers must provide a Downloader; if nil, no checksums will be computed
	downloader := cfg.Downloader

	// Build variable map for template expansion
	vars := map[string]string{
		"version":     versionInfo.Version,
		"version_tag": versionInfo.Tag,
		"os":          targetOS,
		"arch":        targetArch,
	}

	// Create EvalContext for decomposition
	evalCtx := &actions.EvalContext{
		Context:       ctx,
		Version:       versionInfo.Version,
		VersionTag:    versionInfo.Tag,
		OS:            targetOS,
		Arch:          targetArch,
		Recipe:        e.recipe,
		Resolver:      resolver,
		Downloader:    downloader,
		DownloadCache: cfg.DownloadCache,
	}

	// Process each step
	var steps []ResolvedStep
	for _, step := range e.recipe.Steps {
		// Filter steps by target platform and linux_family
		// This implements two-stage filtering:
		// 1. Check action's implicit constraint (e.g., apt_install only on debian family)
		// 2. Check step's explicit When clause (e.g., when = { os = "linux" })

		// Stage 1: Check action's implicit constraint against target
		action := actions.Get(step.Action)
		if sysAction, ok := action.(actions.SystemAction); ok {
			if constraint := sysAction.ImplicitConstraint(); constraint != nil {
				if !constraint.MatchesTarget(target) {
					continue // Skip: action doesn't match target (e.g., apt on rhel)
				}
			}
		}
		// Actions without implicit constraint (non-SystemAction) pass stage 1

		// Stage 2: Check explicit When clause against target platform
		if step.When != nil && !step.When.Matches(target.OS(), target.Arch()) {
			continue // Skip: explicit condition not met
		}
		// Steps without When clause pass stage 2

		// Resolve the step (handles decomposition of composites)
		resolvedSteps, err := e.resolveStep(ctx, step, vars, downloader, cfg, evalCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve step %s: %w", step.Action, err)
		}

		steps = append(steps, resolvedSteps...)
	}

	// Convert patches to apply_patch steps and insert after extraction
	if len(e.recipe.Patches) > 0 {
		steps = insertPatchSteps(steps, e.recipe.Patches)
	}

	// Generate nested dependency plans
	// This makes plans self-contained - they include all steps needed to install
	// the tool and its dependencies. Dependencies form a tree structure.
	var dependencies []DependencyPlan
	if cfg.RecipeLoader != nil {
		processed := make(map[string]bool)
		processed[e.recipe.Metadata.Name] = true // Avoid cycles back to root
		deps, err := generateDependencyPlans(ctx, e.recipe, cfg, processed)
		if err != nil {
			return nil, fmt.Errorf("failed to generate dependency plans: %w", err)
		}
		dependencies = deps
	}

	// Compute plan-level deterministic flag: true only if ALL steps are deterministic
	// (including all dependency steps recursively)
	planDeterministic := computeDeterministic(steps, dependencies)

	// Capture verify section from recipe for plan execution
	var verify *PlanVerify
	if e.recipe.Verify.Command != "" {
		verify = &PlanVerify{
			Command: e.recipe.Verify.Command,
			Pattern: e.recipe.Verify.Pattern,
		}
	}

	return &InstallationPlan{
		FormatVersion: PlanFormatVersion,
		Tool:          e.recipe.Metadata.Name,
		Version:       versionInfo.Version,
		Platform: Platform{
			OS:   targetOS,
			Arch: targetArch,
		},
		GeneratedAt:   time.Now().UTC(),
		RecipeHash:    recipeHash,
		RecipeSource:  recipeSource,
		Deterministic: planDeterministic,
		Dependencies:  dependencies,
		Steps:         steps,
		Verify:        verify,
		RecipeType:    string(e.recipe.Metadata.Type),
	}, nil
}

// computeDeterministic checks if all steps (including nested dependencies) are deterministic.
func computeDeterministic(steps []ResolvedStep, deps []DependencyPlan) bool {
	for _, step := range steps {
		if !step.Deterministic {
			return false
		}
	}
	for _, dep := range deps {
		if !computeDeterministic(dep.Steps, dep.Dependencies) {
			return false
		}
	}
	return true
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
// Package manager conditions are runtime checks and are ignored during plan generation.
func shouldExecuteForPlatform(when *recipe.WhenClause, targetOS, targetArch string) bool {
	if when == nil || when.IsEmpty() {
		return true
	}

	// Use WhenClause.Matches() for platform/OS filtering
	// Package manager check is ignored at plan-time (runtime concern only)
	return when.Matches(targetOS, targetArch)
}

// resolveStep resolves a single recipe step into one or more ResolvedSteps.
// For composite actions, it decomposes them into primitive steps.
func (e *Executor) resolveStep(
	ctx context.Context,
	step recipe.Step,
	vars map[string]string,
	downloader actions.Downloader,
	cfg PlanConfig,
	evalCtx *actions.EvalContext,
) ([]ResolvedStep, error) {
	// Check if this is a decomposable action
	if actions.IsDecomposable(step.Action) {
		// Check eval-time dependencies before decomposition
		if evalDeps := actions.GetEvalDeps(step.Action); len(evalDeps) > 0 {
			missing := actions.CheckEvalDeps(evalDeps)
			if len(missing) > 0 {
				if cfg.OnEvalDepsNeeded != nil {
					if err := cfg.OnEvalDepsNeeded(missing, cfg.AutoAcceptEvalDeps); err != nil {
						return nil, fmt.Errorf("eval-time dependencies not satisfied: %w", err)
					}
				} else {
					return nil, fmt.Errorf("missing eval-time dependencies: %v (install with: tsuku install %s)", missing, missing[0])
				}
			}
		}

		// For decomposable actions, pass raw params - the Decompose method
		// handles template expansion with proper os_mapping/arch_mapping support.
		// Expanding here would bake in raw GOOS/GOARCH values before mappings apply.
		primitiveSteps, err := actions.DecomposeToPrimitives(evalCtx, step.Action, step.Params)
		if err != nil {
			return nil, fmt.Errorf("failed to decompose %s: %w", step.Action, err)
		}

		// Convert primitive steps to ResolvedSteps
		var resolved []ResolvedStep
		for _, pstep := range primitiveSteps {
			evaluable := IsActionEvaluable(pstep.Action)
			deterministic := actions.IsDeterministic(pstep.Action)

			rs := ResolvedStep{
				Action:        pstep.Action,
				Params:        pstep.Params,
				Evaluable:     evaluable,
				Deterministic: deterministic,
			}

			// For download actions, cache the file for offline container execution.
			// If Decompose already provided a checksum, it verified the download.
			// Skip re-downloading for URLs that require special auth (e.g., GHCR).
			// Handle both legacy "download" and new "download_file" actions.
			if pstep.Action == "download" || pstep.Action == "download_file" {
				if url, ok := pstep.Params["url"].(string); ok {
					rs.URL = url

					if pstep.Checksum != "" {
						// Checksum provided by Decompose - it already verified the download
						// This handles URLs requiring special auth (e.g., GHCR)
						rs.Checksum = pstep.Checksum
						rs.Size = pstep.Size
					} else if downloader != nil {
						// No checksum provided, need to download to compute it
						result, err := downloader.Download(ctx, url)
						if err != nil {
							return nil, fmt.Errorf("failed to download for caching: %w", err)
						}
						// Save to cache if configured
						if evalCtx.DownloadCache != nil {
							_ = evalCtx.DownloadCache.Save(url, result.AssetPath, result.Checksum)
						}
						rs.Checksum = result.Checksum
						rs.Size = result.Size
						_ = result.Cleanup()
					}
				}
			} else if pstep.Checksum != "" {
				// Non-download action with checksum from decomposition
				rs.Checksum = pstep.Checksum
				rs.Size = pstep.Size
				if url, ok := pstep.Params["url"].(string); ok {
					rs.URL = url
				}
			}

			resolved = append(resolved, rs)
		}
		return resolved, nil
	}

	// Non-decomposable action: apply mappings first, then expand params
	// Create a copy of vars to apply os_mapping and arch_mapping without mutating the original
	mappedVars := make(map[string]string)
	for k, v := range vars {
		mappedVars[k] = v
	}
	ApplyOSMapping(mappedVars, step.Params)
	ApplyArchMapping(mappedVars, step.Params)
	expandedParams := expandParams(step.Params, mappedVars)
	evaluable := IsActionEvaluable(step.Action)
	deterministic := actions.IsDeterministic(step.Action)

	// Emit warning for non-evaluable actions
	if !evaluable && cfg.OnWarning != nil {
		cfg.OnWarning(step.Action, fmt.Sprintf("action '%s' cannot be deterministically reproduced", step.Action))
	}

	// Create resolved step
	resolved := ResolvedStep{
		Action:        step.Action,
		Params:        expandedParams,
		Evaluable:     evaluable,
		Deterministic: deterministic,
	}

	// For download actions, extract checksum from params or compute via download
	// Always download when a downloader is available to cache the file for offline execution
	if isDownloadAction(step.Action) {
		url, err := extractDownloadURL(step.Action, expandedParams, mappedVars)
		if err != nil {
			return nil, fmt.Errorf("failed to extract download URL: %w", err)
		}

		if url != "" {
			resolved.URL = url

			// Check if checksum is provided in params (e.g., "sha256:...")
			checksumParam, hasChecksumParam := expandedParams["checksum"].(string)
			if hasChecksumParam && checksumParam != "" {
				resolved.Checksum = checksumParam
			}

			// Always download to cache the file for offline execution (e.g., sandbox mode)
			// Even if checksum is provided in params, we need the file cached
			if downloader != nil {
				result, err := downloader.Download(ctx, url)
				if err != nil {
					return nil, fmt.Errorf("failed to download for caching: %w", err)
				}

				// Save to cache if configured
				if evalCtx != nil && evalCtx.DownloadCache != nil {
					checksum := resolved.Checksum
					if checksum == "" {
						checksum = result.Checksum
					}
					if err := evalCtx.DownloadCache.Save(url, result.AssetPath, checksum); err != nil {
						return nil, fmt.Errorf("failed to save to cache: %w", err)
					}
				}

				// Use computed checksum/size if not provided in params
				if resolved.Checksum == "" {
					resolved.Checksum = result.Checksum
				}
				if resolved.Size == 0 {
					resolved.Size = result.Size
				}

				_ = result.Cleanup()
			}
		}
	}

	return []ResolvedStep{resolved}, nil
}

// isDownloadAction returns true if the action involves downloading files.
func isDownloadAction(action string) bool {
	switch action {
	case "download", "download_archive", "github_archive", "github_file", "homebrew":
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

	case "homebrew":
		// Homebrew URLs are complex and depend on formula
		// For now, return empty to skip checksum (bottles have upstream checksums)
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

// insertPatchSteps converts recipe patches to apply_patch steps and inserts them
// after the last extraction step. This ensures patches are applied after source
// extraction but before build steps.
func insertPatchSteps(steps []ResolvedStep, patches []recipe.Patch) []ResolvedStep {
	// Find the position after the last "extract" step
	insertIdx := 0
	for i, step := range steps {
		if step.Action == "extract" {
			insertIdx = i + 1
		}
	}

	// If no extract step found, insert at the beginning (unusual case)
	// Patches typically need extracted source to apply to

	// Convert patches to resolved steps
	patchSteps := make([]ResolvedStep, 0, len(patches))
	for _, patch := range patches {
		params := make(map[string]interface{})

		if patch.URL != "" {
			params["url"] = patch.URL
		}
		if patch.Data != "" {
			params["data"] = patch.Data
		}
		if patch.Checksum != "" {
			params["sha256"] = patch.Checksum
		}
		if patch.Strip != 0 {
			params["strip"] = patch.Strip
		}
		if patch.Subdir != "" {
			params["subdir"] = patch.Subdir
		}

		patchSteps = append(patchSteps, ResolvedStep{
			Action:        "apply_patch",
			Params:        params,
			Evaluable:     true,                                   // apply_patch is evaluable
			Deterministic: actions.IsDeterministic("apply_patch"), // Should be true
		})
	}

	// Insert patch steps at the found position
	result := make([]ResolvedStep, 0, len(steps)+len(patchSteps))
	result = append(result, steps[:insertIdx]...)
	result = append(result, patchSteps...)
	result = append(result, steps[insertIdx:]...)

	return result
}

// generateDependencyPlans generates nested dependency plans for all install-time dependencies.
// Each dependency is represented as a DependencyPlan with its own nested dependencies,
// forming a tree structure. This allows the executor to skip already-installed dependencies.
func generateDependencyPlans(
	ctx context.Context,
	r *recipe.Recipe,
	cfg PlanConfig,
	processed map[string]bool,
) ([]DependencyPlan, error) {
	// Resolve direct dependencies from recipe
	deps := actions.ResolveDependencies(r)

	if len(deps.InstallTime) == 0 {
		return nil, nil
	}

	var plans []DependencyPlan

	// Process dependencies in deterministic order
	depNames := make([]string, 0, len(deps.InstallTime))
	for name := range deps.InstallTime {
		depNames = append(depNames, name)
	}
	sortStrings(depNames)

	for _, depName := range depNames {
		// Skip if already processed (handles cycles and de-duplication)
		if processed[depName] {
			continue
		}
		processed[depName] = true

		depPlan, err := generateSingleDependencyPlan(ctx, depName, cfg, processed)
		if err != nil {
			return nil, fmt.Errorf("failed to generate plan for dependency %s: %w", depName, err)
		}
		if depPlan != nil {
			plans = append(plans, *depPlan)
		}
	}

	return plans, nil
}

// generateSingleDependencyPlan generates a DependencyPlan for a single dependency,
// including its own nested dependencies recursively.
func generateSingleDependencyPlan(
	ctx context.Context,
	depName string,
	cfg PlanConfig,
	processed map[string]bool,
) (*DependencyPlan, error) {
	// Load the dependency recipe
	depRecipe, err := cfg.RecipeLoader.GetWithContext(ctx, depName)
	if err != nil {
		// Dependency recipe not found - skip
		// This could be a system dependency or something not in the registry
		return nil, nil
	}

	// Recursively generate plans for this dependency's own dependencies
	nestedDeps, err := generateDependencyPlans(ctx, depRecipe, cfg, processed)
	if err != nil {
		return nil, err
	}

	// Generate the plan for this dependency (without further recursion)
	exec, err := New(depRecipe)
	if err != nil {
		return nil, fmt.Errorf("failed to create executor for %s: %w", depName, err)
	}
	defer exec.Cleanup()

	// Generate plan without RecipeLoader to get just this tool's steps
	depCfg := PlanConfig{
		OS:                 cfg.OS,
		Arch:               cfg.Arch,
		RecipeSource:       "dependency",
		OnWarning:          cfg.OnWarning,
		Downloader:         cfg.Downloader,
		DownloadCache:      cfg.DownloadCache,
		AutoAcceptEvalDeps: cfg.AutoAcceptEvalDeps,
		OnEvalDepsNeeded:   cfg.OnEvalDepsNeeded,
		RecipeLoader:       nil, // Don't recurse here - we handle it above
	}

	plan, err := exec.GeneratePlan(ctx, depCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to generate plan for %s: %w", depName, err)
	}

	// Build verify info if present
	var verify *PlanVerify
	if depRecipe.Verify.Command != "" {
		verify = &PlanVerify{
			Command: depRecipe.Verify.Command,
			Pattern: depRecipe.Verify.Pattern,
		}
	}

	return &DependencyPlan{
		Tool:         depName,
		Version:      plan.Version,
		RecipeHash:   plan.RecipeHash,
		Dependencies: nestedDeps,
		Steps:        plan.Steps,
		Verify:       verify,
		RecipeType:   plan.RecipeType,
	}, nil
}

// sortStrings sorts a slice of strings in place.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

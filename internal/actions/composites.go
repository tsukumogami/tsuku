package actions

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/version"
)

// Ensure GitHubArchiveAction implements Decomposable
var _ Decomposable = (*GitHubArchiveAction)(nil)

// DownloadArchiveAction downloads, extracts, and installs binaries from an archive
// This is a generic composite action for any URL
type DownloadArchiveAction struct{ BaseAction }

// IsDeterministic returns true because download_archive decomposes to only deterministic primitives.
func (DownloadArchiveAction) IsDeterministic() bool { return true }

func (a *DownloadArchiveAction) Name() string { return "download_archive" }

func (a *DownloadArchiveAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Extract parameters
	url, ok := GetString(params, "url")
	if !ok {
		return fmt.Errorf("url is required")
	}

	archiveFormat, ok := GetString(params, "archive_format")
	if !ok {
		return fmt.Errorf("archive_format is required")
	}

	binariesRaw, ok := params["binaries"]
	if !ok {
		return fmt.Errorf("binaries is required")
	}

	stripDirs, _ := GetInt(params, "strip_dirs") // Defaults to 0 if not present

	// Parse install_mode parameter (optional, for verification enforcement)
	installMode, _ := GetString(params, "install_mode")
	if installMode == "" {
		installMode = "binaries" // Default mode
	}
	installMode = strings.ToLower(installMode) // Normalize to lowercase

	// Enforce verification for directory-based installs
	// Libraries are exempt since they cannot be run directly to verify
	verifyCmd := strings.TrimSpace(ctx.Recipe.Verify.Command)
	isLibrary := ctx.Recipe.Metadata.Type == "library"
	if (installMode == "directory" || installMode == "directory_wrapped") && verifyCmd == "" && !isLibrary {
		return fmt.Errorf("recipes with install_mode='%s' must include a [verify] section with a command to ensure the installation works correctly", installMode)
	}

	// Build variable map for template expansion
	vars := map[string]string{
		"version":     ctx.Version,
		"version_tag": ctx.VersionTag,
		"os":          ctx.OS,
		"arch":        ctx.Arch,
	}

	// Apply OS mapping if present
	if osMapping, ok := params["os_mapping"].(map[string]interface{}); ok {
		if mappedOS, ok := osMapping[ctx.OS].(string); ok {
			vars["os"] = mappedOS
		}
	}

	// Apply arch mapping if present
	if archMapping, ok := params["arch_mapping"].(map[string]interface{}); ok {
		if mappedArch, ok := archMapping[ctx.Arch].(string); ok {
			vars["arch"] = mappedArch
		}
	}

	// Expand URL template
	downloadURL := ExpandVars(url, vars)

	// Extract filename from URL (last component)
	// For nodejs: node-v{version}-{os}-{arch}.tar.gz
	archiveFilename := ExpandVars(archiveFormat, vars) // Use format as filename hint
	// Better: extract from URL path
	// Simple approach: use last part of URL
	parts := []rune(downloadURL)
	lastSlash := -1
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == '/' {
			lastSlash = i
			break
		}
	}
	if lastSlash >= 0 && lastSlash < len(downloadURL)-1 {
		archiveFilename = downloadURL[lastSlash+1:]
	}

	// Step 1: Download archive
	downloadParams := map[string]interface{}{
		"url":  downloadURL,
		"dest": archiveFilename,
	}

	downloadAction := &DownloadAction{}
	if err := downloadAction.Execute(ctx, downloadParams); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Step 2: Extract archive
	extractParams := map[string]interface{}{
		"archive":    archiveFilename,
		"format":     archiveFormat,
		"strip_dirs": stripDirs,
	}

	extractAction := &ExtractAction{}
	if err := extractAction.Execute(ctx, extractParams); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}

	// Step 3: Copy entire extracted directory to install directory
	// For archives like Node.js, we need the whole directory structure (lib/, bin/, etc.)
	// not just individual binaries
	if err := CopyDirectory(ctx.WorkDir, ctx.InstallDir); err != nil {
		return fmt.Errorf("failed to copy extracted content: %w", err)
	}

	// Step 4: Chmod binaries to ensure they're executable
	// Chmod needs to happen in the install directory since we already copied there
	// Temporarily update context to point to install dir for chmod
	installCtx := *ctx
	installCtx.WorkDir = ctx.InstallDir

	chmodFiles := extractSourceFiles(binariesRaw)
	chmodAction := &ChmodAction{}
	chmodParams := map[string]interface{}{
		"files": chmodFiles,
	}

	if err := chmodAction.Execute(&installCtx, chmodParams); err != nil {
		return fmt.Errorf("chmod failed: %w", err)
	}

	binDir := filepath.Join(ctx.InstallDir, "bin")

	fmt.Printf("   ✓ Installed complete directory structure\n")
	fmt.Printf("   ✓ Verified %d executable(s) in %s\n", len(chmodFiles), binDir)

	return nil
}

// Decompose returns the primitive steps for download_archive action.
func (a *DownloadArchiveAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
	// Extract parameters
	url, ok := GetString(params, "url")
	if !ok {
		return nil, fmt.Errorf("url is required")
	}

	archiveFormat, ok := GetString(params, "archive_format")
	if !ok {
		return nil, fmt.Errorf("archive_format is required")
	}

	binariesRaw, ok := params["binaries"]
	if !ok {
		return nil, fmt.Errorf("binaries is required")
	}

	stripDirs, _ := GetInt(params, "strip_dirs")

	installMode, _ := GetString(params, "install_mode")
	if installMode == "" {
		installMode = "binaries"
	}
	installMode = strings.ToLower(installMode)

	// Validate install_mode
	if installMode != "binaries" && installMode != "directory" && installMode != "directory_wrapped" {
		return nil, fmt.Errorf("invalid install_mode '%s': must be 'binaries', 'directory', or 'directory_wrapped'", installMode)
	}

	// Build variable map for template expansion
	vars := map[string]string{
		"version":     ctx.Version,
		"version_tag": ctx.VersionTag,
		"os":          ctx.OS,
		"arch":        ctx.Arch,
	}

	// Apply OS mapping if present
	if osMapping, ok := params["os_mapping"].(map[string]interface{}); ok {
		if mappedOS, ok := osMapping[ctx.OS].(string); ok {
			vars["os"] = mappedOS
		}
	}

	// Apply arch mapping if present
	if archMapping, ok := params["arch_mapping"].(map[string]interface{}); ok {
		if mappedArch, ok := archMapping[ctx.Arch].(string); ok {
			vars["arch"] = mappedArch
		}
	}

	// Expand URL template
	downloadURL := ExpandVars(url, vars)

	// Extract filename from URL
	archiveFilename := ExpandVars(archiveFormat, vars)
	parts := []rune(downloadURL)
	lastSlash := -1
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == '/' {
			lastSlash = i
			break
		}
	}
	if lastSlash >= 0 && lastSlash < len(downloadURL)-1 {
		archiveFilename = downloadURL[lastSlash+1:]
	}

	// Extract chmod files
	chmodFiles := extractSourceFiles(binariesRaw)

	// Download to compute checksum if Downloader is available
	var checksum string
	var size int64

	if ctx.Downloader != nil {
		result, err := ctx.Downloader.Download(ctx.Context, downloadURL)
		if err != nil {
			return nil, fmt.Errorf("failed to download for checksum computation: %w", err)
		}
		checksum = result.Checksum
		size = result.Size
		// Save to cache if configured, then cleanup temp file
		if ctx.DownloadCache != nil {
			_ = ctx.DownloadCache.Save(downloadURL, result.AssetPath, result.Checksum)
		}
		_ = result.Cleanup()
	}

	// Build primitive steps
	steps := []Step{
		{
			Action: "download_file",
			Params: map[string]interface{}{
				"url":      downloadURL,
				"dest":     archiveFilename,
				"checksum": checksum,
			},
			Checksum: checksum,
			Size:     size,
		},
		{
			Action: "extract",
			Params: map[string]interface{}{
				"archive":    archiveFilename,
				"format":     archiveFormat,
				"strip_dirs": stripDirs,
			},
		},
		{
			Action: "chmod",
			Params: map[string]interface{}{
				"files": chmodFiles,
			},
		},
		{
			Action: "install_binaries",
			Params: map[string]interface{}{
				"binaries":     binariesRaw,
				"install_mode": installMode,
			},
		},
	}

	return steps, nil
}

// GitHubArchiveAction downloads and extracts archives from GitHub releases
type GitHubArchiveAction struct{ BaseAction }

// IsDeterministic returns true because github_archive decomposes to only deterministic primitives.
func (GitHubArchiveAction) IsDeterministic() bool { return true }

func (a *GitHubArchiveAction) Name() string { return "github_archive" }

func (a *GitHubArchiveAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Extract parameters
	repo, ok := GetString(params, "repo")
	if !ok {
		return fmt.Errorf("repo is required")
	}

	assetPattern, ok := GetString(params, "asset_pattern")
	if !ok {
		return fmt.Errorf("asset_pattern is required")
	}

	archiveFormat, ok := GetString(params, "archive_format")
	if !ok {
		return fmt.Errorf("archive_format is required")
	}

	stripDirs, _ := GetInt(params, "strip_dirs") // Defaults to 0 if not present

	binariesRaw, ok := params["binaries"]
	if !ok {
		return fmt.Errorf("binaries is required")
	}

	// Parse install_mode parameter (default: "binaries")
	installMode, _ := GetString(params, "install_mode")
	if installMode == "" {
		installMode = "binaries" // Default mode
	}
	installMode = strings.ToLower(installMode) // Normalize to lowercase

	// Validate install_mode
	if installMode != "binaries" && installMode != "directory" && installMode != "directory_wrapped" {
		return fmt.Errorf("invalid install_mode '%s': must be 'binaries', 'directory', or 'directory_wrapped'", installMode)
	}

	// Enforce verification for directory-based installs
	// Libraries are exempt since they cannot be run directly to verify
	verifyCmd := strings.TrimSpace(ctx.Recipe.Verify.Command)
	isLibrary := ctx.Recipe.Metadata.Type == "library"
	if (installMode == "directory" || installMode == "directory_wrapped") && verifyCmd == "" && !isLibrary {
		return fmt.Errorf("recipes with install_mode='%s' must include a [verify] section with a command to ensure the installation works correctly", installMode)
	}

	// Build GitHub release URL
	vars := map[string]string{
		"version": ctx.Version,
		"os":      ctx.OS,
		"arch":    ctx.Arch,
	}

	// Apply OS mapping if present
	if osMapping, ok := params["os_mapping"].(map[string]interface{}); ok {
		if mappedOS, ok := osMapping[ctx.OS].(string); ok {
			vars["os"] = mappedOS
		}
	}

	// Apply arch mapping if present
	if archMapping, ok := params["arch_mapping"].(map[string]interface{}); ok {
		if mappedArch, ok := archMapping[ctx.Arch].(string); ok {
			vars["arch"] = mappedArch
		}
	}

	assetName := ExpandVars(assetPattern, vars)

	// Check if pattern contains wildcards - if so, resolve using GitHub API
	if version.ContainsWildcards(assetName) {
		if ctx.Resolver == nil {
			return fmt.Errorf("resolver not available in context (required for wildcard patterns)")
		}

		// Fetch assets from GitHub API (use ExecutionContext's context for proper cancellation)
		apiCtx, cancel := context.WithTimeout(ctx.Context, 30*time.Second)
		defer cancel()

		assets, err := ctx.Resolver.FetchReleaseAssets(apiCtx, repo, ctx.VersionTag)
		if err != nil {
			return fmt.Errorf("failed to fetch release assets: %w", err)
		}

		// Match pattern against assets
		matchedAsset, err := version.MatchAssetPattern(assetName, assets)
		if err != nil {
			return fmt.Errorf("asset pattern matching failed: %w", err)
		}

		assetName = matchedAsset
		fmt.Printf("   → Resolved wildcard pattern to: %s\n", assetName)
	}

	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, ctx.VersionTag, assetName)

	// Step 1: Download archive
	downloadParams := map[string]interface{}{
		"url":  url,
		"dest": assetName,
	}

	downloadAction := &DownloadAction{}
	if err := downloadAction.Execute(ctx, downloadParams); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Step 2: Extract archive
	extractParams := map[string]interface{}{
		"archive":    assetName,
		"format":     archiveFormat,
		"strip_dirs": stripDirs,
	}

	extractAction := &ExtractAction{}
	if err := extractAction.Execute(ctx, extractParams); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}

	// Step 3: Chmod binaries
	// Extract source files for chmod (binaries can be ["file"] or [{src: "file", dest: "..."}])
	chmodFiles := extractSourceFiles(binariesRaw)
	chmodAction := &ChmodAction{}
	chmodParams := map[string]interface{}{
		"files": chmodFiles,
	}

	if err := chmodAction.Execute(ctx, chmodParams); err != nil {
		return fmt.Errorf("chmod failed: %w", err)
	}

	// Step 4: Install binaries
	installAction := &InstallBinariesAction{}
	installParams := map[string]interface{}{
		"binaries":     binariesRaw, // Pass raw []interface{} from TOML
		"install_mode": installMode, // Pass install_mode to install_binaries action
	}

	if err := installAction.Execute(ctx, installParams); err != nil {
		return fmt.Errorf("install failed: %w", err)
	}

	return nil
}

// Decompose resolves the GitHub release asset and returns primitive steps.
// This enables deterministic plan generation by performing API calls and
// checksum computation at evaluation time rather than execution time.
func (a *GitHubArchiveAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
	// Extract required parameters
	repo, ok := GetString(params, "repo")
	if !ok {
		return nil, fmt.Errorf("repo is required")
	}

	assetPattern, ok := GetString(params, "asset_pattern")
	if !ok {
		return nil, fmt.Errorf("asset_pattern is required")
	}

	archiveFormat, ok := GetString(params, "archive_format")
	if !ok {
		return nil, fmt.Errorf("archive_format is required")
	}

	binariesRaw, ok := params["binaries"]
	if !ok {
		return nil, fmt.Errorf("binaries is required")
	}

	stripDirs, _ := GetInt(params, "strip_dirs") // Defaults to 0 if not present

	// Parse install_mode parameter (default: "binaries")
	installMode, _ := GetString(params, "install_mode")
	if installMode == "" {
		installMode = "binaries"
	}
	installMode = strings.ToLower(installMode)

	// Validate install_mode
	if installMode != "binaries" && installMode != "directory" && installMode != "directory_wrapped" {
		return nil, fmt.Errorf("invalid install_mode '%s': must be 'binaries', 'directory', or 'directory_wrapped'", installMode)
	}

	// Build variable map for template expansion
	vars := map[string]string{
		"version": ctx.Version,
		"os":      ctx.OS,
		"arch":    ctx.Arch,
	}

	// Apply OS mapping if present
	if osMapping, ok := params["os_mapping"].(map[string]interface{}); ok {
		if mappedOS, ok := osMapping[ctx.OS].(string); ok {
			vars["os"] = mappedOS
		}
	}

	// Apply arch mapping if present
	if archMapping, ok := params["arch_mapping"].(map[string]interface{}); ok {
		if mappedArch, ok := archMapping[ctx.Arch].(string); ok {
			vars["arch"] = mappedArch
		}
	}

	assetName := ExpandVars(assetPattern, vars)

	// Check if pattern contains wildcards - if so, resolve using GitHub API
	if version.ContainsWildcards(assetName) {
		if ctx.Resolver == nil {
			return nil, fmt.Errorf("resolver not available in context (required for wildcard patterns)")
		}

		// Fetch assets from GitHub API
		apiCtx, cancel := context.WithTimeout(ctx.Context, 30*time.Second)
		defer cancel()

		assets, err := ctx.Resolver.FetchReleaseAssets(apiCtx, repo, ctx.VersionTag)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch release assets: %w", err)
		}

		// Match pattern against assets
		matchedAsset, err := version.MatchAssetPattern(assetName, assets)
		if err != nil {
			return nil, fmt.Errorf("asset pattern matching failed: %w", err)
		}

		assetName = matchedAsset
	}

	// Construct the download URL
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, ctx.VersionTag, assetName)

	// Download the file to compute checksum
	var checksum string
	var size int64

	if ctx.Downloader != nil {
		result, err := ctx.Downloader.Download(ctx.Context, url)
		if err != nil {
			return nil, fmt.Errorf("failed to download for checksum computation: %w", err)
		}
		checksum = result.Checksum
		size = result.Size
		// Save to cache if configured, then cleanup temp file
		if ctx.DownloadCache != nil {
			_ = ctx.DownloadCache.Save(url, result.AssetPath, result.Checksum)
		}
		_ = result.Cleanup()
	}

	// Extract source files for chmod (binaries can be ["file"] or [{src: "file", dest: "..."}])
	chmodFiles := extractSourceFiles(binariesRaw)

	// Return primitive steps
	return []Step{
		{
			Action: "download_file",
			Params: map[string]interface{}{
				"url":      url,
				"dest":     assetName,
				"checksum": checksum,
			},
			Checksum: checksum,
			Size:     size,
		},
		{
			Action: "extract",
			Params: map[string]interface{}{
				"archive":    assetName,
				"format":     archiveFormat,
				"strip_dirs": stripDirs,
			},
		},
		{
			Action: "chmod",
			Params: map[string]interface{}{
				"files": chmodFiles,
			},
		},
		{
			Action: "install_binaries",
			Params: map[string]interface{}{
				"binaries":     binariesRaw,
				"install_mode": installMode,
			},
		},
	}, nil
}

// GitHubFileAction downloads pre-compiled binary files from GitHub releases
type GitHubFileAction struct{ BaseAction }

// IsDeterministic returns true because github_file decomposes to only deterministic primitives.
func (GitHubFileAction) IsDeterministic() bool { return true }

func (a *GitHubFileAction) Name() string { return "github_file" }

func (a *GitHubFileAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Extract parameters
	repo, ok := GetString(params, "repo")
	if !ok {
		return fmt.Errorf("repo is required")
	}

	assetPattern, ok := GetString(params, "asset_pattern")
	if !ok {
		return fmt.Errorf("asset_pattern is required")
	}

	// Support both 'binary' (backward compat) and 'binaries' (new format)
	var binaries interface{}
	var downloadName string

	if binariesParam, ok := params["binaries"]; ok {
		// New format: binaries array with src/dest
		binaries = binariesParam
		// Extract download name from first binary's src
		if arr, ok := binariesParam.([]interface{}); ok && len(arr) > 0 {
			if m, ok := arr[0].(map[string]interface{}); ok {
				if src, ok := m["src"].(string); ok {
					downloadName = src
				}
			}
		}
		if downloadName == "" {
			return fmt.Errorf("binaries[0].src is required for download")
		}
	} else if binary, ok := GetString(params, "binary"); ok {
		// Old format: single binary string
		downloadName = binary
		binaries = []interface{}{binary}
	} else {
		return fmt.Errorf("either 'binary' or 'binaries' is required")
	}

	// Build GitHub release URL
	vars := map[string]string{
		"version": ctx.Version,
		"os":      ctx.OS,
		"arch":    ctx.Arch,
	}

	// Apply OS mapping if present
	if osMapping, ok := params["os_mapping"].(map[string]interface{}); ok {
		if mappedOS, ok := osMapping[ctx.OS].(string); ok {
			vars["os"] = mappedOS
		}
	}

	// Apply arch mapping if present
	if archMapping, ok := params["arch_mapping"].(map[string]interface{}); ok {
		if mappedArch, ok := archMapping[ctx.Arch].(string); ok {
			vars["arch"] = mappedArch
		}
	}

	assetName := ExpandVars(assetPattern, vars)

	// Check if pattern contains wildcards - if so, resolve using GitHub API
	if version.ContainsWildcards(assetName) {
		if ctx.Resolver == nil {
			return fmt.Errorf("resolver not available in context (required for wildcard patterns)")
		}

		// Fetch assets from GitHub API (use ExecutionContext's context for proper cancellation)
		apiCtx, cancel := context.WithTimeout(ctx.Context, 30*time.Second)
		defer cancel()

		assets, err := ctx.Resolver.FetchReleaseAssets(apiCtx, repo, ctx.VersionTag)
		if err != nil {
			return fmt.Errorf("failed to fetch release assets: %w", err)
		}

		// Match pattern against assets
		matchedAsset, err := version.MatchAssetPattern(assetName, assets)
		if err != nil {
			return fmt.Errorf("asset pattern matching failed: %w", err)
		}

		assetName = matchedAsset
		fmt.Printf("   → Resolved wildcard pattern to: %s\n", assetName)
	}

	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, ctx.VersionTag, assetName)

	// Step 1: Download binary
	downloadParams := map[string]interface{}{
		"url":  url,
		"dest": ExpandVars(downloadName, vars),
	}

	downloadAction := &DownloadAction{}
	if err := downloadAction.Execute(ctx, downloadParams); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Step 2: Chmod binary
	chmodAction := &ChmodAction{}
	chmodParams := map[string]interface{}{
		"files": []string{ExpandVars(downloadName, vars)},
	}

	if err := chmodAction.Execute(ctx, chmodParams); err != nil {
		return fmt.Errorf("chmod failed: %w", err)
	}

	// Step 3: Install binary
	installAction := &InstallBinariesAction{}
	installParams := map[string]interface{}{
		"binaries": binaries,
	}

	if err := installAction.Execute(ctx, installParams); err != nil {
		return fmt.Errorf("install failed: %w", err)
	}

	return nil
}

// Decompose returns the primitive steps for github_file action.
func (a *GitHubFileAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
	// Extract parameters
	repo, ok := GetString(params, "repo")
	if !ok {
		return nil, fmt.Errorf("repo is required")
	}

	assetPattern, ok := GetString(params, "asset_pattern")
	if !ok {
		return nil, fmt.Errorf("asset_pattern is required")
	}

	// Support both 'binary' (backward compat) and 'binaries' (new format)
	var binaries interface{}
	var downloadName string

	if binariesParam, ok := params["binaries"]; ok {
		binaries = binariesParam
		if arr, ok := binariesParam.([]interface{}); ok && len(arr) > 0 {
			if m, ok := arr[0].(map[string]interface{}); ok {
				if src, ok := m["src"].(string); ok {
					downloadName = src
				}
			}
		}
		if downloadName == "" {
			return nil, fmt.Errorf("binaries[0].src is required for download")
		}
	} else if binary, ok := GetString(params, "binary"); ok {
		downloadName = binary
		binaries = []interface{}{binary}
	} else {
		return nil, fmt.Errorf("either 'binary' or 'binaries' is required")
	}

	// Build variable map
	vars := map[string]string{
		"version": ctx.Version,
		"os":      ctx.OS,
		"arch":    ctx.Arch,
	}

	// Apply OS mapping if present
	if osMapping, ok := params["os_mapping"].(map[string]interface{}); ok {
		if mappedOS, ok := osMapping[ctx.OS].(string); ok {
			vars["os"] = mappedOS
		}
	}

	// Apply arch mapping if present
	if archMapping, ok := params["arch_mapping"].(map[string]interface{}); ok {
		if mappedArch, ok := archMapping[ctx.Arch].(string); ok {
			vars["arch"] = mappedArch
		}
	}

	assetName := ExpandVars(assetPattern, vars)

	// Check if pattern contains wildcards - if so, resolve using GitHub API
	if version.ContainsWildcards(assetName) {
		if ctx.Resolver == nil {
			return nil, fmt.Errorf("resolver not available in context (required for wildcard patterns)")
		}

		// Fetch assets from GitHub API
		apiCtx, cancel := context.WithTimeout(ctx.Context, 30*time.Second)
		defer cancel()

		assets, err := ctx.Resolver.FetchReleaseAssets(apiCtx, repo, ctx.VersionTag)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch release assets: %w", err)
		}

		// Match pattern against assets
		matchedAsset, err := version.MatchAssetPattern(assetName, assets)
		if err != nil {
			return nil, fmt.Errorf("asset pattern matching failed: %w", err)
		}

		assetName = matchedAsset
	}

	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, ctx.VersionTag, assetName)
	expandedDownloadName := ExpandVars(downloadName, vars)

	// Download to compute checksum if Downloader is available
	var checksum string
	var size int64

	if ctx.Downloader != nil {
		result, err := ctx.Downloader.Download(ctx.Context, url)
		if err != nil {
			return nil, fmt.Errorf("failed to download for checksum computation: %w", err)
		}
		checksum = result.Checksum
		size = result.Size
		// Save to cache if configured, then cleanup temp file
		if ctx.DownloadCache != nil {
			_ = ctx.DownloadCache.Save(url, result.AssetPath, result.Checksum)
		}
		_ = result.Cleanup()
	}

	steps := []Step{
		{
			Action: "download_file",
			Params: map[string]interface{}{
				"url":      url,
				"dest":     expandedDownloadName,
				"checksum": checksum,
			},
			Checksum: checksum,
			Size:     size,
		},
		{
			Action: "chmod",
			Params: map[string]interface{}{
				"files": []string{expandedDownloadName},
			},
		},
		{
			Action: "install_binaries",
			Params: map[string]interface{}{
				"binaries": binaries,
			},
		},
	}

	return steps, nil
}

// extractSourceFiles extracts source files from binaries parameter
// Handles both ["file"] and [{src: "file", dest: "..."}] formats
func extractSourceFiles(binariesRaw interface{}) []interface{} {
	arr, ok := binariesRaw.([]interface{})
	if !ok {
		return []interface{}{}
	}

	var result []interface{}
	for _, item := range arr {
		switch v := item.(type) {
		case string:
			// Simple string format
			result = append(result, v)
		case map[string]interface{}:
			// Map format with src/dest
			if src, ok := v["src"].(string); ok {
				result = append(result, src)
			}
		}
	}
	return result
}

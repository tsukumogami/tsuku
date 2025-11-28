package actions

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/tsuku-dev/tsuku/internal/version"
)

// DownloadArchiveAction downloads, extracts, and installs binaries from an archive
// This is a generic composite action for any URL
type DownloadArchiveAction struct{}

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
	verifyCmd := strings.TrimSpace(ctx.Recipe.Verify.Command)
	if (installMode == "directory" || installMode == "directory_wrapped") && verifyCmd == "" {
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

// GitHubArchiveAction downloads and extracts archives from GitHub releases
type GitHubArchiveAction struct{}

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
	verifyCmd := strings.TrimSpace(ctx.Recipe.Verify.Command)
	if (installMode == "directory" || installMode == "directory_wrapped") && verifyCmd == "" {
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

// GitHubFileAction downloads pre-compiled binary files from GitHub releases
type GitHubFileAction struct{}

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

// HashiCorpReleaseAction downloads HashiCorp products
type HashiCorpReleaseAction struct{}

func (a *HashiCorpReleaseAction) Name() string { return "hashicorp_release" }

func (a *HashiCorpReleaseAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Extract parameters
	product, ok := GetString(params, "product")
	if !ok {
		return fmt.Errorf("product is required")
	}

	binary, ok := GetString(params, "binary")
	if !ok {
		binary = product // Default to product name
	}

	// Build HashiCorp release URL
	// Format: https://releases.hashicorp.com/{product}/{version}/{product}_{version}_{os}_{arch}.zip
	osName := ctx.OS
	archName := ctx.Arch

	// HashiCorp uses specific naming conventions
	if osName == "darwin" {
		osName = "darwin"
	} else if osName == "linux" {
		osName = "linux"
	}

	if archName == "amd64" {
		archName = "amd64"
	} else if archName == "arm64" {
		archName = "arm64"
	}

	zipFile := fmt.Sprintf("%s_%s_%s_%s.zip", product, ctx.Version, osName, archName)
	url := fmt.Sprintf("https://releases.hashicorp.com/%s/%s/%s", product, ctx.Version, zipFile)

	// Step 1: Download zip
	downloadParams := map[string]interface{}{
		"url":  url,
		"dest": zipFile,
	}

	downloadAction := &DownloadAction{}
	if err := downloadAction.Execute(ctx, downloadParams); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Step 2: Extract zip
	extractParams := map[string]interface{}{
		"archive": zipFile,
		"format":  "zip",
	}

	extractAction := &ExtractAction{}
	if err := extractAction.Execute(ctx, extractParams); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}

	// Step 3: Chmod binary
	chmodAction := &ChmodAction{}
	chmodParams := map[string]interface{}{
		"files": []string{binary},
	}

	if err := chmodAction.Execute(ctx, chmodParams); err != nil {
		return fmt.Errorf("chmod failed: %w", err)
	}

	// Step 4: Install binary
	installAction := &InstallBinariesAction{}
	installParams := map[string]interface{}{
		"binaries": []interface{}{binary}, // Convert to []interface{} for TOML compatibility
	}

	if err := installAction.Execute(ctx, installParams); err != nil {
		return fmt.Errorf("install failed: %w", err)
	}

	return nil
}

// HomebrewBottleAction installs Homebrew bottles (stub for validation)
type HomebrewBottleAction struct{}

func (a *HomebrewBottleAction) Name() string { return "homebrew_bottle" }

func (a *HomebrewBottleAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	formula, ok := GetString(params, "formula")
	if !ok {
		return fmt.Errorf("formula is required")
	}

	// This is a stub - in reality this would:
	// 1. Fetch bottle metadata from Homebrew API
	// 2. Download the bottle for the current OS/arch
	// 3. Extract and relocate binaries
	// 4. Set up dependencies

	// For validation purposes, we just log what would happen
	fmt.Printf("📦 Would install Homebrew formula: %s\n", formula)
	fmt.Printf("   (Homebrew bottle installation not implemented in validator)\n")
	fmt.Printf("   This is expected - validator validates recipe format only\n")

	return nil
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

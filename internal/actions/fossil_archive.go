package actions

import (
	"fmt"
	"strings"
)

// Ensure FossilArchiveAction implements Decomposable
var _ Decomposable = (*FossilArchiveAction)(nil)

// FossilArchiveAction downloads, extracts, and installs source archives from Fossil SCM repositories.
// It constructs URLs using the standard Fossil tarball pattern: {repo}/tarball/{tag}/{project_name}.tar.gz
type FossilArchiveAction struct{ BaseAction }

// IsDeterministic returns true because fossil_archive decomposes to only deterministic primitives.
func (FossilArchiveAction) IsDeterministic() bool { return true }

func (a *FossilArchiveAction) Name() string { return "fossil_archive" }

// Preflight validates parameters without side effects.
func (a *FossilArchiveAction) Preflight(params map[string]interface{}) *PreflightResult {
	result := &PreflightResult{}

	repo, hasRepo := GetString(params, "repo")
	if !hasRepo {
		result.AddError("fossil_archive action requires 'repo' parameter")
	} else {
		// Validate repo is a valid HTTPS URL
		if !strings.HasPrefix(repo, "https://") {
			result.AddError("repo must be an HTTPS URL (e.g., 'https://sqlite.org/src')")
		}
	}

	if _, hasProjectName := GetString(params, "project_name"); !hasProjectName {
		result.AddError("fossil_archive action requires 'project_name' parameter")
	}

	return result
}

// Execute performs the fossil_archive action (runtime path - not used for deterministic plans).
// Downloads and extracts source code from a Fossil SCM repository.
func (a *FossilArchiveAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Extract parameters
	repo, ok := GetString(params, "repo")
	if !ok {
		return fmt.Errorf("repo is required")
	}

	projectName, ok := GetString(params, "project_name")
	if !ok {
		return fmt.Errorf("project_name is required")
	}

	stripDirs, _ := GetInt(params, "strip_dirs")
	if stripDirs == 0 {
		stripDirs = 1 // Default: strip 1 directory level
	}

	// Get optional tag configuration
	tagPrefix, _ := GetString(params, "tag_prefix")
	if tagPrefix == "" {
		tagPrefix = "version-"
	}
	versionSeparator, _ := GetString(params, "version_separator")
	if versionSeparator == "" {
		versionSeparator = "."
	}

	// Construct the tarball URL
	tag := a.versionToTag(ctx.Version, tagPrefix, versionSeparator)
	url := fmt.Sprintf("%s/tarball/%s/%s.tar.gz", repo, tag, projectName)
	archiveFilename := fmt.Sprintf("%s.tar.gz", projectName)

	// Step 1: Download archive
	downloadParams := map[string]interface{}{
		"url":  url,
		"dest": archiveFilename,
	}

	downloadAction := &DownloadAction{}
	if err := downloadAction.Execute(ctx, downloadParams); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Step 2: Extract archive
	extractParams := map[string]interface{}{
		"archive":    archiveFilename,
		"format":     "tar.gz",
		"strip_dirs": stripDirs,
	}

	extractAction := &ExtractAction{}
	if err := extractAction.Execute(ctx, extractParams); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}

	// Note: chmod and install_binaries are NOT performed here
	// because Fossil archives contain source code that needs to be built first.
	// Use configure_make or other build actions after fossil_archive.

	return nil
}

// Decompose returns the primitive steps for fossil_archive action.
// This enables deterministic plan generation by resolving URLs at evaluation time.
// Unlike github_archive, fossil_archive only decomposes to download_file + extract
// because Fossil archives contain source code that needs to be built, not pre-built binaries.
func (a *FossilArchiveAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
	// Extract required parameters
	repo, ok := GetString(params, "repo")
	if !ok {
		return nil, fmt.Errorf("repo is required")
	}

	projectName, ok := GetString(params, "project_name")
	if !ok {
		return nil, fmt.Errorf("project_name is required")
	}

	stripDirs, _ := GetInt(params, "strip_dirs")
	if stripDirs == 0 {
		stripDirs = 1 // Default: strip 1 directory level
	}

	// Get optional tag configuration
	tagPrefix, _ := GetString(params, "tag_prefix")
	if tagPrefix == "" {
		tagPrefix = "version-"
	}
	versionSeparator, _ := GetString(params, "version_separator")
	if versionSeparator == "" {
		versionSeparator = "."
	}

	// Construct the tarball URL using the version from context
	tag := a.versionToTag(ctx.Version, tagPrefix, versionSeparator)
	url := fmt.Sprintf("%s/tarball/%s/%s.tar.gz", repo, tag, projectName)
	archiveFilename := fmt.Sprintf("%s.tar.gz", projectName)

	// Delegate to download action for checksum computation
	downloadStep, err := decomposeDownload(ctx, url, archiveFilename, nil, nil)
	if err != nil {
		return nil, err
	}

	// Return only download + extract steps
	// Fossil archives contain source code, so chmod/install_binaries
	// should happen after the build step (e.g., configure_make)
	return []Step{
		downloadStep,
		{
			Action: "extract",
			Params: map[string]interface{}{
				"archive":    archiveFilename,
				"format":     "tar.gz",
				"strip_dirs": stripDirs,
			},
		},
	}, nil
}

// versionToTag converts a version string to a Fossil tag.
// Example: "3.46.0" with tagPrefix="version-" -> "version-3.46.0"
// Example: "9.0.0" with tagPrefix="core-" and versionSeparator="-" -> "core-9-0-0"
func (a *FossilArchiveAction) versionToTag(version, tagPrefix, versionSeparator string) string {
	tagVersion := version
	if versionSeparator != "." && versionSeparator != "" {
		tagVersion = strings.ReplaceAll(version, ".", versionSeparator)
	}
	return tagPrefix + tagVersion
}

package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// HomebrewSourceAction downloads and extracts source archives from Homebrew formulas.
// This is a composite action that resolves the source URL and checksum from the
// Homebrew API at plan time, enabling version-aware source builds.
type HomebrewSourceAction struct {
	BaseAction
	// HomebrewAPIURL allows overriding the Homebrew API URL for testing
	HomebrewAPIURL string
}

// IsDeterministic returns true because homebrew_source decomposes to only deterministic primitives.
func (HomebrewSourceAction) IsDeterministic() bool { return true }

// Name returns the action name
func (a *HomebrewSourceAction) Name() string { return "homebrew_source" }

// homebrewSourceInfo contains the source URL information from Homebrew API
type homebrewSourceInfo struct {
	URLs struct {
		Stable struct {
			URL      string `json:"url"`
			Checksum string `json:"checksum"`
		} `json:"stable"`
	} `json:"urls"`
}

// fetchSourceInfo fetches source URL and checksum from Homebrew API
func (a *HomebrewSourceAction) fetchSourceInfo(ctx context.Context, formula string) (*homebrewSourceInfo, error) {
	baseURL := a.HomebrewAPIURL
	if baseURL == "" {
		baseURL = "https://formulae.brew.sh"
	}

	url := fmt.Sprintf("%s/api/formula/%s.json", baseURL, formula)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch formula info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Homebrew API returned %d: %s", resp.StatusCode, string(body))
	}

	var info homebrewSourceInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to parse formula info: %w", err)
	}

	if info.URLs.Stable.URL == "" {
		return nil, fmt.Errorf("formula %s has no stable source URL", formula)
	}

	return &info, nil
}

// detectArchiveFormat determines the archive format from the URL
func (a *HomebrewSourceAction) detectArchiveFormat(url string) string {
	lower := strings.ToLower(url)
	switch {
	case strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar.xz"):
		return "tar.xz"
	case strings.HasSuffix(lower, ".tar.bz2"):
		return "tar.bz2"
	case strings.HasSuffix(lower, ".tar.lz"):
		return "tar.lz"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	default:
		return "tar.gz" // Default assumption
	}
}

// Execute downloads and extracts a Homebrew formula's source.
//
// Parameters:
//   - formula (required): Homebrew formula name (e.g., "jq")
//   - strip_dirs (optional): Number of directory levels to strip (default: 1)
func (a *HomebrewSourceAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	// Get formula name (required)
	formula, ok := GetString(params, "formula")
	if !ok {
		return fmt.Errorf("homebrew_source action requires 'formula' parameter")
	}

	// Get optional strip_dirs (default: 1 for typical source tarballs)
	stripDirs, _ := GetInt(params, "strip_dirs")
	if stripDirs == 0 {
		stripDirs = 1
	}

	// Fetch source info from Homebrew API
	apiCtx, cancel := context.WithTimeout(ctx.Context, 30*time.Second)
	defer cancel()

	info, err := a.fetchSourceInfo(apiCtx, formula)
	if err != nil {
		return fmt.Errorf("failed to fetch source info: %w", err)
	}

	sourceURL := info.URLs.Stable.URL
	checksum := info.URLs.Stable.Checksum
	archiveFormat := a.detectArchiveFormat(sourceURL)
	archiveFilename := filepath.Base(sourceURL)

	fmt.Printf("   Fetching source: %s\n", sourceURL)

	// Step 1: Download source archive
	downloadParams := map[string]interface{}{
		"url":  sourceURL,
		"dest": archiveFilename,
	}
	if checksum != "" {
		downloadParams["checksum"] = checksum
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

	fmt.Printf("   Extracted source: %s\n", formula)

	return nil
}

// Decompose resolves the Homebrew source URL and returns primitive steps.
// This enables deterministic plan generation by fetching the URL and checksum
// from the Homebrew API at plan time.
func (a *HomebrewSourceAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
	// Get formula name (required)
	formula, ok := GetString(params, "formula")
	if !ok {
		return nil, fmt.Errorf("homebrew_source action requires 'formula' parameter")
	}

	// Get optional strip_dirs (default: 1 for typical source tarballs)
	stripDirs, _ := GetInt(params, "strip_dirs")
	if stripDirs == 0 {
		stripDirs = 1
	}

	// Fetch source info from Homebrew API
	apiCtx, cancel := context.WithTimeout(ctx.Context, 30*time.Second)
	defer cancel()

	info, err := a.fetchSourceInfo(apiCtx, formula)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch source info: %w", err)
	}

	sourceURL := info.URLs.Stable.URL
	checksum := info.URLs.Stable.Checksum
	archiveFormat := a.detectArchiveFormat(sourceURL)
	archiveFilename := filepath.Base(sourceURL)

	// Return primitive steps
	steps := []Step{
		{
			Action: "download_file",
			Params: map[string]interface{}{
				"url":      sourceURL,
				"dest":     archiveFilename,
				"checksum": checksum,
			},
			Checksum: checksum,
		},
		{
			Action: "extract",
			Params: map[string]interface{}{
				"archive":    archiveFilename,
				"format":     archiveFormat,
				"strip_dirs": stripDirs,
			},
		},
	}

	return steps, nil
}

// Ensure HomebrewSourceAction implements Decomposable
var _ Decomposable = (*HomebrewSourceAction)(nil)

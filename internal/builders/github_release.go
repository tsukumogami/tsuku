package builders

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/llm"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

const (
	// maxGitHubResponseSize limits response body to prevent memory exhaustion (10MB)
	maxGitHubResponseSize = 10 * 1024 * 1024
	// maxREADMESize limits README content (1MB)
	maxREADMESize = 1 * 1024 * 1024
	// releasesToFetch is the number of releases to fetch for pattern inference
	releasesToFetch = 5
)

// GitHubReleaseBuilder generates recipes from GitHub release assets using LLM analysis.
type GitHubReleaseBuilder struct {
	httpClient    *http.Client
	llmClient     *llm.Client
	githubBaseURL string
}

// NewGitHubReleaseBuilder creates a new GitHubReleaseBuilder.
// If httpClient is nil, a default client with timeouts will be created.
// If llmClient is nil, a new client will be created using ANTHROPIC_API_KEY.
func NewGitHubReleaseBuilder(httpClient *http.Client, llmClient *llm.Client) (*GitHubReleaseBuilder, error) {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 60 * time.Second,
		}
	}

	if llmClient == nil {
		var err error
		llmClient, err = llm.NewClientWithHTTPClient(httpClient)
		if err != nil {
			return nil, fmt.Errorf("failed to create LLM client: %w", err)
		}
	}

	return &GitHubReleaseBuilder{
		httpClient:    httpClient,
		llmClient:     llmClient,
		githubBaseURL: "https://api.github.com",
	}, nil
}

// NewGitHubReleaseBuilderWithBaseURL creates a builder with custom GitHub API URL (for testing).
func NewGitHubReleaseBuilderWithBaseURL(httpClient *http.Client, llmClient *llm.Client, githubBaseURL string) (*GitHubReleaseBuilder, error) {
	b, err := NewGitHubReleaseBuilder(httpClient, llmClient)
	if err != nil {
		return nil, err
	}
	b.githubBaseURL = githubBaseURL
	return b, nil
}

// Name returns the builder identifier.
func (b *GitHubReleaseBuilder) Name() string {
	return "github"
}

// CanBuild checks if the SourceArg contains a valid owner/repo format.
func (b *GitHubReleaseBuilder) CanBuild(ctx context.Context, packageName string) (bool, error) {
	// This builder requires SourceArg, not packageName
	// Return false to indicate this builder cannot auto-detect packages
	return false, nil
}

// Build generates a recipe from GitHub release assets.
func (b *GitHubReleaseBuilder) Build(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	// Parse owner/repo from SourceArg
	owner, repo, err := parseRepo(req.SourceArg)
	if err != nil {
		return nil, fmt.Errorf("invalid source argument: %w", err)
	}

	repoPath := fmt.Sprintf("%s/%s", owner, repo)

	// Fetch releases
	releases, err := b.fetchReleases(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}

	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found for %s", repoPath)
	}

	// Fetch repo metadata
	repoMeta, err := b.fetchRepoMeta(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repo metadata: %w", err)
	}

	// Fetch README (non-fatal if it fails)
	readme := b.fetchREADME(ctx, owner, repo, releases[0].Tag)

	// Call LLM to extract pattern
	llmReq := &llm.GenerateRequest{
		Repo:        repoPath,
		Releases:    releases,
		Description: repoMeta.Description,
		README:      readme,
	}

	pattern, usage, err := b.llmClient.GenerateRecipe(ctx, llmReq)
	if err != nil {
		return nil, fmt.Errorf("LLM recipe generation failed: %w", err)
	}

	// Generate recipe from pattern
	r, err := generateRecipe(req.Package, repoPath, repoMeta, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to generate recipe: %w", err)
	}

	result := &BuildResult{
		Recipe: r,
		Source: fmt.Sprintf("github:%s", repoPath),
		Warnings: []string{
			fmt.Sprintf("LLM usage: %s", usage.String()),
		},
	}

	return result, nil
}

// parseRepo parses "owner/repo" into separate components.
func parseRepo(sourceArg string) (owner, repo string, err error) {
	if sourceArg == "" {
		return "", "", fmt.Errorf("source argument is required (use --from github:owner/repo)")
	}

	parts := strings.SplitN(sourceArg, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("expected owner/repo format, got: %s", sourceArg)
	}

	return parts[0], parts[1], nil
}

// githubRelease represents a GitHub release from the API.
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

// githubAsset represents a release asset.
type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// githubRepo represents GitHub repository metadata.
type githubRepo struct {
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
	HTMLURL     string `json:"html_url"`
}

// repoMeta holds processed repository metadata.
type repoMeta struct {
	Description string
	Homepage    string
}

// fetchReleases fetches the last N releases from GitHub API.
func (b *GitHubReleaseBuilder) fetchReleases(ctx context.Context, owner, repo string) ([]llm.Release, error) {
	baseURL, err := url.Parse(b.githubBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	apiURL := baseURL.JoinPath("repos", owner, repo, "releases")
	q := apiURL.Query()
	q.Set("per_page", fmt.Sprintf("%d", releasesToFetch))
	apiURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	b.setGitHubHeaders(req)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("repository %s/%s not found", owner, repo)
	}

	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("GitHub API rate limit exceeded; set GITHUB_TOKEN for higher limits")
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	limitedReader := io.LimitReader(resp.Body, maxGitHubResponseSize)

	var ghReleases []githubRelease
	if err := json.NewDecoder(limitedReader).Decode(&ghReleases); err != nil {
		return nil, fmt.Errorf("failed to parse releases: %w", err)
	}

	// Convert to llm.Release format
	releases := make([]llm.Release, 0, len(ghReleases))
	for _, r := range ghReleases {
		assets := make([]string, 0, len(r.Assets))
		for _, a := range r.Assets {
			assets = append(assets, a.Name)
		}
		releases = append(releases, llm.Release{
			Tag:    r.TagName,
			Assets: assets,
		})
	}

	return releases, nil
}

// fetchRepoMeta fetches repository metadata from GitHub API.
func (b *GitHubReleaseBuilder) fetchRepoMeta(ctx context.Context, owner, repo string) (*repoMeta, error) {
	baseURL, err := url.Parse(b.githubBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	apiURL := baseURL.JoinPath("repos", owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	b.setGitHubHeaders(req)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	limitedReader := io.LimitReader(resp.Body, maxGitHubResponseSize)

	var ghRepo githubRepo
	if err := json.NewDecoder(limitedReader).Decode(&ghRepo); err != nil {
		return nil, fmt.Errorf("failed to parse repo: %w", err)
	}

	meta := &repoMeta{
		Description: ghRepo.Description,
		Homepage:    ghRepo.Homepage,
	}

	// Use GitHub URL as fallback homepage
	if meta.Homepage == "" {
		meta.Homepage = ghRepo.HTMLURL
	}

	return meta, nil
}

// fetchREADME fetches the README from raw.githubusercontent.com.
// Returns empty string on failure (non-fatal).
func (b *GitHubReleaseBuilder) fetchREADME(ctx context.Context, owner, repo, tag string) string {
	readmeURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/README.md", owner, repo, tag)

	req, err := http.NewRequestWithContext(ctx, "GET", readmeURL, nil)
	if err != nil {
		return ""
	}

	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	limitedReader := io.LimitReader(resp.Body, maxREADMESize)
	content, err := io.ReadAll(limitedReader)
	if err != nil {
		return ""
	}

	return string(content)
}

// setGitHubHeaders sets common headers for GitHub API requests.
func (b *GitHubReleaseBuilder) setGitHubHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	// Use GITHUB_TOKEN if available for higher rate limits
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// generateRecipe creates a recipe.Recipe from the LLM pattern response.
func generateRecipe(packageName, repoPath string, meta *repoMeta, pattern *llm.AssetPattern) (*recipe.Recipe, error) {
	if len(pattern.Mappings) == 0 {
		return nil, fmt.Errorf("no platform mappings in pattern")
	}

	// Build OS and arch mappings from the pattern
	osMapping := make(map[string]string)
	archMapping := make(map[string]string)

	for _, m := range pattern.Mappings {
		osMapping[m.OS] = m.OS
		archMapping[m.Arch] = m.Arch
	}

	// Derive asset pattern from the first mapping
	// The LLM gives us specific assets; we need to infer the pattern
	assetPattern := deriveAssetPattern(pattern.Mappings)

	// Determine format from the first mapping
	format := pattern.Mappings[0].Format

	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:          packageName,
			Description:   meta.Description,
			Homepage:      meta.Homepage,
			VersionFormat: "semver",
		},
		Version: recipe.VersionSection{
			Source:     "github_releases",
			GitHubRepo: repoPath,
		},
		Verify: recipe.VerifySection{
			Command: pattern.VerifyCommand,
			Pattern: "{version}",
		},
	}

	if format == "binary" {
		// Use github_file for standalone binaries
		r.Steps = []recipe.Step{{
			Action: "github_file",
			Params: map[string]interface{}{
				"repo":          repoPath,
				"asset_pattern": assetPattern,
				"binary":        pattern.Executable,
				"os_mapping":    osMapping,
				"arch_mapping":  archMapping,
			},
		}}
	} else {
		// Use github_archive for archives
		stripDirs := 0
		if pattern.StripPrefix != "" {
			stripDirs = 1
		}

		params := map[string]interface{}{
			"repo":           repoPath,
			"asset_pattern":  assetPattern,
			"archive_format": format,
			"strip_dirs":     stripDirs,
			"binaries":       []string{pattern.Executable},
			"os_mapping":     osMapping,
			"arch_mapping":   archMapping,
		}

		if pattern.InstallSubpath != "" {
			params["install_subpath"] = pattern.InstallSubpath
		}

		r.Steps = []recipe.Step{{
			Action: "github_archive",
			Params: params,
		}}
	}

	return r, nil
}

// deriveAssetPattern infers a pattern string from concrete asset mappings.
// For example, from "gh_2.42.0_linux_amd64.tar.gz" it derives "gh_{version}_{os}_{arch}.tar.gz"
func deriveAssetPattern(mappings []llm.PlatformMapping) string {
	if len(mappings) == 0 {
		return ""
	}

	// Use the first mapping as the template
	asset := mappings[0].Asset
	os := mappings[0].OS
	arch := mappings[0].Arch

	// Replace OS and arch with placeholders
	pattern := asset
	if os != "" {
		pattern = strings.Replace(pattern, os, "{os}", 1)
	}
	if arch != "" {
		pattern = strings.Replace(pattern, arch, "{arch}", 1)
	}

	return pattern
}

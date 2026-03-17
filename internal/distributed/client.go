package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/discover"
	"github.com/tsukumogami/tsuku/internal/httputil"
	"github.com/tsukumogami/tsuku/internal/secrets"
)

// allowedDownloadHosts is the set of hostnames permitted in download URLs
// returned by the GitHub Contents API.
var allowedDownloadHosts = map[string]bool{
	"raw.githubusercontent.com":     true,
	"objects.githubusercontent.com": true,
}

// contentsAPIHost is the hostname for GitHub's API.
const contentsAPIHost = "api.github.com"

// recipeDirName is the directory name in repositories that holds tsuku recipes.
const recipeDirName = ".tsuku-recipes"

// defaultBranches lists branch names to try when the Contents API is unavailable
// (e.g., rate limited with no cached branch info).
var defaultBranches = []string{"main", "master"}

// contentsEntry represents a single entry from the GitHub Contents API response.
type contentsEntry struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
}

// authTransport is an http.RoundTripper that adds an Authorization header
// only for requests to api.github.com.
type authTransport struct {
	token string
	base  http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.EqualFold(req.URL.Hostname(), contentsAPIHost) && t.token != "" {
		req = req.Clone(req.Context())
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	return t.base.RoundTrip(req)
}

// GitHubClient fetches recipes from GitHub repositories using a two-tier
// approach: the Contents API for directory listings and raw content URLs
// for file downloads.
type GitHubClient struct {
	apiClient *http.Client // authenticated, for api.github.com only
	rawClient *http.Client // unauthenticated, for raw content downloads
	cache     *CacheManager
	hasToken  bool
}

// NewGitHubClient creates a GitHubClient with separate authenticated and
// unauthenticated HTTP clients. The authenticated client attaches a
// GITHUB_TOKEN (if available) only to api.github.com requests.
func NewGitHubClient(cache *CacheManager) *GitHubClient {
	rawClient := httputil.NewSecureClient(httputil.DefaultOptions())

	apiClient := httputil.NewSecureClient(httputil.DefaultOptions())
	hasToken := false

	if token, err := secrets.Get("github_token"); err == nil && token != "" {
		hasToken = true
		apiClient.Transport = &authTransport{
			token: token,
			base:  apiClient.Transport,
		}
	}

	return &GitHubClient{
		apiClient: apiClient,
		rawClient: rawClient,
		cache:     cache,
		hasToken:  hasToken,
	}
}

// newGitHubClientWithHTTP creates a GitHubClient with caller-supplied HTTP
// clients and cache. This is intended for testing.
func newGitHubClientWithHTTP(apiClient, rawClient *http.Client, cache *CacheManager, hasToken bool) *GitHubClient {
	return &GitHubClient{
		apiClient: apiClient,
		rawClient: rawClient,
		cache:     cache,
		hasToken:  hasToken,
	}
}

// ListRecipes returns the available recipe names in a repository's
// .tsuku-recipes/ directory. It uses the cache when fresh, falling back to
// the Contents API, and finally to raw URL probing on rate limit.
func (gc *GitHubClient) ListRecipes(ctx context.Context, owner, repo string) (*SourceMeta, error) {
	if err := discover.ValidateGitHubURL(owner + "/" + repo); err != nil {
		return nil, err
	}

	// Check cache first
	cached, err := gc.cache.GetSourceMeta(owner, repo)
	if err == nil && gc.cache.IsSourceFresh(cached) {
		return cached, nil
	}

	// Try Contents API
	meta, apiErr := gc.listViaContentsAPI(ctx, owner, repo)
	if apiErr == nil {
		// Cache write failure is non-fatal
		_ = gc.cache.PutSourceMeta(owner, repo, meta)
		return meta, nil
	}

	// If rate limited and we have a stale cache, use it
	if _, ok := apiErr.(*ErrRateLimited); ok && cached != nil {
		return cached, nil
	}

	// If rate limited with no cache, fall back to branch probing
	if rateLimitErr, ok := apiErr.(*ErrRateLimited); ok {
		meta, probeErr := gc.probeDefaultBranches(ctx, owner, repo)
		if probeErr != nil {
			// Return the rate limit error since probing also failed
			return nil, rateLimitErr
		}
		_ = gc.cache.PutSourceMeta(owner, repo, meta)
		return meta, nil
	}

	return nil, apiErr
}

// FetchRecipe downloads a single recipe TOML file from the given download URL.
// It validates the URL, checks the cache, and downloads if needed.
func (gc *GitHubClient) FetchRecipe(ctx context.Context, owner, repo, name, downloadURL string) ([]byte, error) {
	if err := validateDownloadURL(downloadURL); err != nil {
		return nil, err
	}

	// Check cache -- return immediately only when content exists and is fresh
	cached, err := gc.cache.GetRecipe(owner, repo, name)
	recipeMeta, _ := gc.cache.GetRecipeMeta(owner, repo, name)
	if err == nil && cached != nil && gc.cache.IsRecipeFresh(recipeMeta) {
		return cached, nil
	}

	// Download (stale or missing cache triggers a network request)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, &ErrNetwork{Operation: "creating request", Err: err}
	}
	req.Header.Set("User-Agent", httputil.DefaultUserAgent)

	// Add conditional headers only when we have both cached content and metadata.
	// Without cached content, a 304 response can't be fulfilled.
	if cached != nil && recipeMeta != nil {
		if recipeMeta.ETag != "" {
			req.Header.Set("If-None-Match", recipeMeta.ETag)
		}
		if recipeMeta.LastModified != "" {
			req.Header.Set("If-Modified-Since", recipeMeta.LastModified)
		}
	}

	resp, err := gc.rawClient.Do(req)
	if err != nil {
		return nil, &ErrNetwork{Operation: "fetching recipe", Err: err}
	}
	defer resp.Body.Close()

	// 304 Not Modified -- return cached content
	if resp.StatusCode == http.StatusNotModified && cached != nil {
		return cached, nil
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("recipe %q not found in %s/%s", name, owner, repo)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching recipe %q from %s/%s: unexpected status %d", name, owner, repo, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, &ErrNetwork{Operation: "reading recipe body", Err: err}
	}

	// Cache the result
	newMeta := &RecipeMeta{
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		FetchedAt:    time.Now(),
	}
	// Cache write failure is non-fatal
	_ = gc.cache.PutRecipe(owner, repo, name, data, newMeta)

	return data, nil
}

// listViaContentsAPI calls the GitHub Contents API to list .tsuku-recipes/.
func (gc *GitHubClient) listViaContentsAPI(ctx context.Context, owner, repo string) (*SourceMeta, error) {
	apiURL := fmt.Sprintf("https://%s/repos/%s/%s/contents/%s", contentsAPIHost, owner, repo, recipeDirName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, &ErrNetwork{Operation: "creating API request", Err: err}
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", httputil.DefaultUserAgent)

	resp, err := gc.apiClient.Do(req)
	if err != nil {
		return nil, &ErrNetwork{Operation: "calling Contents API", Err: err}
	}
	defer resp.Body.Close()

	// Check for rate limiting
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, parseRateLimitError(resp, gc.hasToken)
	}

	if resp.StatusCode == http.StatusNotFound {
		// Could be repo not found or directory not found -- try to distinguish
		// by checking if the repo itself exists would require another API call.
		// Keep it simple: report no recipe directory.
		return nil, &ErrNoRecipeDir{Owner: owner, Repo: repo}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Contents API for %s/%s returned unexpected status %d", owner, repo, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, &ErrNetwork{Operation: "reading API response", Err: err}
	}

	var entries []contentsEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("parsing Contents API response for %s/%s: %w", owner, repo, err)
	}

	files := make(map[string]string)
	for _, entry := range entries {
		if entry.Type != "file" || !strings.HasSuffix(entry.Name, ".toml") {
			continue
		}
		if err := validateDownloadURL(entry.DownloadURL); err != nil {
			continue // Skip files with invalid download URLs
		}
		name := strings.TrimSuffix(entry.Name, ".toml")
		files[name] = entry.DownloadURL
	}

	// Try to extract branch from a download URL to cache it
	branch := extractBranchFromURL(entries)

	return &SourceMeta{
		Branch:    branch,
		Files:     files,
		FetchedAt: time.Now(),
	}, nil
}

// probeDefaultBranches attempts to discover recipes by fetching raw content
// URLs directly, trying common branch names. This is the fallback when the
// Contents API is rate-limited.
func (gc *GitHubClient) probeDefaultBranches(ctx context.Context, owner, repo string) (*SourceMeta, error) {
	// If we have a cached branch, try it first
	cached, _ := gc.cache.GetSourceMeta(owner, repo)
	if cached != nil && cached.Branch != "" {
		branches := []string{cached.Branch}
		// Append default branches in case the cached one is stale
		for _, b := range defaultBranches {
			if b != cached.Branch {
				branches = append(branches, b)
			}
		}
		return gc.tryBranches(ctx, owner, repo, branches)
	}
	return gc.tryBranches(ctx, owner, repo, defaultBranches)
}

// tryBranches attempts to access the recipe directory via raw URLs on each
// branch in order. Since we can't list files via raw URLs, this only
// confirms the branch exists. A real listing must come from the Contents API
// once rate limits reset.
func (gc *GitHubClient) tryBranches(ctx context.Context, owner, repo string, branches []string) (*SourceMeta, error) {
	for _, branch := range branches {
		// Try to fetch a known probe URL (we can't list, but we can check if the branch exists)
		probeURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s/", owner, repo, branch, recipeDirName)
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, probeURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", httputil.DefaultUserAgent)

		resp, err := gc.rawClient.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusMovedPermanently {
			return &SourceMeta{
				Branch:     branch,
				Files:      nil, // Can't list via raw URLs; will populate on next successful API call
				FetchedAt:  time.Now(),
				Incomplete: true, // Short TTL so full listing is fetched once rate limits reset
			}, nil
		}
	}
	return nil, &ErrNoRecipeDir{Owner: owner, Repo: repo}
}

// validateDownloadURL checks that a download URL uses HTTPS and points to
// an allowed GitHub content host.
func validateDownloadURL(rawURL string) error {
	if rawURL == "" {
		return &ErrInvalidDownloadURL{URL: rawURL, Reason: "empty URL"}
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return &ErrInvalidDownloadURL{URL: rawURL, Reason: "malformed URL"}
	}

	if u.Scheme != "https" {
		return &ErrInvalidDownloadURL{URL: rawURL, Reason: "must use HTTPS"}
	}

	host := strings.ToLower(u.Hostname())
	if !allowedDownloadHosts[host] {
		return &ErrInvalidDownloadURL{
			URL:    rawURL,
			Reason: fmt.Sprintf("hostname %q not in allowlist (allowed: raw.githubusercontent.com, objects.githubusercontent.com)", host),
		}
	}

	return nil
}

// extractBranchFromURL tries to extract the branch name from a Contents API
// download_url. The URL format is:
// https://raw.githubusercontent.com/{owner}/{repo}/{branch}/{path}
func extractBranchFromURL(entries []contentsEntry) string {
	for _, e := range entries {
		if e.DownloadURL == "" {
			continue
		}
		u, err := url.Parse(e.DownloadURL)
		if err != nil {
			continue
		}
		// Path: /{owner}/{repo}/{branch}/.tsuku-recipes/{file}
		parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 5)
		if len(parts) >= 3 {
			return parts[2]
		}
	}
	return ""
}

// parseRateLimitError extracts rate limit information from response headers.
func parseRateLimitError(resp *http.Response, hasToken bool) *ErrRateLimited {
	remaining, _ := strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining"))
	resetUnix, _ := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64)

	var resetAt time.Time
	if resetUnix > 0 {
		resetAt = time.Unix(resetUnix, 0)
	}

	return &ErrRateLimited{
		Remaining: remaining,
		ResetAt:   resetAt,
		HasToken:  hasToken,
	}
}

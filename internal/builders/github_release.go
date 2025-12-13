package builders

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/llm"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/telemetry"
	"github.com/tsukumogami/tsuku/internal/validate"
)

const (
	// maxGitHubResponseSize limits response body to prevent memory exhaustion (10MB)
	maxGitHubResponseSize = 10 * 1024 * 1024
	// maxREADMESize limits README content (1MB)
	maxREADMESize = 1 * 1024 * 1024
	// releasesToFetch is the number of releases to fetch for pattern inference
	releasesToFetch = 5
	// MaxTurns is the maximum number of conversation turns to prevent infinite loops.
	MaxTurns = 5
	// MaxRepairAttempts is the maximum number of times to retry after validation failure.
	MaxRepairAttempts = 2
)

// ProgressReporter receives progress updates during recipe generation.
type ProgressReporter interface {
	// OnStageStart is called when a stage begins. The stage name is printed
	// followed by "... " (no newline).
	OnStageStart(stage string)
	// OnStageDone is called when a stage completes successfully.
	// If detail is non-empty, prints "done (detail)", otherwise just "done".
	OnStageDone(detail string)
	// OnStageFailed is called when a stage fails.
	OnStageFailed()
}

// GitHubReleaseBuilder generates recipes from GitHub release assets using LLM analysis.
type GitHubReleaseBuilder struct {
	httpClient      *http.Client
	factory         *llm.Factory
	executor        *validate.Executor
	sanitizer       *validate.Sanitizer
	githubBaseURL   string
	telemetryClient *telemetry.Client
	progress        ProgressReporter
}

// GitHubReleaseBuilderOption configures a GitHubReleaseBuilder.
type GitHubReleaseBuilderOption func(*GitHubReleaseBuilder)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(c *http.Client) GitHubReleaseBuilderOption {
	return func(b *GitHubReleaseBuilder) {
		b.httpClient = c
	}
}

// WithFactory sets the LLM provider factory.
func WithFactory(f *llm.Factory) GitHubReleaseBuilderOption {
	return func(b *GitHubReleaseBuilder) {
		b.factory = f
	}
}

// WithExecutor sets the validation executor.
func WithExecutor(e *validate.Executor) GitHubReleaseBuilderOption {
	return func(b *GitHubReleaseBuilder) {
		b.executor = e
	}
}

// WithSanitizer sets the error sanitizer.
func WithSanitizer(s *validate.Sanitizer) GitHubReleaseBuilderOption {
	return func(b *GitHubReleaseBuilder) {
		b.sanitizer = s
	}
}

// WithGitHubBaseURL sets a custom GitHub API base URL (for testing).
func WithGitHubBaseURL(url string) GitHubReleaseBuilderOption {
	return func(b *GitHubReleaseBuilder) {
		b.githubBaseURL = url
	}
}

// WithTelemetryClient sets the telemetry client for emitting LLM events.
func WithTelemetryClient(c *telemetry.Client) GitHubReleaseBuilderOption {
	return func(b *GitHubReleaseBuilder) {
		b.telemetryClient = c
	}
}

// WithProgressReporter sets the progress reporter for stage updates.
func WithProgressReporter(p ProgressReporter) GitHubReleaseBuilderOption {
	return func(b *GitHubReleaseBuilder) {
		b.progress = p
	}
}

// NewGitHubReleaseBuilder creates a new GitHubReleaseBuilder.
// If no options are provided, defaults are used:
// - HTTP client with 60s timeout
// - Factory auto-detected from environment (ANTHROPIC_API_KEY, GOOGLE_API_KEY)
// - Sanitizer with default patterns
// - No executor (validation skipped)
// - Default telemetry client (respects TSUKU_NO_TELEMETRY)
func NewGitHubReleaseBuilder(ctx context.Context, opts ...GitHubReleaseBuilderOption) (*GitHubReleaseBuilder, error) {
	b := &GitHubReleaseBuilder{
		githubBaseURL: "https://api.github.com",
	}

	for _, opt := range opts {
		opt(b)
	}

	// Set defaults for unset options
	if b.httpClient == nil {
		b.httpClient = &http.Client{
			Timeout: 60 * time.Second,
		}
	}

	if b.factory == nil {
		factory, err := llm.NewFactory(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to create LLM factory: %w", err)
		}
		b.factory = factory
	}

	if b.sanitizer == nil {
		b.sanitizer = validate.NewSanitizer()
	}

	if b.telemetryClient == nil {
		b.telemetryClient = telemetry.NewClient()
	}

	// Set up factory callback for circuit breaker telemetry
	b.factory.SetOnBreakerTrip(func(provider string, failures int) {
		b.telemetryClient.SendLLM(telemetry.NewLLMCircuitBreakerTripEvent(provider, failures))
	})

	// executor is optional - validation skipped if nil

	return b, nil
}

// Name returns the builder identifier.
func (b *GitHubReleaseBuilder) Name() string {
	return "github"
}

// reportStart reports a stage starting, if progress reporter is set.
func (b *GitHubReleaseBuilder) reportStart(stage string) {
	if b.progress != nil {
		b.progress.OnStageStart(stage)
	}
}

// reportDone reports a stage completed successfully, if progress reporter is set.
func (b *GitHubReleaseBuilder) reportDone(detail string) {
	if b.progress != nil {
		b.progress.OnStageDone(detail)
	}
}

// reportFailed reports a stage failed, if progress reporter is set.
func (b *GitHubReleaseBuilder) reportFailed() {
	if b.progress != nil {
		b.progress.OnStageFailed()
	}
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
	b.reportStart("Fetching release metadata")
	releases, err := b.fetchReleases(ctx, owner, repo)
	if err != nil {
		b.reportFailed()
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}

	if len(releases) == 0 {
		b.reportFailed()
		return nil, fmt.Errorf("no releases found for %s", repoPath)
	}

	// Fetch repo metadata
	repoMeta, err := b.fetchRepoMeta(ctx, owner, repo)
	if err != nil {
		b.reportFailed()
		return nil, fmt.Errorf("failed to fetch repo metadata: %w", err)
	}

	// Fetch README (non-fatal if it fails)
	readme := b.fetchREADME(ctx, owner, repo, releases[0].Tag)

	// Report metadata fetch complete with details
	assetCount := 0
	if len(releases) > 0 {
		assetCount = len(releases[0].Assets)
	}
	b.reportDone(fmt.Sprintf("%s, %d assets", releases[0].Tag, assetCount))

	// Build generation context
	genCtx := &generationContext{
		repo:        repoPath,
		releases:    releases,
		description: repoMeta.Description,
		readme:      readme,
		httpClient:  b.httpClient,
	}
	if len(releases) > 0 {
		genCtx.tag = releases[0].Tag
	}

	// Get provider from factory
	provider, err := b.factory.GetProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("no LLM provider available: %w", err)
	}

	// Emit generation started event
	b.telemetryClient.SendLLM(telemetry.NewLLMGenerationStartedEvent(provider.Name(), req.Package, repoPath))
	startTime := time.Now()

	// Generate recipe with repair loop
	pattern, usage, repairAttempts, validationSkipped, err := b.generateWithRepair(ctx, provider, genCtx, req.Package, repoPath, repoMeta)

	// Calculate duration for completed event
	durationMs := time.Since(startTime).Milliseconds()

	if err != nil {
		b.factory.ReportFailure(provider.Name())
		// Emit generation completed (failure) event
		b.telemetryClient.SendLLM(telemetry.NewLLMGenerationCompletedEvent(provider.Name(), req.Package, false, durationMs, repairAttempts+1))
		return nil, err
	}
	b.factory.ReportSuccess(provider.Name())

	// Emit generation completed (success) event
	b.telemetryClient.SendLLM(telemetry.NewLLMGenerationCompletedEvent(provider.Name(), req.Package, true, durationMs, repairAttempts+1))

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
		RepairAttempts:    repairAttempts,
		Provider:          provider.Name(),
		ValidationSkipped: validationSkipped,
		Cost:              usage.Cost(),
	}

	if repairAttempts > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Recipe repaired after %d attempt(s)", repairAttempts))
	}

	return result, nil
}

// generationContext holds context needed during recipe generation.
type generationContext struct {
	repo        string // GitHub repository (owner/repo)
	tag         string // Release tag to use for file fetching
	releases    []llm.Release
	description string
	readme      string
	httpClient  *http.Client
}

// generateWithRepair runs the conversation loop with validation and repair.
func (b *GitHubReleaseBuilder) generateWithRepair(
	ctx context.Context,
	provider llm.Provider,
	genCtx *generationContext,
	packageName, repoPath string,
	repoMeta *repoMeta,
) (*llm.AssetPattern, *llm.Usage, int, bool, error) {
	// Build initial conversation
	systemPrompt := buildSystemPrompt()
	userMessage := buildUserMessage(genCtx)
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: userMessage},
	}
	tools := buildToolDefs()

	var totalUsage llm.Usage
	var repairAttempts int
	var validationSkipped bool
	var lastErrorCategory string

	for attempt := 0; attempt <= MaxRepairAttempts; attempt++ {
		// Emit repair attempt event for retries (not the first attempt)
		if attempt > 0 {
			b.telemetryClient.SendLLM(telemetry.NewLLMRepairAttemptEvent(provider.Name(), attempt, lastErrorCategory))
			// Report repair progress
			b.reportStart(fmt.Sprintf("Repairing recipe (attempt %d/%d)", attempt, MaxRepairAttempts+1))
		} else {
			// Report LLM analysis starting (first attempt only)
			b.reportStart(fmt.Sprintf("Analyzing assets with %s", provider.Name()))
		}

		// Run conversation loop to get pattern
		pattern, turnUsage, err := b.runConversationLoop(ctx, provider, systemPrompt, messages, tools, genCtx)
		if err != nil {
			b.reportFailed()
			return nil, &totalUsage, repairAttempts, validationSkipped, err
		}
		totalUsage.Add(*turnUsage)

		// Report LLM analysis done
		b.reportDone("")

		// If no executor, skip validation
		if b.executor == nil {
			validationSkipped = true
			// Emit validation skipped event
			b.telemetryClient.SendLLM(telemetry.NewLLMValidationResultEvent(true, "skipped", attempt+1))
			return pattern, &totalUsage, repairAttempts, validationSkipped, nil
		}

		// Generate recipe for validation
		r, err := generateRecipe(packageName, repoPath, repoMeta, pattern)
		if err != nil {
			return nil, &totalUsage, repairAttempts, validationSkipped, fmt.Errorf("failed to generate recipe for validation: %w", err)
		}

		// Substitute {version} in verify command and pattern with actual version tag
		// This allows the pattern check to work correctly during validation
		if genCtx.tag != "" {
			// Strip 'v' prefix if present for version matching
			version := strings.TrimPrefix(genCtx.tag, "v")
			if r.Verify.Command != "" {
				r.Verify.Command = strings.ReplaceAll(r.Verify.Command, "{version}", version)
			}
			if r.Verify.Pattern != "" {
				r.Verify.Pattern = strings.ReplaceAll(r.Verify.Pattern, "{version}", version)
			}
		}

		// Build asset URL for validation
		assetURL := b.buildAssetURL(genCtx, pattern)

		// Report validation starting
		b.reportStart("Validating in container")

		// Validate in container
		result, err := b.executor.Validate(ctx, r, assetURL)
		if err != nil {
			b.reportFailed()
			return nil, &totalUsage, repairAttempts, validationSkipped, fmt.Errorf("validation error: %w", err)
		}

		// Check validation result
		if result.Skipped {
			validationSkipped = true
			// Emit validation skipped event
			b.telemetryClient.SendLLM(telemetry.NewLLMValidationResultEvent(true, "skipped", attempt+1))
			b.reportDone("skipped")
			return pattern, &totalUsage, repairAttempts, validationSkipped, nil
		}

		if result.Passed {
			// Emit validation passed event
			b.telemetryClient.SendLLM(telemetry.NewLLMValidationResultEvent(true, "", attempt+1))
			b.reportDone("")
			return pattern, &totalUsage, repairAttempts, validationSkipped, nil
		}

		// Validation failed
		b.reportFailed()

		// Parse error category for telemetry
		parsed := validate.ParseValidationError(result.Stdout, result.Stderr, result.ExitCode)
		lastErrorCategory = string(parsed.Category)

		// Emit validation failed event
		b.telemetryClient.SendLLM(telemetry.NewLLMValidationResultEvent(false, lastErrorCategory, attempt+1))

		// Validation failed - prepare repair if we have attempts left
		if attempt >= MaxRepairAttempts {
			return nil, &totalUsage, repairAttempts, validationSkipped, fmt.Errorf("recipe validation failed after %d repair attempts: %s", repairAttempts, b.formatValidationError(result))
		}

		// Continue conversation with error feedback
		repairAttempts++
		repairMessage := b.buildRepairMessage(result)
		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: repairMessage})
	}

	return nil, &totalUsage, repairAttempts, validationSkipped, fmt.Errorf("unexpected end of repair loop")
}

// runConversationLoop executes the multi-turn conversation until extract_pattern is called.
func (b *GitHubReleaseBuilder) runConversationLoop(
	ctx context.Context,
	provider llm.Provider,
	systemPrompt string,
	messages []llm.Message,
	tools []llm.ToolDef,
	genCtx *generationContext,
) (*llm.AssetPattern, *llm.Usage, error) {
	var totalUsage llm.Usage

	for turn := 0; turn < MaxTurns; turn++ {
		resp, err := provider.Complete(ctx, &llm.CompletionRequest{
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Tools:        tools,
			MaxTokens:    4096,
		})
		if err != nil {
			return nil, &totalUsage, err
		}

		totalUsage.Add(resp.Usage)

		// Add assistant response to conversation
		messages = append(messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Process tool calls
		var toolResults []llm.Message
		var pattern *llm.AssetPattern

		for _, tc := range resp.ToolCalls {
			result, extractedPattern, err := b.executeToolCall(ctx, genCtx, tc)
			if err != nil {
				// Return error as tool result so LLM can try again
				toolResults = append(toolResults, llm.Message{
					Role: llm.RoleUser,
					ToolResult: &llm.ToolResult{
						CallID:  tc.ID,
						Content: fmt.Sprintf("Error: %v", err),
						IsError: true,
					},
				})
				continue
			}

			if extractedPattern != nil {
				pattern = extractedPattern
			} else {
				toolResults = append(toolResults, llm.Message{
					Role: llm.RoleUser,
					ToolResult: &llm.ToolResult{
						CallID:  tc.ID,
						Content: result,
						IsError: false,
					},
				})
			}
		}

		// If extract_pattern was called, return the pattern
		if pattern != nil {
			return pattern, &totalUsage, nil
		}

		// If there were tool calls, add results and continue
		if len(toolResults) > 0 {
			messages = append(messages, toolResults...)
			continue
		}

		// No tool calls and no extract_pattern - LLM is done but didn't call the tool
		if resp.StopReason == "end_turn" {
			return nil, &totalUsage, fmt.Errorf("conversation ended without extract_pattern being called")
		}
	}

	return nil, &totalUsage, fmt.Errorf("max turns (%d) exceeded without completing recipe generation", MaxTurns)
}

// executeToolCall executes a tool call and returns the result.
func (b *GitHubReleaseBuilder) executeToolCall(ctx context.Context, genCtx *generationContext, tc llm.ToolCall) (string, *llm.AssetPattern, error) {
	switch tc.Name {
	case llm.ToolFetchFile:
		path, _ := tc.Arguments["path"].(string)
		if path == "" {
			return "", nil, fmt.Errorf("invalid fetch_file input: missing path")
		}
		content, err := fetchFile(ctx, genCtx.httpClient, genCtx.repo, genCtx.tag, path)
		if err != nil {
			return "", nil, err
		}
		return content, nil, nil

	case llm.ToolInspectArchive:
		url, _ := tc.Arguments["url"].(string)
		if url == "" {
			return "", nil, fmt.Errorf("invalid inspect_archive input: missing url")
		}
		listing, err := inspectArchive(ctx, genCtx.httpClient, url)
		if err != nil {
			return "", nil, err
		}
		return listing, nil, nil

	case llm.ToolExtractPattern:
		argsJSON, err := json.Marshal(tc.Arguments)
		if err != nil {
			return "", nil, fmt.Errorf("invalid extract_pattern input: %w", err)
		}
		var input llm.ExtractPatternInput
		if err := json.Unmarshal(argsJSON, &input); err != nil {
			return "", nil, fmt.Errorf("invalid extract_pattern input: %w", err)
		}
		pattern := &llm.AssetPattern{
			Mappings:       input.Mappings,
			Executable:     input.Executable,
			VerifyCommand:  input.VerifyCommand,
			StripPrefix:    input.StripPrefix,
			InstallSubpath: input.InstallSubpath,
		}
		return "", pattern, nil

	default:
		return "", nil, fmt.Errorf("unknown tool: %s", tc.Name)
	}
}

// buildAssetURL constructs the download URL for the first matching platform.
func (b *GitHubReleaseBuilder) buildAssetURL(genCtx *generationContext, pattern *llm.AssetPattern) string {
	if len(pattern.Mappings) == 0 || len(genCtx.releases) == 0 {
		return ""
	}

	// Find asset URL for the first mapping (typically linux/amd64)
	firstAsset := pattern.Mappings[0].Asset
	release := genCtx.releases[0]

	// Construct GitHub release download URL
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s",
		genCtx.repo, release.Tag, firstAsset)
}

// buildRepairMessage constructs the error feedback message for the LLM.
func (b *GitHubReleaseBuilder) buildRepairMessage(result *validate.ValidationResult) string {
	// Combine stdout and stderr
	output := result.Stdout + "\n" + result.Stderr

	// Sanitize the output
	sanitizedOutput := b.sanitizer.Sanitize(output)

	// Parse the error for structured feedback
	parsed := validate.ParseValidationError(result.Stdout, result.Stderr, result.ExitCode)

	var sb strings.Builder
	sb.WriteString("The recipe you generated failed validation. Here is the error:\n\n")
	sb.WriteString("---\n")
	sb.WriteString(sanitizedOutput)
	sb.WriteString("\n---\n\n")
	sb.WriteString(fmt.Sprintf("Exit code: %d\n", result.ExitCode))
	sb.WriteString(fmt.Sprintf("Error category: %s\n", parsed.Category))

	if len(parsed.Suggestions) > 0 {
		sb.WriteString("\nSuggested fixes:\n")
		for _, suggestion := range parsed.Suggestions {
			sb.WriteString(fmt.Sprintf("- %s\n", suggestion))
		}
	}

	sb.WriteString("\nPlease analyze what went wrong and call extract_pattern again with a corrected recipe.")

	return sb.String()
}

// formatValidationError creates a human-readable validation error message.
func (b *GitHubReleaseBuilder) formatValidationError(result *validate.ValidationResult) string {
	output := result.Stdout + "\n" + result.Stderr
	sanitized := b.sanitizer.Sanitize(output)
	if len(sanitized) > 500 {
		sanitized = sanitized[:500] + "..."
	}
	return fmt.Sprintf("exit code %d: %s", result.ExitCode, sanitized)
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
		return nil, &GitHubRepoNotFoundError{Owner: owner, Repo: repo}
	}

	if resp.StatusCode == 403 || resp.StatusCode == 429 {
		// Parse rate limit reset time from headers
		retryAfter := 60 * time.Minute // Default fallback
		if resetStr := resp.Header.Get("X-RateLimit-Reset"); resetStr != "" {
			if resetUnix, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
				resetTime := time.Unix(resetUnix, 0)
				retryAfter = time.Until(resetTime)
				if retryAfter < 0 {
					retryAfter = time.Minute
				}
			}
		}
		return nil, &GitHubRateLimitError{
			RetryAfter:    retryAfter,
			Authenticated: os.Getenv("GITHUB_TOKEN") != "",
		}
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

	if resp.StatusCode == 404 {
		return nil, &GitHubRepoNotFoundError{Owner: owner, Repo: repo}
	}

	if resp.StatusCode == 403 || resp.StatusCode == 429 {
		retryAfter := 60 * time.Minute
		if resetStr := resp.Header.Get("X-RateLimit-Reset"); resetStr != "" {
			if resetUnix, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
				resetTime := time.Unix(resetUnix, 0)
				retryAfter = time.Until(resetTime)
				if retryAfter < 0 {
					retryAfter = time.Minute
				}
			}
		}
		return nil, &GitHubRateLimitError{
			RetryAfter:    retryAfter,
			Authenticated: os.Getenv("GITHUB_TOKEN") != "",
		}
	}

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

// normalizeOS maps common OS identifiers to Go runtime constants.
func normalizeOS(os string) string {
	switch strings.ToLower(os) {
	case "linux", "gnu", "unknown-linux-gnu", "unknown-linux-musl":
		return "linux"
	case "darwin", "macos", "apple-darwin", "osx":
		return "darwin"
	case "windows", "win", "win32", "win64", "pc-windows-msvc", "pc-windows-gnu":
		return "windows"
	default:
		// Check for target triple patterns
		lower := strings.ToLower(os)
		if strings.Contains(lower, "linux") {
			return "linux"
		}
		if strings.Contains(lower, "darwin") || strings.Contains(lower, "apple") || strings.Contains(lower, "macos") {
			return "darwin"
		}
		if strings.Contains(lower, "windows") {
			return "windows"
		}
		return strings.ToLower(os)
	}
}

// normalizeArch maps common architecture identifiers to Go runtime constants.
func normalizeArch(arch string) string {
	switch strings.ToLower(arch) {
	case "amd64", "x86_64", "x64", "64bit", "64-bit":
		return "amd64"
	case "arm64", "aarch64":
		return "arm64"
	case "386", "i386", "i686", "x86", "32bit", "32-bit":
		return "386"
	default:
		return strings.ToLower(arch)
	}
}

// normalizeFormat maps archive format aliases to canonical names.
func normalizeFormat(format string) string {
	switch strings.ToLower(format) {
	case "tgz":
		return "tar.gz"
	default:
		return format
	}
}

// generateRecipe creates a recipe.Recipe from the LLM pattern response.
func generateRecipe(packageName, repoPath string, meta *repoMeta, pattern *llm.AssetPattern) (*recipe.Recipe, error) {
	if len(pattern.Mappings) == 0 {
		return nil, fmt.Errorf("no platform mappings in pattern")
	}

	// Build OS and arch mappings from the pattern, normalizing to Go runtime constants
	osMapping := make(map[string]string)
	archMapping := make(map[string]string)

	for _, m := range pattern.Mappings {
		normalizedOS := normalizeOS(m.OS)
		normalizedArch := normalizeArch(m.Arch)
		osMapping[normalizedOS] = normalizedOS
		archMapping[normalizedArch] = normalizedArch
	}

	// Derive asset pattern from the first mapping
	// The LLM gives us specific assets; we need to infer the pattern
	assetPattern := deriveAssetPattern(pattern.Mappings)

	// Determine format from the first mapping, normalizing aliases
	format := normalizeFormat(pattern.Mappings[0].Format)

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

// buildSystemPrompt creates the system prompt for recipe generation.
func buildSystemPrompt() string {
	return `You are an expert at analyzing GitHub releases to create installation recipes for tsuku, a package manager.

Your task is to analyze the provided release information and determine how to match release assets to different platforms (linux/darwin, amd64/arm64).

You have three tools available:
1. fetch_file: Fetch a file from a URL to examine its contents (useful for READMEs)
2. inspect_archive: Inspect the contents of an archive to find the executable
3. extract_pattern: Call this when you've determined the asset-to-platform mappings

When calling extract_pattern, use these target platforms:
- os: "linux" or "darwin"
- arch: "amd64" or "arm64"

Example - k9s_Linux_amd64.tar.gz:
{
  "mappings": [
    {"asset": "k9s_Linux_amd64.tar.gz", "os": "linux", "arch": "amd64", "format": "tar.gz"},
    {"asset": "k9s_Linux_arm64.tar.gz", "os": "linux", "arch": "arm64", "format": "tar.gz"},
    {"asset": "k9s_Darwin_amd64.tar.gz", "os": "darwin", "arch": "amd64", "format": "tar.gz"},
    {"asset": "k9s_Darwin_arm64.tar.gz", "os": "darwin", "arch": "arm64", "format": "tar.gz"}
  ],
  "executable": "k9s",
  "verify_command": "k9s version"
}

When analyzing assets:
- Look for patterns in filenames that indicate OS and architecture
- Identify the archive format from the file extension: tar.gz, tar.xz, zip, tbz (bzip2 tar), tgz, or binary (no extension)
- Determine the executable name inside the archive
- Consider common verification commands (tool --version, tool version)

Once you understand the pattern, call extract_pattern with the mappings.
Focus on linux (amd64, arm64) and darwin (amd64, arm64) platforms.`
}

// buildUserMessage creates the initial user message with release context.
func buildUserMessage(genCtx *generationContext) string {
	releasesJSON, _ := json.MarshalIndent(genCtx.releases, "", "  ")

	msg := fmt.Sprintf(`Please analyze this GitHub repository and its releases to create a recipe.

Repository: %s
Description: %s

Recent releases:
%s

`, genCtx.repo, genCtx.description, string(releasesJSON))

	if genCtx.readme != "" {
		// Truncate README if too long
		readme := genCtx.readme
		if len(readme) > 10000 {
			readme = readme[:10000] + "\n...(truncated)"
		}
		msg += fmt.Sprintf("README.md:\n%s\n", readme)
	}

	msg += "\nAnalyze the release assets and call extract_pattern with the platform mappings."

	return msg
}

// buildToolDefs creates the tool definitions for recipe generation.
func buildToolDefs() []llm.ToolDef {
	return []llm.ToolDef{
		{
			Name:        llm.ToolFetchFile,
			Description: "Fetch a file from the repository to examine its contents. Use this to read READMEs, Makefiles, or other documentation that might help understand the project structure.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "The path to the file in the repository (e.g., 'README.md', 'Makefile')",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        llm.ToolInspectArchive,
			Description: "Download and inspect the contents of an archive to find the executable. Returns a listing of files in the archive.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "The URL of the archive to inspect",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        llm.ToolExtractPattern,
			Description: "Report the discovered pattern for matching release assets to platforms. Call this when you've determined how to map assets to OS/arch combinations.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"mappings": map[string]any{
						"type":        "array",
						"description": "List of platform-to-asset mappings",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"os": map[string]any{
									"type":        "string",
									"description": "OS identifier as it appears in the asset name (e.g., 'linux', 'darwin', 'x86_64-unknown-linux-musl')",
								},
								"arch": map[string]any{
									"type":        "string",
									"description": "Architecture identifier as it appears in the asset name (e.g., 'amd64', 'arm64', '')",
								},
								"asset": map[string]any{
									"type":        "string",
									"description": "The exact asset filename for this platform",
								},
								"format": map[string]any{
									"type":        "string",
									"description": "Archive format detected from file extension",
									"enum":        []string{"tar.gz", "tar.xz", "zip", "tbz", "tgz", "binary"},
								},
							},
							"required": []string{"os", "arch", "asset", "format"},
						},
					},
					"executable": map[string]any{
						"type":        "string",
						"description": "Name of the executable binary",
					},
					"verify_command": map[string]any{
						"type":        "string",
						"description": "Command to verify installation (e.g., 'mytool --version')",
					},
					"strip_prefix": map[string]any{
						"type":        "string",
						"description": "Directory prefix to strip from archives (optional)",
					},
					"install_subpath": map[string]any{
						"type":        "string",
						"description": "Subdirectory in archive where binary is located (optional)",
					},
				},
				"required": []string{"mappings", "executable", "verify_command"},
			},
		},
	}
}

// fetchFile fetches a file from a GitHub repository using raw.githubusercontent.com.
func fetchFile(ctx context.Context, httpClient *http.Client, repo, tag, path string) (string, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", repo, tag, path)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("file not found: %s (check if the file exists in the repository at tag %s)", path, tag)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Check content type - reject binary files
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !isTextContentType(contentType) {
		return "", fmt.Errorf("file appears to be binary (Content-Type: %s), only text files are supported", contentType)
	}

	// Limit response size to 1MB to prevent memory issues
	const maxSize = 1 * 1024 * 1024
	content := make([]byte, maxSize)
	n, err := resp.Body.Read(content)
	if err != nil && err.Error() != "EOF" {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return string(content[:n]), nil
}

// isTextContentType checks if the content type indicates a text file.
func isTextContentType(contentType string) bool {
	textPrefixes := []string{
		"text/",
		"application/json",
		"application/xml",
		"application/javascript",
		"application/x-yaml",
		"application/toml",
	}
	for _, prefix := range textPrefixes {
		if len(contentType) >= len(prefix) && contentType[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// inspectArchive downloads and lists the contents of an archive.
func inspectArchive(ctx context.Context, httpClient *http.Client, archiveURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", archiveURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download archive: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// For now, return a placeholder - archive inspection requires more complex logic
	// This is consistent with the existing implementation in client.go
	return "Archive inspection not fully implemented - please analyze based on filename patterns", nil
}

// Legacy constructors for backward compatibility

// NewGitHubReleaseBuilderLegacy creates a builder with legacy constructor signature.
// Deprecated: Use NewGitHubReleaseBuilder instead.
func NewGitHubReleaseBuilderLegacy(httpClient *http.Client, llmClient *llm.Client) (*GitHubReleaseBuilder, error) {
	ctx := context.Background()
	opts := []GitHubReleaseBuilderOption{}

	if httpClient != nil {
		opts = append(opts, WithHTTPClient(httpClient))
	}

	// Note: llmClient is ignored in new implementation - factory is used instead

	return NewGitHubReleaseBuilder(ctx, opts...)
}

// NewGitHubReleaseBuilderWithBaseURL creates a builder with custom GitHub API URL.
// Deprecated: Use NewGitHubReleaseBuilder with WithGitHubBaseURL option instead.
func NewGitHubReleaseBuilderWithBaseURL(httpClient *http.Client, llmClient *llm.Client, githubBaseURL string) (*GitHubReleaseBuilder, error) {
	ctx := context.Background()
	opts := []GitHubReleaseBuilderOption{
		WithGitHubBaseURL(githubBaseURL),
	}

	if httpClient != nil {
		opts = append(opts, WithHTTPClient(httpClient))
	}

	return NewGitHubReleaseBuilder(ctx, opts...)
}

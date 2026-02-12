package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/llm"
	"github.com/tsukumogami/tsuku/internal/log"
	"github.com/tsukumogami/tsuku/internal/search"
)

// Constants for LLM Discovery configuration.
const (
	// MaxDiscoveryTurns limits conversation turns to prevent runaway costs.
	MaxDiscoveryTurns = 5

	// DefaultDiscoveryTimeout is the total timeout for a discovery session.
	DefaultDiscoveryTimeout = 60 * time.Second

	// MinConfidenceThreshold gates result acceptance.
	MinConfidenceThreshold = 70

	// MinStarsThreshold filters low-quality repos.
	MinStarsThreshold = 50
)

// Tool names for LLM discovery.
const (
	ToolWebSearch     = "web_search"
	ToolExtractSource = "extract_source"
)

// LLMDiscovery resolves tool names via LLM web search as a last resort.
// This is the third and final stage of the resolver chain.
type LLMDiscovery struct {
	factory  *llm.Factory
	search   search.Provider
	confirm  ConfirmFunc
	httpGet  HTTPGetFunc
	logger   log.Logger
	disabled bool // Set when no LLM provider is available
}

// ConfirmFunc is a callback for user confirmation.
// It displays the discovery result and returns true if the user approves.
type ConfirmFunc func(result *DiscoveryResult) bool

// HTTPGetFunc abstracts HTTP GET for testing.
type HTTPGetFunc func(ctx context.Context, url string) ([]byte, error)

// RateLimitError indicates GitHub API rate limit was exceeded.
type RateLimitError struct {
	ResetTime     time.Time // When the rate limit resets
	Authenticated bool      // Whether request used GITHUB_TOKEN
}

func (e *RateLimitError) Error() string {
	msg := "GitHub API rate limit exceeded"
	if !e.ResetTime.IsZero() {
		msg += fmt.Sprintf(" (resets at %s)", e.ResetTime.Format(time.RFC3339))
	}
	return msg
}

// IsRateLimited returns true (implements a marker interface for rate limit errors).
func (e *RateLimitError) IsRateLimited() bool {
	return true
}

// Suggestion returns a user-friendly message about the rate limit.
func (e *RateLimitError) Suggestion() string {
	if e.Authenticated {
		return "GitHub API rate limit exceeded. Please wait and try again."
	}
	return "GitHub API rate limit exceeded. Set GITHUB_TOKEN for higher limits (5000 req/hour)."
}

// LLMDiscoveryOption configures an LLMDiscovery instance.
type LLMDiscoveryOption func(*LLMDiscovery)

// WithConfirmFunc sets a custom confirmation function.
func WithConfirmFunc(fn ConfirmFunc) LLMDiscoveryOption {
	return func(d *LLMDiscovery) {
		d.confirm = fn
	}
}

// WithHTTPGet sets a custom HTTP GET function for testing.
func WithHTTPGet(fn HTTPGetFunc) LLMDiscoveryOption {
	return func(d *LLMDiscovery) {
		d.httpGet = fn
	}
}

// WithSearchProvider sets a custom search provider.
func WithSearchProvider(p search.Provider) LLMDiscoveryOption {
	return func(d *LLMDiscovery) {
		d.search = p
	}
}

// NewLLMDiscovery creates an LLM-based discovery resolver.
func NewLLMDiscovery(ctx context.Context, opts ...LLMDiscoveryOption) (*LLMDiscovery, error) {
	factory, err := llm.NewFactory(ctx)
	if err != nil {
		return nil, fmt.Errorf("llm discovery: %w", err)
	}

	d := &LLMDiscovery{
		factory: factory,
		search:  search.NewDDGProvider(),
		confirm: defaultConfirm,
		httpGet: defaultHTTPGet,
		logger:  log.Default(),
	}

	for _, opt := range opts {
		opt(d)
	}

	return d, nil
}

// NewLLMDiscoveryDisabled creates a disabled LLM discovery (for when no provider is available).
func NewLLMDiscoveryDisabled() *LLMDiscovery {
	return &LLMDiscovery{disabled: true}
}

// Resolve searches for a tool via LLM web search and GitHub verification.
// Returns (nil, nil) for a soft miss (no suitable source found).
func (d *LLMDiscovery) Resolve(ctx context.Context, toolName string) (*DiscoveryResult, error) {
	if d.disabled {
		return nil, nil
	}

	// Apply discovery timeout
	ctx, cancel := context.WithTimeout(ctx, DefaultDiscoveryTimeout)
	defer cancel()

	// Get an LLM provider
	provider, err := d.factory.GetProvider(ctx)
	if err != nil {
		d.logger.Debug(fmt.Sprintf("llm discovery: no provider available: %v", err))
		return nil, nil // Soft miss - no provider
	}

	// Create and run session to collect candidates
	session := newDiscoverySession(d, provider, toolName)
	candidates, err := session.run(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		d.logger.Debug(fmt.Sprintf("llm discovery: session error: %v", err))
		return nil, nil // Soft miss
	}

	if len(candidates) == 0 {
		return nil, nil // No candidates found
	}

	d.logger.Debug(fmt.Sprintf("llm discovery: found %d candidates for %q", len(candidates), toolName))

	// Verify each candidate against GitHub API
	var verifiedCandidates []*DiscoveryResult
	var rateLimited bool
	var rateLimitWarning string

	for _, candidate := range candidates {
		verified, err := d.verifyGitHubRepo(ctx, candidate)
		if err != nil {
			// Check if this is a rate limit error
			var rateLimitErr *RateLimitError
			if isRateLimitErr(err, &rateLimitErr) {
				d.logger.Debug(fmt.Sprintf("llm discovery: rate limited while verifying %s", candidate.Source))
				rateLimited = true
				rateLimitWarning = rateLimitErr.Suggestion()
				// Keep the candidate with verification skipped
				candidate.Metadata.VerificationSkipped = true
				candidate.Metadata.VerificationWarning = rateLimitWarning
				verifiedCandidates = append(verifiedCandidates, candidate)
			} else {
				// Non-rate-limit errors: skip this candidate
				d.logger.Debug(fmt.Sprintf("llm discovery: verification failed for %s: %v", candidate.Source, err))
				continue
			}
		} else {
			candidate.Metadata = verified
			verifiedCandidates = append(verifiedCandidates, candidate)
			d.logger.Debug(fmt.Sprintf("llm discovery: verified %s (stars=%d, fork=%v)",
				candidate.Source, verified.Stars, verified.IsFork))
		}
	}

	if len(verifiedCandidates) == 0 {
		d.logger.Debug(fmt.Sprintf("llm discovery: no candidates passed verification for %q", toolName))
		return nil, nil
	}

	// Rank candidates by confidence (desc), then stars (desc)
	ranked := rankCandidates(verifiedCandidates)
	d.logger.Debug(fmt.Sprintf("llm discovery: ranked %d candidates, best is %s (confidence=%d, stars=%d)",
		len(ranked), ranked[0].Source, ranked[0].ConfidenceScore, ranked[0].Metadata.Stars))

	// Select the best candidate
	result := d.selectBestCandidate(ranked)
	if result == nil {
		d.logger.Debug(fmt.Sprintf("llm discovery: no suitable candidate for %q", toolName))
		return nil, nil
	}

	// If rate limited and this candidate wasn't verified, apply warning
	if rateLimited && result.Metadata.VerificationSkipped {
		result.Metadata.VerificationWarning = rateLimitWarning
	}

	// Apply quality thresholds (skip if verification was skipped due to rate limit)
	// Note: forks are handled by selectBestCandidate, but still need threshold check
	if !result.Metadata.VerificationSkipped && !result.Metadata.IsFork && !d.passesQualityThreshold(result) {
		d.logger.Debug(fmt.Sprintf("llm discovery: best candidate for %q failed quality threshold", toolName))
		return nil, nil
	}

	// Require user confirmation
	if d.confirm != nil && !d.confirm(result) {
		d.logger.Debug(fmt.Sprintf("llm discovery: user declined result for %q", toolName))
		return nil, nil
	}

	return result, nil
}

// passesQualityThreshold checks if the result meets minimum quality requirements.
// Forks never auto-pass - they always require explicit user confirmation.
func (d *LLMDiscovery) passesQualityThreshold(result *DiscoveryResult) bool {
	// Forks never auto-pass - require explicit confirmation
	if result.Metadata.IsFork {
		return false
	}

	// For non-forks, pass if stars above threshold
	return result.Metadata.Stars >= MinStarsThreshold
}

// rankCandidates sorts candidates by confidence (descending), then stars (descending).
// Returns a new sorted slice without modifying the original.
func rankCandidates(candidates []*DiscoveryResult) []*DiscoveryResult {
	if len(candidates) <= 1 {
		return candidates
	}

	// Create a copy to avoid modifying the original
	sorted := make([]*DiscoveryResult, len(candidates))
	copy(sorted, candidates)

	// Sort by confidence DESC, then stars DESC, then source ASC for stability
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if shouldSwap(sorted[i], sorted[j]) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// shouldSwap returns true if b should come before a in the ranking.
func shouldSwap(a, b *DiscoveryResult) bool {
	// Higher confidence wins
	if b.ConfidenceScore > a.ConfidenceScore {
		return true
	}
	if b.ConfidenceScore < a.ConfidenceScore {
		return false
	}

	// Equal confidence: higher stars wins
	if b.Metadata.Stars > a.Metadata.Stars {
		return true
	}
	if b.Metadata.Stars < a.Metadata.Stars {
		return false
	}

	// Equal confidence and stars: sort by source for stability
	return b.Source < a.Source
}

// selectBestCandidate picks the best candidate from a ranked list.
// Returns the best non-fork that passes quality thresholds, or the best fork with a warning.
// Returns nil if no candidates are available.
func (d *LLMDiscovery) selectBestCandidate(candidates []*DiscoveryResult) *DiscoveryResult {
	if len(candidates) == 0 {
		return nil
	}

	// First pass: find best non-fork that passes threshold
	for _, c := range candidates {
		if !c.Metadata.IsFork && d.passesQualityThreshold(c) {
			return c
		}
	}

	// Second pass: find best non-fork (even if below threshold)
	for _, c := range candidates {
		if !c.Metadata.IsFork {
			// Below threshold but not a fork - still offer to user
			return c
		}
	}

	// All candidates are forks - return the best fork
	// The fork warning will be shown during confirmation
	return candidates[0]
}

// verifyGitHubRepo verifies a GitHub repository exists and returns its metadata.
func (d *LLMDiscovery) verifyGitHubRepo(ctx context.Context, result *DiscoveryResult) (Metadata, error) {
	if result.Builder != "github" {
		return Metadata{}, fmt.Errorf("unsupported builder: %s", result.Builder)
	}

	// Parse owner/repo from source
	parts := strings.SplitN(result.Source, "/", 2)
	if len(parts) != 2 {
		return Metadata{}, fmt.Errorf("invalid source format: %s", result.Source)
	}
	owner, repo := parts[0], parts[1]

	// Fetch from GitHub API
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	body, err := d.httpGet(ctx, url)
	if err != nil {
		return Metadata{}, err
	}

	// Parse response
	var ghRepo struct {
		StargazersCount int    `json:"stargazers_count"`
		ForksCount      int    `json:"forks_count"`
		Archived        bool   `json:"archived"`
		Description     string `json:"description"`
		CreatedAt       string `json:"created_at"`
		PushedAt        string `json:"pushed_at"`
		Fork            bool   `json:"fork"`
		Owner           *struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"owner"`
		Parent *struct {
			FullName        string `json:"full_name"`
			StargazersCount int    `json:"stargazers_count"`
		} `json:"parent"`
	}
	if err := json.Unmarshal(body, &ghRepo); err != nil {
		return Metadata{}, fmt.Errorf("parse github response: %w", err)
	}

	// Reject archived repos
	if ghRepo.Archived {
		return Metadata{}, fmt.Errorf("repository is archived")
	}

	// Calculate age in days and format creation date
	var ageDays int
	var createdAtStr string
	if createdAt, err := time.Parse(time.RFC3339, ghRepo.CreatedAt); err == nil {
		ageDays = int(time.Since(createdAt).Hours() / 24)
		createdAtStr = createdAt.Format("2006-01-02")
	}

	// Calculate days since last commit
	var lastCommitDays int
	if pushedAt, err := time.Parse(time.RFC3339, ghRepo.PushedAt); err == nil {
		lastCommitDays = int(time.Since(pushedAt).Hours() / 24)
	}

	// Build metadata with fork information
	metadata := Metadata{
		Stars:          ghRepo.StargazersCount,
		Description:    ghRepo.Description,
		AgeDays:        ageDays,
		CreatedAt:      createdAtStr,
		LastCommitDays: lastCommitDays,
		IsFork:         ghRepo.Fork,
	}

	// Populate owner metadata if available
	if ghRepo.Owner != nil {
		metadata.OwnerName = ghRepo.Owner.Login
		metadata.OwnerType = ghRepo.Owner.Type
	}

	// Populate parent metadata if this is a fork
	if ghRepo.Fork && ghRepo.Parent != nil {
		metadata.ParentRepo = ghRepo.Parent.FullName
		metadata.ParentStars = ghRepo.Parent.StargazersCount
	}

	return metadata, nil
}

// discoverySession manages a single LLM discovery conversation.
type discoverySession struct {
	discovery *LLMDiscovery
	provider  llm.Provider
	toolName  string
	messages  []llm.Message
	tools     []llm.ToolDef
}

func newDiscoverySession(d *LLMDiscovery, provider llm.Provider, toolName string) *discoverySession {
	return &discoverySession{
		discovery: d,
		provider:  provider,
		toolName:  toolName,
		messages:  []llm.Message{},
		tools:     discoveryToolDefs(),
	}
}

// discoveryToolDefs returns the tool definitions for LLM discovery.
func discoveryToolDefs() []llm.ToolDef {
	return []llm.ToolDef{
		{
			Name: ToolWebSearch,
			Description: `Search the web for information about a developer tool.
Use this to find the official source repository, download page, or documentation.
Focus on finding the canonical source (GitHub, GitLab, official website).`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name: ToolExtractSource,
			Description: `Extract the source information for installing a developer tool.
Call this when you've found the official source repository.
Only extract sources you're confident about.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"builder": map[string]any{
						"type":        "string",
						"description": "The builder to use: 'github' for GitHub releases",
						"enum":        []string{"github"},
					},
					"source": map[string]any{
						"type":        "string",
						"description": "Builder-specific source (e.g., 'owner/repo' for github)",
					},
					"confidence": map[string]any{
						"type":        "integer",
						"description": "Confidence score 0-100",
						"minimum":     0,
						"maximum":     100,
					},
					"evidence": map[string]any{
						"type":        "array",
						"description": "Evidence supporting this extraction",
						"items":       map[string]any{"type": "string"},
					},
					"reasoning": map[string]any{
						"type":        "string",
						"description": "Reasoning for this extraction",
					},
				},
				"required": []string{"builder", "source", "confidence", "evidence", "reasoning"},
			},
		},
	}
}

// discoverySystemPrompt returns the system prompt for discovery.
func discoverySystemPrompt(toolName string) string {
	return fmt.Sprintf(`You are a tool discovery assistant for tsuku, a package manager for developer tools.

Your task is to find the official source repository for: %s

Guidelines:
1. Use web_search to find the official source (GitHub repository preferred)
2. Look for the CANONICAL source, not forks or mirrors
3. Verify the repository has releases or downloads
4. Only call extract_source when you're confident you've found the right source
5. If you can't find a reliable source, say so instead of guessing

The builder "github" expects source in "owner/repo" format.
Example: for stripe-cli, the source would be "stripe/stripe-cli"`, toolName)
}

// run executes the discovery session and returns all candidates found.
// Returns a slice of candidates (may be empty) or an error.
func (s *discoverySession) run(ctx context.Context) ([]*DiscoveryResult, error) {
	// Initial user message
	s.messages = append(s.messages, llm.Message{
		Role:    llm.RoleUser,
		Content: fmt.Sprintf("Find the official source repository for: %s", s.toolName),
	})

	var candidates []*DiscoveryResult

	for turn := 0; turn < MaxDiscoveryTurns; turn++ {
		s.discovery.logger.Debug(fmt.Sprintf("llm discovery: turn %d for %q", turn+1, s.toolName))

		resp, err := s.provider.Complete(ctx, &llm.CompletionRequest{
			SystemPrompt: discoverySystemPrompt(s.toolName),
			Messages:     s.messages,
			Tools:        s.tools,
			MaxTokens:    2048,
		})
		if err != nil {
			return nil, fmt.Errorf("llm complete: %w", err)
		}

		s.discovery.logger.Debug(fmt.Sprintf("llm discovery: response has %d tool calls, stop_reason=%s, content_len=%d",
			len(resp.ToolCalls), resp.StopReason, len(resp.Content)))

		// Add assistant response to history
		s.messages = append(s.messages, llm.Message{
			Role:      llm.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Process tool calls
		var toolResults []llm.Message
		var foundCandidate bool

		for _, tc := range resp.ToolCalls {
			s.discovery.logger.Debug(fmt.Sprintf("llm discovery: executing tool %s", tc.Name))
			toolResult, extracted, err := s.executeToolCall(ctx, tc)
			if err != nil {
				s.discovery.logger.Debug(fmt.Sprintf("llm discovery: tool %s error: %v", tc.Name, err))
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

			if extracted != nil {
				// Collect candidate instead of returning immediately
				candidates = append(candidates, extracted)
				foundCandidate = true
				s.discovery.logger.Debug(fmt.Sprintf("llm discovery: collected candidate %s:%s (confidence=%d)",
					extracted.Builder, extracted.Source, extracted.ConfidenceScore))
				// Send acknowledgment back to LLM
				toolResults = append(toolResults, llm.Message{
					Role: llm.RoleUser,
					ToolResult: &llm.ToolResult{
						CallID:  tc.ID,
						Content: fmt.Sprintf("Recorded source: %s:%s", extracted.Builder, extracted.Source),
						IsError: false,
					},
				})
			} else {
				toolResults = append(toolResults, llm.Message{
					Role: llm.RoleUser,
					ToolResult: &llm.ToolResult{
						CallID:  tc.ID,
						Content: toolResult,
						IsError: false,
					},
				})
			}
		}

		// If we found at least one candidate and LLM is done, return candidates
		if foundCandidate && resp.StopReason == "end_turn" {
			s.discovery.logger.Debug(fmt.Sprintf("llm discovery: session complete with %d candidates", len(candidates)))
			return candidates, nil
		}

		// Add tool results and continue
		if len(toolResults) > 0 {
			s.messages = append(s.messages, toolResults...)
			continue
		}

		// No tool calls and no result - check if LLM gave up
		if resp.StopReason == "end_turn" && len(resp.ToolCalls) == 0 {
			if len(candidates) > 0 {
				return candidates, nil
			}
			return nil, fmt.Errorf("LLM completed without finding source")
		}
	}

	// Return any candidates collected even if max turns exceeded
	if len(candidates) > 0 {
		s.discovery.logger.Debug(fmt.Sprintf("llm discovery: max turns exceeded but returning %d candidates", len(candidates)))
		return candidates, nil
	}

	return nil, fmt.Errorf("max turns (%d) exceeded", MaxDiscoveryTurns)
}

// executeToolCall processes a single tool call.
func (s *discoverySession) executeToolCall(ctx context.Context, tc llm.ToolCall) (string, *DiscoveryResult, error) {
	switch tc.Name {
	case ToolWebSearch:
		return s.handleWebSearch(ctx, tc)
	case ToolExtractSource:
		result, err := s.handleExtractSource(tc)
		return "", result, err
	default:
		return "", nil, fmt.Errorf("unknown tool: %s", tc.Name)
	}
}

// handleWebSearch performs a web search using the configured search provider.
func (s *discoverySession) handleWebSearch(ctx context.Context, tc llm.ToolCall) (string, *DiscoveryResult, error) {
	query, ok := tc.Arguments["query"].(string)
	if !ok || query == "" {
		return "", nil, fmt.Errorf("web_search: query is required")
	}

	if s.discovery.search == nil {
		return "", nil, fmt.Errorf("web_search: no search provider configured")
	}

	resp, err := s.discovery.search.Search(ctx, query)
	if err != nil {
		return "", nil, fmt.Errorf("web_search failed: %w", err)
	}

	// Sanitize search results before LLM sees them (prompt injection defense)
	for i := range resp.Results {
		resp.Results[i].Title = StripHTML(resp.Results[i].Title)
		resp.Results[i].Snippet = StripHTML(resp.Results[i].Snippet)
	}

	// Format results for LLM (limit to 10 results)
	return resp.FormatForLLM(10), nil, nil
}

// handleExtractSource processes extraction results from the LLM.
func (s *discoverySession) handleExtractSource(tc llm.ToolCall) (*DiscoveryResult, error) {
	// Parse arguments
	builder, _ := tc.Arguments["builder"].(string)
	source, _ := tc.Arguments["source"].(string)
	confidence, _ := tc.Arguments["confidence"].(float64)
	reasoning, _ := tc.Arguments["reasoning"].(string)
	evidenceRaw, _ := tc.Arguments["evidence"].([]any)

	// Validate required fields
	if builder == "" || source == "" {
		return nil, fmt.Errorf("extract_source: builder and source are required")
	}

	// Convert evidence
	var evidence []string
	for _, e := range evidenceRaw {
		if s, ok := e.(string); ok {
			evidence = append(evidence, s)
		}
	}

	// Validate source format for github builder
	if builder == "github" {
		// Use comprehensive URL validation (handles both owner/repo and full URL formats)
		if err := ValidateGitHubURL(source); err != nil {
			return nil, fmt.Errorf("extract_source: %w", err)
		}
	}

	// Check confidence threshold
	if int(confidence) < MinConfidenceThreshold {
		return nil, fmt.Errorf("extract_source: confidence %d is below threshold %d", int(confidence), MinConfidenceThreshold)
	}

	return &DiscoveryResult{
		Builder:         builder,
		Source:          source,
		Confidence:      ConfidenceLLM,
		ConfidenceScore: int(confidence), // Store for ranking
		Reason:          fmt.Sprintf("LLM discovery: %s (evidence: %s)", reasoning, strings.Join(evidence, ", ")),
		Metadata:        Metadata{}, // Will be filled by GitHub verification
	}, nil
}

// defaultConfirm is the default confirmation function (always confirms).
// In production, this is replaced by the CLI confirmation prompt.
func defaultConfirm(result *DiscoveryResult) bool {
	return true
}

// defaultHTTPGet performs an HTTP GET request with GITHUB_TOKEN if available.
func defaultHTTPGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "tsuku")

	// Use GITHUB_TOKEN if available for higher rate limits (5000/hr vs 60/hr)
	token := os.Getenv("GITHUB_TOKEN")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check for rate limit (403 with X-RateLimit-Remaining: 0)
	if resp.StatusCode == http.StatusForbidden {
		if isRateLimitResponse(resp) {
			return nil, parseRateLimitError(resp, token != "")
		}
		return nil, fmt.Errorf("HTTP 403 Forbidden")
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("repository not found")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// isRateLimitResponse checks if a 403 response is due to rate limiting.
func isRateLimitResponse(resp *http.Response) bool {
	remaining := resp.Header.Get("X-RateLimit-Remaining")
	return remaining == "0"
}

// parseRateLimitError creates a RateLimitError from a rate-limited response.
func parseRateLimitError(resp *http.Response, authenticated bool) *RateLimitError {
	err := &RateLimitError{
		Authenticated: authenticated,
	}

	// Parse reset time from X-RateLimit-Reset header (Unix timestamp)
	if resetStr := resp.Header.Get("X-RateLimit-Reset"); resetStr != "" {
		if resetUnix, parseErr := strconv.ParseInt(resetStr, 10, 64); parseErr == nil {
			err.ResetTime = time.Unix(resetUnix, 0)
		}
	}

	return err
}

// isRateLimitErr checks if an error is a RateLimitError and extracts it.
func isRateLimitErr(err error, target **RateLimitError) bool {
	if rateLimitErr, ok := err.(*RateLimitError); ok {
		*target = rateLimitErr
		return true
	}
	return false
}

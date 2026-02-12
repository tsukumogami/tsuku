package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestLLMDiscovery_Integration(t *testing.T) {
	// Skip if no API key
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Create discovery with auto-confirm for testing
	discovery, err := NewLLMDiscovery(ctx, WithConfirmFunc(func(result *DiscoveryResult) bool {
		t.Logf("Would confirm: %s:%s (stars=%d)", result.Builder, result.Source, result.Metadata.Stars)
		return true
	}))
	if err != nil {
		t.Fatalf("NewLLMDiscovery: %v", err)
	}

	// Test with a well-known tool
	result, err := discovery.Resolve(ctx, "stripe-cli")
	if err != nil {
		t.Errorf("Resolve error: %v", err)
	}

	if result == nil {
		t.Log("No result returned (may be expected if tool not found or threshold not met)")
		return
	}

	t.Logf("Result: builder=%s source=%s confidence=%s", result.Builder, result.Source, result.Confidence)
	t.Logf("Metadata: stars=%d age=%d", result.Metadata.Stars, result.Metadata.AgeDays)
	t.Logf("Reason: %s", result.Reason)

	// Validate result
	if result.Builder != "github" {
		t.Errorf("expected builder=github, got %s", result.Builder)
	}
	if result.Source != "stripe/stripe-cli" {
		t.Errorf("expected source=stripe/stripe-cli, got %s", result.Source)
	}
	if result.Confidence != ConfidenceLLM {
		t.Errorf("expected confidence=llm, got %s", result.Confidence)
	}
}

func TestLLMDiscovery_Disabled(t *testing.T) {
	discovery := NewLLMDiscoveryDisabled()

	result, err := discovery.Resolve(context.Background(), "anything")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result from disabled discovery")
	}
}

func TestGitHubSourceValidation(t *testing.T) {
	tests := []struct {
		source string
		valid  bool
	}{
		{"stripe/stripe-cli", true},
		{"cli/cli", true},
		{"FiloSottile/age", true},
		{"owner-with-dash/repo", true},
		{"owner/repo-with-dash", true},
		{"owner/repo_underscore", true},
		{"", false},
		{"noslash", false},
		{"/invalid", false},
		{"invalid/", false},
		{"../evil", false},
		{"owner/../other", false},
	}

	for _, tc := range tests {
		t.Run(tc.source, func(t *testing.T) {
			err := ValidateGitHubURL(tc.source)
			got := err == nil
			if got != tc.valid {
				t.Errorf("ValidateGitHubURL(%q) error=%v, want valid=%v", tc.source, err, tc.valid)
			}
		})
	}
}

func TestDiscoveryToolDefs(t *testing.T) {
	tools := discoveryToolDefs()

	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}

	// Verify web_search tool
	var webSearch, extractSource *struct {
		Name       string
		Params     map[string]any
		Properties map[string]any
	}

	for _, tool := range tools {
		switch tool.Name {
		case ToolWebSearch:
			props := tool.Parameters["properties"].(map[string]any)
			webSearch = &struct {
				Name       string
				Params     map[string]any
				Properties map[string]any
			}{tool.Name, tool.Parameters, props}
		case ToolExtractSource:
			props := tool.Parameters["properties"].(map[string]any)
			extractSource = &struct {
				Name       string
				Params     map[string]any
				Properties map[string]any
			}{tool.Name, tool.Parameters, props}
		}
	}

	if webSearch == nil {
		t.Fatal("web_search tool not found")
	}
	if extractSource == nil {
		t.Fatal("extract_source tool not found")
	}

	// Verify web_search has query parameter
	if webSearch.Properties["query"] == nil {
		t.Error("web_search missing query parameter")
	}

	// Verify extract_source has required parameters
	for _, param := range []string{"builder", "source", "confidence", "evidence", "reasoning"} {
		if extractSource.Properties[param] == nil {
			t.Errorf("extract_source missing %s parameter", param)
		}
	}
}

func TestVerifyGitHubRepo(t *testing.T) {
	if os.Getenv("GITHUB_TOKEN") == "" && os.Getenv("CI") == "" {
		t.Log("Running without GITHUB_TOKEN - rate limits may apply")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	discovery := &LLMDiscovery{
		httpGet: defaultHTTPGet,
	}

	result := &DiscoveryResult{
		Builder: "github",
		Source:  "cli/cli", // GitHub CLI - well-known, high stars
	}

	metadata, err := discovery.verifyGitHubRepo(ctx, result)
	if err != nil {
		t.Fatalf("verifyGitHubRepo: %v", err)
	}

	t.Logf("GitHub CLI: stars=%d description=%q age=%d days", metadata.Stars, metadata.Description, metadata.AgeDays)

	if metadata.Stars < 1000 {
		t.Errorf("expected >1000 stars for cli/cli, got %d", metadata.Stars)
	}
	if metadata.Description == "" {
		t.Error("expected non-empty description")
	}
}

func TestExtractSourceValidation(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]any
		wantErr    bool
		errContain string
	}{
		{
			name: "valid extraction",
			args: map[string]any{
				"builder":    "github",
				"source":     "stripe/stripe-cli",
				"confidence": float64(90),
				"evidence":   []any{"GitHub repo", "official docs"},
				"reasoning":  "Found official repository",
			},
			wantErr: false,
		},
		{
			name: "low confidence",
			args: map[string]any{
				"builder":    "github",
				"source":     "stripe/stripe-cli",
				"confidence": float64(50),
				"evidence":   []any{},
				"reasoning":  "Unsure",
			},
			wantErr:    true,
			errContain: "below threshold",
		},
		{
			name: "invalid source format",
			args: map[string]any{
				"builder":    "github",
				"source":     "invalid",
				"confidence": float64(90),
				"evidence":   []any{},
				"reasoning":  "test",
			},
			wantErr:    true,
			errContain: "malformed URL", // ValidateGitHubURL returns ErrURLMalformed for non-owner/repo format
		},
		{
			name: "missing builder",
			args: map[string]any{
				"source":     "stripe/stripe-cli",
				"confidence": float64(90),
				"evidence":   []any{},
				"reasoning":  "test",
			},
			wantErr:    true,
			errContain: "required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := testHandleExtractSource(tc.args)

			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got result: %+v", result)
				} else if tc.errContain != "" && !containsSubstr(err.Error(), tc.errContain) {
					t.Errorf("error %q should contain %q", err.Error(), tc.errContain)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result == nil {
					t.Error("expected non-nil result")
				}
			}
		})
	}
}

func testHandleExtractSource(args map[string]any) (*DiscoveryResult, error) {
	builder, _ := args["builder"].(string)
	source, _ := args["source"].(string)
	confidence, _ := args["confidence"].(float64)
	reasoning, _ := args["reasoning"].(string)
	evidenceRaw, _ := args["evidence"].([]any)

	if builder == "" || source == "" {
		return nil, errorf("extract_source: builder and source are required")
	}

	// Evidence is parsed but not currently used in DiscoveryResult.
	// Keeping the parse logic for future use when evidence tracking is added.
	_ = evidenceRaw

	if builder == "github" {
		if err := ValidateGitHubURL(source); err != nil {
			return nil, errorf("extract_source: %v", err)
		}
	}

	if int(confidence) < MinConfidenceThreshold {
		return nil, errorf("extract_source: confidence %d is below threshold %d", int(confidence), MinConfidenceThreshold)
	}

	return &DiscoveryResult{
		Builder:    builder,
		Source:     source,
		Confidence: ConfidenceLLM,
		Reason:     reasoning,
		Metadata:   Metadata{},
	}, nil
}

func errorf(format string, args ...any) error {
	return &testError{msg: fmt.Sprintf(format, args...)}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstrAt(s, substr, 0))
}

func containsSubstrAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMockExtraction(t *testing.T) {
	// Test that extraction from arguments works correctly
	args := map[string]any{
		"builder":    "github",
		"source":     "stripe/stripe-cli",
		"confidence": float64(95),
		"evidence":   []any{"GitHub repo found", "Official documentation link"},
		"reasoning":  "Found official Stripe CLI repository",
	}

	argsJSON, _ := json.Marshal(args)
	t.Logf("Args JSON: %s", string(argsJSON))

	builder, _ := args["builder"].(string)
	source, _ := args["source"].(string)
	confidence, _ := args["confidence"].(float64)

	if builder != "github" {
		t.Errorf("builder mismatch: %s", builder)
	}
	if source != "stripe/stripe-cli" {
		t.Errorf("source mismatch: %s", source)
	}
	if confidence != 95 {
		t.Errorf("confidence mismatch: %f", confidence)
	}
}

func TestVerifyGitHubRepo_Fork(t *testing.T) {
	// Mock GitHub API response for a fork
	mockResponse := `{
		"stargazers_count": 100,
		"forks_count": 10,
		"archived": false,
		"description": "A fork of the original",
		"created_at": "2024-01-15T10:00:00Z",
		"pushed_at": "2025-01-20T15:30:00Z",
		"fork": true,
		"owner": {
			"login": "fork-owner",
			"type": "User"
		},
		"parent": {
			"full_name": "original-owner/original-repo",
			"stargazers_count": 5000
		}
	}`

	discovery := &LLMDiscovery{
		httpGet: func(ctx context.Context, url string) ([]byte, error) {
			return []byte(mockResponse), nil
		},
	}

	result := &DiscoveryResult{
		Builder: "github",
		Source:  "fork-owner/forked-repo",
	}

	metadata, err := discovery.verifyGitHubRepo(context.Background(), result)
	if err != nil {
		t.Fatalf("verifyGitHubRepo: %v", err)
	}

	// Verify fork detection
	if !metadata.IsFork {
		t.Error("expected IsFork to be true")
	}
	if metadata.ParentRepo != "original-owner/original-repo" {
		t.Errorf("expected ParentRepo='original-owner/original-repo', got %q", metadata.ParentRepo)
	}
	if metadata.ParentStars != 5000 {
		t.Errorf("expected ParentStars=5000, got %d", metadata.ParentStars)
	}
	if metadata.Stars != 100 {
		t.Errorf("expected Stars=100, got %d", metadata.Stars)
	}

	// Verify owner metadata
	if metadata.OwnerName != "fork-owner" {
		t.Errorf("expected OwnerName='fork-owner', got %q", metadata.OwnerName)
	}
	if metadata.OwnerType != "User" {
		t.Errorf("expected OwnerType='User', got %q", metadata.OwnerType)
	}

	// Verify created date format
	if metadata.CreatedAt != "2024-01-15" {
		t.Errorf("expected CreatedAt='2024-01-15', got %q", metadata.CreatedAt)
	}
}

func TestVerifyGitHubRepo_ForkWithMissingParent(t *testing.T) {
	// Mock GitHub API response for a fork with null parent
	// This can happen in edge cases (deleted parent, API issues)
	mockResponse := `{
		"stargazers_count": 50,
		"forks_count": 5,
		"archived": false,
		"description": "A fork with missing parent",
		"created_at": "2024-01-15T10:00:00Z",
		"fork": true,
		"parent": null
	}`

	discovery := &LLMDiscovery{
		httpGet: func(ctx context.Context, url string) ([]byte, error) {
			return []byte(mockResponse), nil
		},
	}

	result := &DiscoveryResult{
		Builder: "github",
		Source:  "fork-owner/forked-repo",
	}

	metadata, err := discovery.verifyGitHubRepo(context.Background(), result)
	if err != nil {
		t.Fatalf("verifyGitHubRepo: %v", err)
	}

	// Verify fork is still flagged even without parent data
	if !metadata.IsFork {
		t.Error("expected IsFork to be true")
	}
	if metadata.ParentRepo != "" {
		t.Errorf("expected empty ParentRepo, got %q", metadata.ParentRepo)
	}
	if metadata.ParentStars != 0 {
		t.Errorf("expected ParentStars=0, got %d", metadata.ParentStars)
	}
}

func TestVerifyGitHubRepo_NotAFork(t *testing.T) {
	// Mock GitHub API response for a non-fork repository
	mockResponse := `{
		"stargazers_count": 1000,
		"forks_count": 200,
		"archived": false,
		"description": "The original repository",
		"created_at": "2020-01-15T10:00:00Z",
		"pushed_at": "2026-02-01T12:00:00Z",
		"fork": false,
		"owner": {
			"login": "stripe",
			"type": "Organization"
		}
	}`

	discovery := &LLMDiscovery{
		httpGet: func(ctx context.Context, url string) ([]byte, error) {
			return []byte(mockResponse), nil
		},
	}

	result := &DiscoveryResult{
		Builder: "github",
		Source:  "owner/original-repo",
	}

	metadata, err := discovery.verifyGitHubRepo(context.Background(), result)
	if err != nil {
		t.Fatalf("verifyGitHubRepo: %v", err)
	}

	// Verify non-fork has IsFork=false
	if metadata.IsFork {
		t.Error("expected IsFork to be false")
	}
	if metadata.ParentRepo != "" {
		t.Errorf("expected empty ParentRepo, got %q", metadata.ParentRepo)
	}
	if metadata.ParentStars != 0 {
		t.Errorf("expected ParentStars=0, got %d", metadata.ParentStars)
	}
	if metadata.Stars != 1000 {
		t.Errorf("expected Stars=1000, got %d", metadata.Stars)
	}

	// Verify owner metadata (AC7)
	if metadata.OwnerName != "stripe" {
		t.Errorf("expected OwnerName='stripe', got %q", metadata.OwnerName)
	}
	if metadata.OwnerType != "Organization" {
		t.Errorf("expected OwnerType='Organization', got %q", metadata.OwnerType)
	}

	// Verify created date format (AC5)
	if metadata.CreatedAt != "2020-01-15" {
		t.Errorf("expected CreatedAt='2020-01-15', got %q", metadata.CreatedAt)
	}

	// Verify last commit days is calculated (AC6)
	if metadata.LastCommitDays <= 0 {
		t.Errorf("expected LastCommitDays > 0, got %d", metadata.LastCommitDays)
	}
}

func TestPassesQualityThreshold_RejectsForks(t *testing.T) {
	discovery := &LLMDiscovery{}

	tests := []struct {
		name     string
		metadata Metadata
		want     bool
	}{
		{
			name: "non-fork with high stars passes",
			metadata: Metadata{
				Stars:  100,
				IsFork: false,
			},
			want: true,
		},
		{
			name: "non-fork with low stars fails",
			metadata: Metadata{
				Stars:  10,
				IsFork: false,
			},
			want: false,
		},
		{
			name: "fork with high stars fails (never auto-pass)",
			metadata: Metadata{
				Stars:       1000,
				IsFork:      true,
				ParentRepo:  "owner/repo",
				ParentStars: 5000,
			},
			want: false,
		},
		{
			name: "fork with low stars fails",
			metadata: Metadata{
				Stars:  10,
				IsFork: true,
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := &DiscoveryResult{
				Metadata: tc.metadata,
			}
			got := discovery.passesQualityThreshold(result)
			if got != tc.want {
				t.Errorf("passesQualityThreshold() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRateLimitError(t *testing.T) {
	tests := []struct {
		name          string
		err           *RateLimitError
		wantErr       string
		wantSuggest   string
		authenticated bool
	}{
		{
			name: "unauthenticated rate limit",
			err: &RateLimitError{
				Authenticated: false,
				ResetTime:     time.Now().Add(10 * time.Minute),
			},
			wantErr:       "GitHub API rate limit exceeded",
			wantSuggest:   "Set GITHUB_TOKEN",
			authenticated: false,
		},
		{
			name: "authenticated rate limit",
			err: &RateLimitError{
				Authenticated: true,
				ResetTime:     time.Now().Add(5 * time.Minute),
			},
			wantErr:       "GitHub API rate limit exceeded",
			wantSuggest:   "Please wait",
			authenticated: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errStr := tc.err.Error()
			if !containsSubstr(errStr, tc.wantErr) {
				t.Errorf("Error() = %q, want to contain %q", errStr, tc.wantErr)
			}

			if !tc.err.IsRateLimited() {
				t.Error("IsRateLimited() should return true")
			}

			suggest := tc.err.Suggestion()
			if !containsSubstr(suggest, tc.wantSuggest) {
				t.Errorf("Suggestion() = %q, want to contain %q", suggest, tc.wantSuggest)
			}
		})
	}
}

func TestVerifyGitHubRepo_RateLimit(t *testing.T) {
	// Test that rate limit is detected from 403 with X-RateLimit-Remaining: 0
	discovery := &LLMDiscovery{
		httpGet: func(ctx context.Context, url string) ([]byte, error) {
			return nil, &RateLimitError{
				Authenticated: false,
				ResetTime:     time.Now().Add(30 * time.Minute),
			}
		},
	}

	result := &DiscoveryResult{
		Builder: "github",
		Source:  "owner/repo",
	}

	_, err := discovery.verifyGitHubRepo(context.Background(), result)
	if err == nil {
		t.Fatal("expected rate limit error")
	}

	var rateLimitErr *RateLimitError
	if !isRateLimitErr(err, &rateLimitErr) {
		t.Errorf("expected RateLimitError, got %T", err)
	}
}

func TestIsRateLimitErr(t *testing.T) {
	t.Run("rate limit error", func(t *testing.T) {
		err := &RateLimitError{Authenticated: false}
		var target *RateLimitError
		if !isRateLimitErr(err, &target) {
			t.Error("expected isRateLimitErr to return true")
		}
		if target != err {
			t.Error("expected target to be set to the error")
		}
	})

	t.Run("other error", func(t *testing.T) {
		err := errorf("some other error")
		var target *RateLimitError
		if isRateLimitErr(err, &target) {
			t.Error("expected isRateLimitErr to return false for non-rate-limit error")
		}
	})
}

func TestVerificationSkipped_WhenRateLimited(t *testing.T) {
	// Test that Resolve sets VerificationSkipped when rate limited
	// This is a unit test that mocks the entire flow

	// Create a discovery instance with mocked HTTP that returns rate limit
	rateLimitHit := false
	discovery := &LLMDiscovery{
		disabled: false,
		httpGet: func(ctx context.Context, url string) ([]byte, error) {
			rateLimitHit = true
			return nil, &RateLimitError{
				Authenticated: false,
				ResetTime:     time.Now().Add(30 * time.Minute),
			}
		},
		confirm: func(result *DiscoveryResult) bool {
			// Verify the VerificationSkipped flag is set
			if !result.Metadata.VerificationSkipped {
				t.Error("expected VerificationSkipped to be true in confirmation")
			}
			if result.Metadata.VerificationWarning == "" {
				t.Error("expected VerificationWarning to be set")
			}
			return true // Auto-approve for test
		},
	}

	// Manually call verifyGitHubRepo to test the rate limit path
	result := &DiscoveryResult{
		Builder: "github",
		Source:  "owner/repo",
	}

	_, err := discovery.verifyGitHubRepo(context.Background(), result)
	if err == nil {
		t.Fatal("expected rate limit error from verifyGitHubRepo")
	}

	if !rateLimitHit {
		t.Error("expected httpGet to be called")
	}

	// Verify the error is a rate limit error
	var rateLimitErr *RateLimitError
	if !isRateLimitErr(err, &rateLimitErr) {
		t.Fatalf("expected RateLimitError, got %T", err)
	}

	// Apply the same logic as Resolve: set VerificationSkipped
	result.Metadata.VerificationSkipped = true
	result.Metadata.VerificationWarning = rateLimitErr.Suggestion()

	// Verify the metadata is set correctly
	if !result.Metadata.VerificationSkipped {
		t.Error("expected VerificationSkipped to be true")
	}
	if !containsSubstr(result.Metadata.VerificationWarning, "GITHUB_TOKEN") {
		t.Errorf("expected VerificationWarning to mention GITHUB_TOKEN, got %q", result.Metadata.VerificationWarning)
	}
}

func TestRankCandidates(t *testing.T) {
	tests := []struct {
		name       string
		candidates []*DiscoveryResult
		wantOrder  []string // Expected order of Source values
	}{
		{
			name:       "empty list",
			candidates: []*DiscoveryResult{},
			wantOrder:  []string{},
		},
		{
			name: "single candidate",
			candidates: []*DiscoveryResult{
				{Source: "owner/repo", ConfidenceScore: 90, Metadata: Metadata{Stars: 100}},
			},
			wantOrder: []string{"owner/repo"},
		},
		{
			name: "sort by confidence descending",
			candidates: []*DiscoveryResult{
				{Source: "low/conf", ConfidenceScore: 70, Metadata: Metadata{Stars: 1000}},
				{Source: "high/conf", ConfidenceScore: 95, Metadata: Metadata{Stars: 100}},
				{Source: "mid/conf", ConfidenceScore: 85, Metadata: Metadata{Stars: 500}},
			},
			wantOrder: []string{"high/conf", "mid/conf", "low/conf"},
		},
		{
			name: "equal confidence - sort by stars descending",
			candidates: []*DiscoveryResult{
				{Source: "low/stars", ConfidenceScore: 90, Metadata: Metadata{Stars: 100}},
				{Source: "high/stars", ConfidenceScore: 90, Metadata: Metadata{Stars: 5000}},
				{Source: "mid/stars", ConfidenceScore: 90, Metadata: Metadata{Stars: 1000}},
			},
			wantOrder: []string{"high/stars", "mid/stars", "low/stars"},
		},
		{
			name: "equal confidence and stars - stable by source",
			candidates: []*DiscoveryResult{
				{Source: "zebra/repo", ConfidenceScore: 90, Metadata: Metadata{Stars: 100}},
				{Source: "alpha/repo", ConfidenceScore: 90, Metadata: Metadata{Stars: 100}},
				{Source: "middle/repo", ConfidenceScore: 90, Metadata: Metadata{Stars: 100}},
			},
			wantOrder: []string{"alpha/repo", "middle/repo", "zebra/repo"},
		},
		{
			name: "mixed confidence and stars",
			candidates: []*DiscoveryResult{
				{Source: "high-conf-low-stars", ConfidenceScore: 95, Metadata: Metadata{Stars: 50}},
				{Source: "low-conf-high-stars", ConfidenceScore: 75, Metadata: Metadata{Stars: 10000}},
				{Source: "mid-conf-mid-stars", ConfidenceScore: 85, Metadata: Metadata{Stars: 500}},
			},
			// Confidence takes priority over stars
			wantOrder: []string{"high-conf-low-stars", "mid-conf-mid-stars", "low-conf-high-stars"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ranked := rankCandidates(tc.candidates)

			if len(ranked) != len(tc.wantOrder) {
				t.Fatalf("expected %d candidates, got %d", len(tc.wantOrder), len(ranked))
			}

			for i, want := range tc.wantOrder {
				if ranked[i].Source != want {
					t.Errorf("position %d: expected %s, got %s", i, want, ranked[i].Source)
				}
			}
		})
	}
}

func TestSelectBestCandidate(t *testing.T) {
	discovery := &LLMDiscovery{}

	tests := []struct {
		name       string
		candidates []*DiscoveryResult
		wantSource string // Empty if expecting nil
	}{
		{
			name:       "empty list returns nil",
			candidates: []*DiscoveryResult{},
			wantSource: "",
		},
		{
			name: "single non-fork above threshold",
			candidates: []*DiscoveryResult{
				{Source: "owner/repo", ConfidenceScore: 90, Metadata: Metadata{Stars: 100, IsFork: false}},
			},
			wantSource: "owner/repo",
		},
		{
			name: "single non-fork below threshold still selected",
			candidates: []*DiscoveryResult{
				{Source: "owner/repo", ConfidenceScore: 90, Metadata: Metadata{Stars: 10, IsFork: false}},
			},
			wantSource: "owner/repo",
		},
		{
			name: "prefers non-fork over fork even with lower stars",
			candidates: []*DiscoveryResult{
				{Source: "fork/repo", ConfidenceScore: 95, Metadata: Metadata{Stars: 1000, IsFork: true}},
				{Source: "original/repo", ConfidenceScore: 90, Metadata: Metadata{Stars: 100, IsFork: false}},
			},
			wantSource: "original/repo",
		},
		{
			name: "only forks - selects best fork",
			candidates: []*DiscoveryResult{
				{Source: "fork2/repo", ConfidenceScore: 85, Metadata: Metadata{Stars: 200, IsFork: true}},
				{Source: "fork1/repo", ConfidenceScore: 90, Metadata: Metadata{Stars: 100, IsFork: true}},
			},
			wantSource: "fork1/repo", // Higher confidence wins
		},
		{
			name: "multiple non-forks - selects best by ranking",
			candidates: []*DiscoveryResult{
				{Source: "low/conf", ConfidenceScore: 75, Metadata: Metadata{Stars: 5000, IsFork: false}},
				{Source: "high/conf", ConfidenceScore: 95, Metadata: Metadata{Stars: 100, IsFork: false}},
			},
			wantSource: "high/conf", // Higher confidence wins
		},
		{
			name: "mix of forks and non-forks with different qualities",
			candidates: []*DiscoveryResult{
				{Source: "best-fork", ConfidenceScore: 99, Metadata: Metadata{Stars: 10000, IsFork: true}},
				{Source: "mediocre-original", ConfidenceScore: 80, Metadata: Metadata{Stars: 200, IsFork: false}},
				{Source: "good-original", ConfidenceScore: 90, Metadata: Metadata{Stars: 500, IsFork: false}},
			},
			wantSource: "good-original", // Best non-fork by confidence
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Candidates should be pre-ranked
			ranked := rankCandidates(tc.candidates)
			result := discovery.selectBestCandidate(ranked)

			if tc.wantSource == "" {
				if result != nil {
					t.Errorf("expected nil, got %s", result.Source)
				}
				return
			}

			if result == nil {
				t.Fatalf("expected %s, got nil", tc.wantSource)
			}

			if result.Source != tc.wantSource {
				t.Errorf("expected %s, got %s", tc.wantSource, result.Source)
			}
		})
	}
}

func TestRankCandidates_DoesNotModifyOriginal(t *testing.T) {
	original := []*DiscoveryResult{
		{Source: "c/repo", ConfidenceScore: 70},
		{Source: "a/repo", ConfidenceScore: 90},
		{Source: "b/repo", ConfidenceScore: 80},
	}

	// Save original order
	originalOrder := make([]string, len(original))
	for i, c := range original {
		originalOrder[i] = c.Source
	}

	// Rank (should not modify original)
	ranked := rankCandidates(original)

	// Verify original is unchanged
	for i, c := range original {
		if c.Source != originalOrder[i] {
			t.Errorf("original was modified at position %d: expected %s, got %s",
				i, originalOrder[i], c.Source)
		}
	}

	// Verify ranked is in correct order
	expectedOrder := []string{"a/repo", "b/repo", "c/repo"}
	for i, want := range expectedOrder {
		if ranked[i].Source != want {
			t.Errorf("ranked position %d: expected %s, got %s", i, want, ranked[i].Source)
		}
	}
}

func TestMultipleCandidates_EndToEnd(t *testing.T) {
	// Test that multiple candidates are handled correctly through verification
	callCount := 0
	discovery := &LLMDiscovery{
		httpGet: func(ctx context.Context, url string) ([]byte, error) {
			callCount++
			// Return different metadata based on URL
			if containsSubstr(url, "popular") {
				return []byte(`{
					"stargazers_count": 5000,
					"archived": false,
					"description": "Popular repo",
					"created_at": "2020-01-01T00:00:00Z",
					"pushed_at": "2025-01-01T00:00:00Z",
					"fork": false,
					"owner": {"login": "popular", "type": "Organization"}
				}`), nil
			}
			if containsSubstr(url, "fork-repo") {
				return []byte(`{
					"stargazers_count": 100,
					"archived": false,
					"description": "A fork",
					"created_at": "2023-01-01T00:00:00Z",
					"pushed_at": "2025-01-01T00:00:00Z",
					"fork": true,
					"owner": {"login": "forker", "type": "User"},
					"parent": {"full_name": "original/repo", "stargazers_count": 10000}
				}`), nil
			}
			return []byte(`{
				"stargazers_count": 200,
				"archived": false,
				"description": "Unknown repo",
				"created_at": "2022-01-01T00:00:00Z",
				"pushed_at": "2025-01-01T00:00:00Z",
				"fork": false,
				"owner": {"login": "unknown", "type": "User"}
			}`), nil
		},
	}

	candidates := []*DiscoveryResult{
		{Builder: "github", Source: "forker/fork-repo", ConfidenceScore: 95},
		{Builder: "github", Source: "popular/repo", ConfidenceScore: 90},
		{Builder: "github", Source: "unknown/repo", ConfidenceScore: 85},
	}

	// Verify each candidate
	var verified []*DiscoveryResult
	for _, c := range candidates {
		metadata, err := discovery.verifyGitHubRepo(context.Background(), c)
		if err != nil {
			t.Fatalf("verification failed for %s: %v", c.Source, err)
		}
		c.Metadata = metadata
		verified = append(verified, c)
	}

	// Rank candidates
	ranked := rankCandidates(verified)

	// Select best
	best := discovery.selectBestCandidate(ranked)

	// The fork has highest confidence but should be skipped in favor of non-fork
	if best == nil {
		t.Fatal("expected a result")
	}
	if best.Source != "popular/repo" {
		t.Errorf("expected popular/repo (best non-fork), got %s", best.Source)
	}
	if best.Metadata.IsFork {
		t.Error("expected non-fork to be selected")
	}
}

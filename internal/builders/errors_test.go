package builders

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRateLimitError(t *testing.T) {
	err := &RateLimitError{
		Limit:      10,
		Count:      10,
		RetryAfter: 23 * time.Minute,
		ConfigKey:  "llm.hourly_rate_limit",
	}

	// Test Error()
	errMsg := err.Error()
	if !strings.Contains(errMsg, "rate limit exceeded") {
		t.Errorf("Error() should contain 'rate limit exceeded', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "10 generations per hour") {
		t.Errorf("Error() should contain limit, got: %s", errMsg)
	}

	// Test Suggestion()
	suggestion := err.Suggestion()
	if !strings.Contains(suggestion, "23 minutes") {
		t.Errorf("Suggestion() should contain retry time, got: %s", suggestion)
	}
	if !strings.Contains(suggestion, "tsuku config set llm.hourly_rate_limit") {
		t.Errorf("Suggestion() should contain config command, got: %s", suggestion)
	}
}

func TestBudgetError(t *testing.T) {
	err := &BudgetError{
		Budget:    5.00,
		Spent:     5.00,
		ConfigKey: "llm.daily_budget",
	}

	// Test Error()
	errMsg := err.Error()
	if !strings.Contains(errMsg, "daily LLM budget exhausted") {
		t.Errorf("Error() should contain 'daily LLM budget exhausted', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "$5.00") {
		t.Errorf("Error() should contain spent amount, got: %s", errMsg)
	}

	// Test Suggestion()
	suggestion := err.Suggestion()
	if !strings.Contains(suggestion, "midnight UTC") {
		t.Errorf("Suggestion() should mention reset time, got: %s", suggestion)
	}
	if !strings.Contains(suggestion, "tsuku config set llm.daily_budget") {
		t.Errorf("Suggestion() should contain config command, got: %s", suggestion)
	}
}

func TestGitHubRateLimitError(t *testing.T) {
	underlying := errors.New("HTTP 429")

	tests := []struct {
		name          string
		authenticated bool
		retryAfter    time.Duration
		wantToken     bool
	}{
		{
			name:          "unauthenticated",
			authenticated: false,
			retryAfter:    45 * time.Minute,
			wantToken:     true,
		},
		{
			name:          "authenticated",
			authenticated: true,
			retryAfter:    10 * time.Minute,
			wantToken:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &GitHubRateLimitError{
				RetryAfter:    tt.retryAfter,
				Authenticated: tt.authenticated,
				Err:           underlying,
			}

			// Test Error()
			errMsg := err.Error()
			if !strings.Contains(errMsg, "GitHub API rate limit exceeded") {
				t.Errorf("Error() should mention rate limit, got: %s", errMsg)
			}

			// Test Suggestion()
			suggestion := err.Suggestion()
			if !strings.Contains(suggestion, "Try again in") {
				t.Errorf("Suggestion() should mention retry time, got: %s", suggestion)
			}
			if tt.wantToken && !strings.Contains(suggestion, "GITHUB_TOKEN") {
				t.Errorf("Suggestion() should mention GITHUB_TOKEN for unauthenticated, got: %s", suggestion)
			}
			if !tt.wantToken && strings.Contains(suggestion, "GITHUB_TOKEN") {
				t.Errorf("Suggestion() should not mention GITHUB_TOKEN for authenticated, got: %s", suggestion)
			}

			// Test Unwrap()
			if err.Unwrap() != underlying {
				t.Error("Unwrap() should return underlying error")
			}
		})
	}
}

func TestGitHubRepoNotFoundError(t *testing.T) {
	err := &GitHubRepoNotFoundError{
		Owner: "owner",
		Repo:  "repo",
	}

	// Test Error()
	errMsg := err.Error()
	if !strings.Contains(errMsg, "repository not found") {
		t.Errorf("Error() should mention 'repository not found', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "owner/repo") {
		t.Errorf("Error() should contain owner/repo, got: %s", errMsg)
	}

	// Test Suggestion()
	suggestion := err.Suggestion()
	if !strings.Contains(suggestion, "Check spelling") {
		t.Errorf("Suggestion() should mention checking spelling, got: %s", suggestion)
	}
	if !strings.Contains(suggestion, "public") {
		t.Errorf("Suggestion() should mention verifying repository is public, got: %s", suggestion)
	}
}

func TestLLMAuthError(t *testing.T) {
	underlying := errors.New("invalid API key")
	err := &LLMAuthError{
		Provider: "anthropic",
		EnvVar:   "ANTHROPIC_API_KEY",
		DocsURL:  "https://docs.anthropic.com/en/api/getting-started",
		Err:      underlying,
	}

	// Test Error()
	errMsg := err.Error()
	if !strings.Contains(errMsg, "LLM API authentication failed") {
		t.Errorf("Error() should mention authentication failed, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "anthropic") {
		t.Errorf("Error() should contain provider name, got: %s", errMsg)
	}

	// Test Suggestion()
	suggestion := err.Suggestion()
	if !strings.Contains(suggestion, "ANTHROPIC_API_KEY") {
		t.Errorf("Suggestion() should mention env var, got: %s", suggestion)
	}
	if !strings.Contains(suggestion, "https://docs.anthropic.com") {
		t.Errorf("Suggestion() should contain docs URL, got: %s", suggestion)
	}

	// Test Unwrap()
	if err.Unwrap() != underlying {
		t.Error("Unwrap() should return underlying error")
	}
}

func TestSandboxError(t *testing.T) {
	underlying := errors.New("exit code 1")
	err := &SandboxError{
		Tool:           "mytool",
		RepairAttempts: 3,
		LastOutput:     "command not found",
		Err:            underlying,
	}

	// Test Error()
	errMsg := err.Error()
	if !strings.Contains(errMsg, "recipe sandbox testing failed") {
		t.Errorf("Error() should mention sandbox testing failed, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "3 repair attempts") {
		t.Errorf("Error() should contain repair attempts, got: %s", errMsg)
	}

	// Test Suggestion()
	suggestion := err.Suggestion()
	if !strings.Contains(suggestion, "could not be automatically fixed") {
		t.Errorf("Suggestion() should explain the situation, got: %s", suggestion)
	}
	if !strings.Contains(suggestion, "--skip-sandbox") {
		t.Errorf("Suggestion() should mention --skip-sandbox, got: %s", suggestion)
	}
	if !strings.Contains(suggestion, "mytool") {
		t.Errorf("Suggestion() should contain tool name, got: %s", suggestion)
	}
	if !strings.Contains(suggestion, "github.com/tsukumogami/tsuku/issues/new") {
		t.Errorf("Suggestion() should contain issue URL, got: %s", suggestion)
	}

	// Test Unwrap()
	if err.Unwrap() != underlying {
		t.Error("Unwrap() should return underlying error")
	}
}

func TestRateLimitError_ConfirmationPrompt(t *testing.T) {
	err := &RateLimitError{
		Limit: 10,
		Count: 10,
	}
	prompt := err.ConfirmationPrompt()
	if !strings.Contains(prompt, "Rate limit exceeded") {
		t.Errorf("ConfirmationPrompt() should mention rate limit, got: %s", prompt)
	}
}

func TestBudgetError_ConfirmationPrompt(t *testing.T) {
	err := &BudgetError{
		Budget: 5.00,
		Spent:  5.00,
	}
	prompt := err.ConfirmationPrompt()
	if !strings.Contains(prompt, "budget exhausted") {
		t.Errorf("ConfirmationPrompt() should mention budget, got: %s", prompt)
	}
	if !strings.Contains(prompt, "$5.00") {
		t.Errorf("ConfirmationPrompt() should contain spent amount, got: %s", prompt)
	}
}

func TestRateLimitError_ShortRetryAfter(t *testing.T) {
	err := &RateLimitError{
		Limit:      10,
		Count:      10,
		RetryAfter: 30 * time.Second,
		ConfigKey:  "llm.hourly_rate_limit",
	}
	suggestion := err.Suggestion()
	if !strings.Contains(suggestion, "1 minutes") {
		t.Errorf("Suggestion() should show at least 1 minute, got: %s", suggestion)
	}
}

func TestGitHubRateLimitError_ShortRetryAfter(t *testing.T) {
	err := &GitHubRateLimitError{
		RetryAfter:    10 * time.Second,
		Authenticated: true,
	}
	suggestion := err.Suggestion()
	if !strings.Contains(suggestion, "1 minutes") {
		t.Errorf("Suggestion() should show at least 1 minute, got: %s", suggestion)
	}
}

func TestDeterministicFailedError(t *testing.T) {
	underlying := errors.New("no bottles available")
	err := &DeterministicFailedError{
		Formula:  "fzf",
		Category: FailureCategoryNoBottles,
		Message:  "no pre-built bottles for this platform",
		Err:      underlying,
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "fzf") {
		t.Errorf("Error() should contain formula name, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "no pre-built bottles") {
		t.Errorf("Error() should contain message, got: %s", errMsg)
	}

	if err.Unwrap() != underlying {
		t.Error("Unwrap() should return underlying error")
	}
}

func TestGitHubRepoNotFoundError_Unwrap(t *testing.T) {
	underlying := errors.New("404 not found")
	err := &GitHubRepoNotFoundError{
		Owner: "owner",
		Repo:  "repo",
		Err:   underlying,
	}
	if err.Unwrap() != underlying {
		t.Error("Unwrap() should return underlying error")
	}
}

func TestLLMAuthError_Unwrap(t *testing.T) {
	underlying := errors.New("unauthorized")
	err := &LLMAuthError{
		Provider: "test",
		EnvVar:   "TEST_KEY",
		DocsURL:  "https://example.com",
		Err:      underlying,
	}
	if err.Unwrap() != underlying {
		t.Error("Unwrap() should return underlying error")
	}
}

func TestSandboxError_Unwrap(t *testing.T) {
	underlying := errors.New("exit 1")
	err := &SandboxError{
		Tool: "test",
		Err:  underlying,
	}
	if err.Unwrap() != underlying {
		t.Error("Unwrap() should return underlying error")
	}
}

func TestRepairNotSupportedError(t *testing.T) {
	err := &RepairNotSupportedError{BuilderType: "ecosystem"}

	msg := err.Error()
	if !strings.Contains(msg, "ecosystem") {
		t.Errorf("Error() should contain builder type, got: %s", msg)
	}
	if !strings.Contains(msg, "do not support repair") {
		t.Errorf("Error() should explain repair not supported, got: %s", msg)
	}
}

func TestRepairNotSupportedError_Is(t *testing.T) {
	err1 := &RepairNotSupportedError{BuilderType: "ecosystem"}
	err2 := &RepairNotSupportedError{BuilderType: "github"}

	// Is() should match any RepairNotSupportedError regardless of BuilderType
	if !err1.Is(err2) {
		t.Error("Is() should match other RepairNotSupportedError instances")
	}

	// Should not match regular errors
	if err1.Is(errors.New("other")) {
		t.Error("Is() should not match non-RepairNotSupportedError")
	}

	// errors.Is should work
	if !errors.Is(err1, &RepairNotSupportedError{}) {
		t.Error("errors.Is() should match RepairNotSupportedError")
	}
}

func TestLLMDisabledError(t *testing.T) {
	err := &LLMDisabledError{}
	msg := err.Error()
	if !strings.Contains(msg, "LLM features are disabled") {
		t.Errorf("Error() should explain LLM disabled, got: %s", msg)
	}
	if !strings.Contains(msg, "tsuku config set llm.enabled true") {
		t.Errorf("Error() should contain enablement command, got: %s", msg)
	}
}

func TestRecordLLMCost(t *testing.T) {
	// nil tracker should return nil
	if err := RecordLLMCost(nil, 1.0); err != nil {
		t.Errorf("RecordLLMCost(nil, 1.0) error = %v, want nil", err)
	}

	// Zero cost should return nil
	tracker := &mockLLMTracker{}
	if err := RecordLLMCost(tracker, 0); err != nil {
		t.Errorf("RecordLLMCost(tracker, 0) error = %v, want nil", err)
	}
	if tracker.recordedCost != 0 {
		t.Error("should not record zero cost")
	}

	// Negative cost should return nil
	if err := RecordLLMCost(tracker, -1); err != nil {
		t.Errorf("RecordLLMCost(tracker, -1) error = %v, want nil", err)
	}

	// Positive cost should be recorded
	if err := RecordLLMCost(tracker, 0.5); err != nil {
		t.Errorf("RecordLLMCost(tracker, 0.5) error = %v, want nil", err)
	}
	if tracker.recordedCost != 0.5 {
		t.Errorf("recorded cost = %v, want 0.5", tracker.recordedCost)
	}
}

func TestCheckLLMPrerequisites_NilOpts(t *testing.T) {
	if err := CheckLLMPrerequisites(nil); err != nil {
		t.Errorf("CheckLLMPrerequisites(nil) error = %v, want nil", err)
	}
}

func TestCheckLLMPrerequisites_ForceInit(t *testing.T) {
	opts := &SessionOptions{ForceInit: true}
	if err := CheckLLMPrerequisites(opts); err != nil {
		t.Errorf("CheckLLMPrerequisites with ForceInit error = %v, want nil", err)
	}
}

func TestCheckLLMPrerequisites_LLMDisabled(t *testing.T) {
	cfg := &mockLLMConfig{enabled: false}
	opts := &SessionOptions{LLMConfig: cfg}

	err := CheckLLMPrerequisites(opts)
	if err == nil {
		t.Error("expected error when LLM disabled")
	}

	var llmErr *LLMDisabledError
	if !errors.As(err, &llmErr) {
		t.Errorf("expected LLMDisabledError, got %T", err)
	}
}

func TestCheckLLMPrerequisites_BudgetExceeded(t *testing.T) {
	cfg := &mockLLMConfig{enabled: true, dailyBudget: 5.0, hourlyLimit: 10}
	tracker := &mockLLMTracker{
		canGenerate:    false,
		denyReason:     "daily budget exceeded",
		dailySpent:     5.5,
		recentGenCount: 3,
	}
	opts := &SessionOptions{LLMConfig: cfg, LLMStateTracker: tracker}

	err := CheckLLMPrerequisites(opts)
	if err == nil {
		t.Error("expected error when budget exceeded")
	}

	var budgetErr *BudgetError
	if !errors.As(err, &budgetErr) {
		t.Errorf("expected BudgetError, got %T", err)
	}
}

func TestCheckLLMPrerequisites_RateLimitExceeded(t *testing.T) {
	cfg := &mockLLMConfig{enabled: true, dailyBudget: 5.0, hourlyLimit: 10}
	tracker := &mockLLMTracker{
		canGenerate:    false,
		denyReason:     "rate limit exceeded",
		dailySpent:     1.0,
		recentGenCount: 10,
	}
	opts := &SessionOptions{LLMConfig: cfg, LLMStateTracker: tracker}

	err := CheckLLMPrerequisites(opts)
	if err == nil {
		t.Error("expected error when rate limit exceeded")
	}

	var rateErr *RateLimitError
	if !errors.As(err, &rateErr) {
		t.Errorf("expected RateLimitError, got %T", err)
	}
}

func TestCheckLLMPrerequisites_Allowed(t *testing.T) {
	cfg := &mockLLMConfig{enabled: true, dailyBudget: 5.0, hourlyLimit: 10}
	tracker := &mockLLMTracker{
		canGenerate:    true,
		dailySpent:     1.0,
		recentGenCount: 3,
	}
	opts := &SessionOptions{LLMConfig: cfg, LLMStateTracker: tracker}

	if err := CheckLLMPrerequisites(opts); err != nil {
		t.Errorf("expected nil error when generation allowed, got %v", err)
	}
}

// Mock types for testing

type mockLLMConfig struct {
	enabled     bool
	dailyBudget float64
	hourlyLimit int
}

func (m *mockLLMConfig) LLMEnabled() bool        { return m.enabled }
func (m *mockLLMConfig) LLMDailyBudget() float64 { return m.dailyBudget }
func (m *mockLLMConfig) LLMHourlyRateLimit() int { return m.hourlyLimit }

type mockLLMTracker struct {
	canGenerate    bool
	denyReason     string
	dailySpent     float64
	recentGenCount int
	recordedCost   float64
}

func (m *mockLLMTracker) CanGenerate(hourlyLimit int, dailyBudget float64) (bool, string) {
	return m.canGenerate, m.denyReason
}

func (m *mockLLMTracker) RecordGeneration(cost float64) error {
	m.recordedCost = cost
	return nil
}

func (m *mockLLMTracker) DailySpent() float64        { return m.dailySpent }
func (m *mockLLMTracker) RecentGenerationCount() int { return m.recentGenCount }

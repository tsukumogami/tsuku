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

package builders

import (
	"fmt"
	"time"
)

// RateLimitError indicates the hourly LLM generation rate limit was exceeded.
// Implements ConfirmableError to allow users to bypass the limit with confirmation.
type RateLimitError struct {
	Limit      int           // Maximum generations per hour
	Count      int           // Current count in the last hour
	RetryAfter time.Duration // Time until next generation is allowed
	ConfigKey  string        // Config key to adjust limit
}

// Error implements the error interface.
func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded (%d generations per hour)", e.Limit)
}

// Suggestion returns actionable steps for the user.
func (e *RateLimitError) Suggestion() string {
	minutes := int(e.RetryAfter.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("Next generation available in: %d minutes\nTo adjust: tsuku config set %s <new-limit>", minutes, e.ConfigKey)
}

// ConfirmationPrompt implements ConfirmableError.
func (e *RateLimitError) ConfirmationPrompt() string {
	return "Rate limit exceeded. Continue anyway?"
}

// BudgetError indicates the daily LLM budget was exhausted.
// Implements ConfirmableError to allow users to bypass the limit with confirmation.
type BudgetError struct {
	Budget    float64 // Daily budget in USD
	Spent     float64 // Amount spent today in USD
	ConfigKey string  // Config key to adjust budget
}

// Error implements the error interface.
func (e *BudgetError) Error() string {
	return fmt.Sprintf("daily LLM budget exhausted ($%.2f spent today)", e.Spent)
}

// Suggestion returns actionable steps for the user.
func (e *BudgetError) Suggestion() string {
	return fmt.Sprintf("Budget resets at midnight UTC.\nTo adjust: tsuku config set %s <new-budget>", e.ConfigKey)
}

// ConfirmationPrompt implements ConfirmableError.
func (e *BudgetError) ConfirmationPrompt() string {
	return fmt.Sprintf("Daily budget exhausted ($%.2f spent). Continue anyway?", e.Spent)
}

// GitHubRateLimitError indicates GitHub API rate limit was exceeded.
type GitHubRateLimitError struct {
	RetryAfter    time.Duration // Time until rate limit resets
	Authenticated bool          // Whether request was authenticated
	Err           error         // Underlying error
}

// Error implements the error interface.
func (e *GitHubRateLimitError) Error() string {
	return "GitHub API rate limit exceeded"
}

// Unwrap returns the underlying error.
func (e *GitHubRateLimitError) Unwrap() error {
	return e.Err
}

// Suggestion returns actionable steps for the user.
func (e *GitHubRateLimitError) Suggestion() string {
	minutes := int(e.RetryAfter.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	suggestion := fmt.Sprintf("Try again in: %d minutes", minutes)
	if !e.Authenticated {
		suggestion += "\nOr set GITHUB_TOKEN for higher limits (5000 req/hour)"
	}
	return suggestion
}

// GitHubRepoNotFoundError indicates the requested GitHub repository was not found.
type GitHubRepoNotFoundError struct {
	Owner string
	Repo  string
	Err   error
}

// Error implements the error interface.
func (e *GitHubRepoNotFoundError) Error() string {
	return fmt.Sprintf("repository not found: %s/%s", e.Owner, e.Repo)
}

// Unwrap returns the underlying error.
func (e *GitHubRepoNotFoundError) Unwrap() error {
	return e.Err
}

// Suggestion returns actionable steps for the user.
func (e *GitHubRepoNotFoundError) Suggestion() string {
	return "Check spelling or verify the repository is public"
}

// LLMAuthError indicates LLM API authentication failed.
type LLMAuthError struct {
	Provider string // LLM provider name (e.g., "anthropic", "gemini")
	EnvVar   string // Environment variable for API key
	DocsURL  string // Documentation URL
	Err      error  // Underlying error
}

// Error implements the error interface.
func (e *LLMAuthError) Error() string {
	return fmt.Sprintf("LLM API authentication failed (%s)", e.Provider)
}

// Unwrap returns the underlying error.
func (e *LLMAuthError) Unwrap() error {
	return e.Err
}

// Suggestion returns actionable steps for the user.
func (e *LLMAuthError) Suggestion() string {
	return fmt.Sprintf("Verify %s is set correctly\nDocs: %s", e.EnvVar, e.DocsURL)
}

// SandboxError indicates recipe sandbox testing failed after repair attempts.
type SandboxError struct {
	Tool           string // Tool name being created
	RepairAttempts int    // Number of repair attempts made
	LastOutput     string // Last sandbox output (truncated)
	Err            error  // Underlying error
}

// Error implements the error interface.
func (e *SandboxError) Error() string {
	return fmt.Sprintf("recipe sandbox testing failed after %d repair attempts", e.RepairAttempts)
}

// Unwrap returns the underlying error.
func (e *SandboxError) Unwrap() error {
	return e.Err
}

// Suggestion returns actionable steps for the user.
func (e *SandboxError) Suggestion() string {
	return fmt.Sprintf("The generated recipe could not be automatically fixed.\n\nTo skip sandbox testing (use with caution):\n  tsuku create %s --from github:<owner>/<repo> --skip-sandbox\n\nTo report this issue:\n  https://github.com/tsukumogami/tsuku/issues/new", e.Tool)
}

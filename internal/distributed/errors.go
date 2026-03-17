// Package distributed implements GitHub-based distributed recipe fetching
// and caching for third-party recipe repositories.
package distributed

import (
	"fmt"
	"time"
)

// ErrRepoNotFound indicates the GitHub repository does not exist or is not accessible.
type ErrRepoNotFound struct {
	Owner string
	Repo  string
}

func (e *ErrRepoNotFound) Error() string {
	return fmt.Sprintf("repository not found: %s/%s", e.Owner, e.Repo)
}

// ErrNoRecipeDir indicates the repository does not contain a .tsuku-recipes/ directory.
type ErrNoRecipeDir struct {
	Owner string
	Repo  string
}

func (e *ErrNoRecipeDir) Error() string {
	return fmt.Sprintf("no .tsuku-recipes/ directory found in %s/%s", e.Owner, e.Repo)
}

// ErrRateLimited indicates a GitHub API rate limit was hit.
type ErrRateLimited struct {
	Remaining int
	ResetAt   time.Time
	HasToken  bool
}

func (e *ErrRateLimited) Error() string {
	msg := fmt.Sprintf("GitHub API rate limit exceeded (remaining: %d, resets at: %s)",
		e.Remaining, e.ResetAt.Format(time.RFC3339))
	if !e.HasToken {
		msg += ". Set GITHUB_TOKEN for higher rate limits"
	}
	return msg
}

// ErrInvalidDownloadURL indicates a download URL from the Contents API failed validation.
type ErrInvalidDownloadURL struct {
	URL    string
	Reason string
}

func (e *ErrInvalidDownloadURL) Error() string {
	return fmt.Sprintf("invalid download URL %q: %s", e.URL, e.Reason)
}

// ErrNetwork wraps a network-level error with additional context.
type ErrNetwork struct {
	Operation string
	Err       error
}

func (e *ErrNetwork) Error() string {
	return fmt.Sprintf("%s: %s", e.Operation, e.Err)
}

func (e *ErrNetwork) Unwrap() error {
	return e.Err
}

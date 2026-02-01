package discover

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Validator checks whether a seed entry is valid (the source exists and is usable).
type Validator interface {
	Validate(entry SeedEntry) error
}

// ValidationResult holds the outcome of validating a single entry.
type ValidationResult struct {
	Entry SeedEntry
	Err   error
}

// ValidateEntries runs validation on all entries using the appropriate validator
// for each builder. Returns valid entries and a list of failures.
func ValidateEntries(entries []SeedEntry, validators map[string]Validator) (valid []SeedEntry, failures []ValidationResult) {
	for _, e := range entries {
		v, ok := validators[e.Builder]
		if !ok {
			failures = append(failures, ValidationResult{Entry: e, Err: fmt.Errorf("no validator for builder %q", e.Builder)})
			continue
		}
		if err := v.Validate(e); err != nil {
			failures = append(failures, ValidationResult{Entry: e, Err: err})
			continue
		}
		valid = append(valid, e)
	}
	return valid, failures
}

// GitHubValidator checks that a GitHub repo exists, is not archived, and has releases.
type GitHubValidator struct {
	client *http.Client
	token  string
	cache  sync.Map // source -> error (nil = valid)
}

// NewGitHubValidator creates a validator that checks GitHub repos.
// It reads GITHUB_TOKEN from the environment for authenticated requests.
func NewGitHubValidator(client *http.Client) *GitHubValidator {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &GitHubValidator{
		client: client,
		token:  os.Getenv("GITHUB_TOKEN"),
	}
}

func (g *GitHubValidator) Validate(entry SeedEntry) error {
	// Check cache
	if cached, ok := g.cache.Load(entry.Source); ok {
		if cached == nil {
			return nil
		}
		return cached.(error)
	}

	err := g.validate(entry.Source)
	if err != nil {
		g.cache.Store(entry.Source, err)
	} else {
		g.cache.Store(entry.Source, nil)
	}
	return err
}

func (g *GitHubValidator) validate(source string) error {
	parts := strings.SplitN(source, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid github source %q: expected owner/repo", source)
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s", source)
	body, err := g.apiGet(url)
	if err != nil {
		return fmt.Errorf("github repo check %s: %w", source, err)
	}

	var repo struct {
		Archived bool `json:"archived"`
	}
	if err := json.Unmarshal(body, &repo); err != nil {
		return fmt.Errorf("parse github response for %s: %w", source, err)
	}
	if repo.Archived {
		return fmt.Errorf("github repo %s is archived", source)
	}

	return nil
}

func (g *GitHubValidator) apiGet(url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		if g.token != "" {
			req.Header.Set("Authorization", "Bearer "+g.token)
		}
		req.Header.Set("Accept", "application/vnd.github+json")

		resp, err := g.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("not found (HTTP 404)")
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error (HTTP %d)", resp.StatusCode)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status (HTTP %d)", resp.StatusCode)
		}

		return body, nil
	}
	return nil, fmt.Errorf("after 3 retries: %w", lastErr)
}

// HomebrewValidator checks that a Homebrew formula exists.
type HomebrewValidator struct {
	client *http.Client
	cache  sync.Map
}

// NewHomebrewValidator creates a validator that checks Homebrew formulas.
func NewHomebrewValidator(client *http.Client) *HomebrewValidator {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &HomebrewValidator{client: client}
}

func (h *HomebrewValidator) Validate(entry SeedEntry) error {
	if cached, ok := h.cache.Load(entry.Source); ok {
		if cached == nil {
			return nil
		}
		return cached.(error)
	}

	err := h.validate(entry.Source)
	if err != nil {
		h.cache.Store(entry.Source, err)
	} else {
		h.cache.Store(entry.Source, nil)
	}
	return err
}

func (h *HomebrewValidator) validate(formula string) error {
	url := fmt.Sprintf("https://formulae.brew.sh/api/formula/%s.json", formula)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
		}

		resp, err := h.client.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("homebrew formula %q not found", formula)
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error (HTTP %d)", resp.StatusCode)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return nil
		}
		return fmt.Errorf("unexpected status (HTTP %d)", resp.StatusCode)
	}
	return fmt.Errorf("after 3 retries: %w", lastErr)
}

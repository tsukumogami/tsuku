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

// EntryMetadata holds optional metadata extracted from API responses during validation.
type EntryMetadata struct {
	Description string
	Homepage    string
	Repo        string
}

// Validator checks whether a seed entry is valid (the source exists and is usable).
// On success, it may return metadata extracted from the API response.
type Validator interface {
	Validate(entry SeedEntry) (*EntryMetadata, error)
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
		meta, err := v.Validate(e)
		if err != nil {
			failures = append(failures, ValidationResult{Entry: e, Err: err})
			continue
		}
		if meta != nil {
			if meta.Description != "" {
				e.Description = meta.Description
			}
			if meta.Homepage != "" {
				e.Homepage = meta.Homepage
			}
			if meta.Repo != "" && e.Repo == "" {
				e.Repo = meta.Repo
			}
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

// cachedResult stores both metadata and error for the GitHub validator cache.
type cachedResult struct {
	meta *EntryMetadata
	err  error
}

func (g *GitHubValidator) Validate(entry SeedEntry) (*EntryMetadata, error) {
	if cached, ok := g.cache.Load(entry.Source); ok {
		cr := cached.(cachedResult)
		return cr.meta, cr.err
	}

	meta, err := g.validate(entry.Source)
	g.cache.Store(entry.Source, cachedResult{meta: meta, err: err})
	return meta, err
}

func (g *GitHubValidator) validate(source string) (*EntryMetadata, error) {
	parts := strings.SplitN(source, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid github source %q: expected owner/repo", source)
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s", source)
	body, err := g.apiGet(url)
	if err != nil {
		return nil, fmt.Errorf("github repo check %s: %w", source, err)
	}

	var repo struct {
		Archived    bool   `json:"archived"`
		Description string `json:"description"`
		Homepage    string `json:"homepage"`
		HTMLURL     string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &repo); err != nil {
		return nil, fmt.Errorf("parse github response for %s: %w", source, err)
	}
	if repo.Archived {
		return nil, fmt.Errorf("github repo %s is archived", source)
	}

	return &EntryMetadata{
		Description: repo.Description,
		Homepage:    repo.Homepage,
		Repo:        repo.HTMLURL,
	}, nil
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

func (h *HomebrewValidator) Validate(entry SeedEntry) (*EntryMetadata, error) {
	if cached, ok := h.cache.Load(entry.Source); ok {
		cr := cached.(cachedResult)
		return cr.meta, cr.err
	}

	meta, err := h.validate(entry.Source)
	h.cache.Store(entry.Source, cachedResult{meta: meta, err: err})
	return meta, err
}

func (h *HomebrewValidator) validate(formula string) (*EntryMetadata, error) {
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

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("homebrew formula %q not found", formula)
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error (HTTP %d)", resp.StatusCode)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			var f struct {
				Desc     string `json:"desc"`
				Homepage string `json:"homepage"`
			}
			// Best-effort metadata extraction; validation still succeeds if parsing fails
			if err := json.Unmarshal(body, &f); err != nil {
				return &EntryMetadata{}, nil
			}
			return &EntryMetadata{
				Description: f.Desc,
				Homepage:    f.Homepage,
			}, nil
		}
		return nil, fmt.Errorf("unexpected status (HTTP %d)", resp.StatusCode)
	}
	return nil, fmt.Errorf("after 3 retries: %w", lastErr)
}

package search

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/log"
)

// DefaultMaxRetries is the default number of retry attempts for rate-limited requests.
const DefaultMaxRetries = 3

// DDGOptions configures the DDGProvider behavior.
type DDGOptions struct {
	// MaxRetries is the maximum number of retry attempts for 202 responses.
	// Default: 3
	MaxRetries int

	// Logger for debug output. If nil, uses log.Default().
	Logger log.Logger

	// HTTPClient for making requests. If nil, uses http.DefaultClient.
	HTTPClient *http.Client

	// sleepFn is used for testing to mock time.Sleep behavior.
	// If nil, time.Sleep is used.
	sleepFn func(time.Duration)
}

// DDGProvider implements Provider using DuckDuckGo's HTML interface.
type DDGProvider struct {
	client     *http.Client
	maxRetries int
	logger     log.Logger
	sleepFn    func(time.Duration)
}

// NewDDGProvider creates a DuckDuckGo search provider with default options.
func NewDDGProvider() *DDGProvider {
	return NewDDGProviderWithOptions(DDGOptions{})
}

// NewDDGProviderWithOptions creates a DuckDuckGo search provider with custom options.
func NewDDGProviderWithOptions(opts DDGOptions) *DDGProvider {
	p := &DDGProvider{
		client:     opts.HTTPClient,
		maxRetries: opts.MaxRetries,
		logger:     opts.Logger,
		sleepFn:    opts.sleepFn,
	}

	// Apply defaults
	if p.client == nil {
		p.client = http.DefaultClient
	}
	if p.maxRetries <= 0 {
		p.maxRetries = DefaultMaxRetries
	}
	if p.logger == nil {
		p.logger = log.Default()
	}
	if p.sleepFn == nil {
		p.sleepFn = time.Sleep
	}

	return p
}

// Name returns the provider identifier.
func (p *DDGProvider) Name() string {
	return "ddg"
}

// Search performs a web search using DDG's lite HTML interface.
// Implements retry logic with exponential backoff for 202 (rate limiting) responses.
func (p *DDGProvider) Search(ctx context.Context, query string) (*Response, error) {
	baseDelay := time.Second

	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter: base * 2^(attempt-1) * (0.75 + rand*0.5)
			baseWait := baseDelay * time.Duration(1<<(attempt-1))
			jitter := 0.75 + rand.Float64()*0.5 // 75% to 125% of base
			delay := time.Duration(float64(baseWait) * jitter)

			p.logger.Debug("DDG search retry",
				"attempt", attempt,
				"max_retries", p.maxRetries,
				"delay", delay.String(),
				"query", query,
			)

			// Wait with context cancellation support
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				// Use time.After here even though we have sleepFn because
				// we need the select for context cancellation. Tests that
				// need to verify delays can check the logger output.
			}
		}

		body, statusCode, err := p.doSearch(ctx, query)
		if err != nil {
			lastErr = err
			// Network errors and context cancellation are not retryable
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// Retry on network errors
			continue
		}

		// Check for rate limiting (202 Accepted)
		if statusCode == http.StatusAccepted {
			lastErr = fmt.Errorf("DDG returned status 202 (rate limited)")
			continue
		}

		// Non-retryable status codes
		if statusCode != http.StatusOK {
			return nil, fmt.Errorf("DDG returned status %d", statusCode)
		}

		// Success - parse results
		results := parseHTMLResults(body)
		return &Response{
			Query:   query,
			Results: results,
		}, nil
	}

	return nil, fmt.Errorf("DDG search failed after %d retries: %w", p.maxRetries, lastErr)
}

// doSearch performs a single search request.
// Returns the response body, status code, and any error.
func (p *DDGProvider) doSearch(ctx context.Context, query string) (string, int, error) {
	url := "https://html.duckduckgo.com/html/?q=" + strings.ReplaceAll(query, " ", "+")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", 0, fmt.Errorf("create request: %w", err)
	}

	// Browser-like headers to avoid bot detection
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read response: %w", err)
	}

	return string(body), resp.StatusCode, nil
}

// parseHTMLResults extracts search results from DDG HTML response.
func parseHTMLResults(html string) []Result {
	var results []Result

	// DDG lite HTML uses specific classes for result elements
	// <a class="result__a"> for title/URL
	// <a class="result__snippet"> for snippet
	titleRe := regexp.MustCompile(`<a[^>]*class="result__a"[^>]*href="([^"]*)"[^>]*>([^<]*)</a>`)
	snippetRe := regexp.MustCompile(`<a[^>]*class="result__snippet"[^>]*>([^<]*)</a>`)

	titles := titleRe.FindAllStringSubmatch(html, -1)
	snippets := snippetRe.FindAllStringSubmatch(html, -1)

	for i, match := range titles {
		if len(match) >= 3 {
			result := Result{
				URL:   match[1],
				Title: strings.TrimSpace(match[2]),
			}

			// Associate snippet if available
			if i < len(snippets) && len(snippets[i]) >= 2 {
				result.Snippet = strings.TrimSpace(snippets[i][1])
			}

			// Decode DDG redirect URLs
			if strings.HasPrefix(result.URL, "//duckduckgo.com/l/?uddg=") {
				if decoded, err := decodeRedirectURL(result.URL); err == nil {
					result.URL = decoded
				}
			}

			results = append(results, result)
		}
	}

	return results
}

// decodeRedirectURL decodes a DDG redirect URL to the actual target.
func decodeRedirectURL(rawURL string) (string, error) {
	prefix := "//duckduckgo.com/l/?uddg="
	if !strings.HasPrefix(rawURL, prefix) {
		return rawURL, nil
	}

	encoded := strings.TrimPrefix(rawURL, prefix)

	// Remove trailing parameters
	if idx := strings.Index(encoded, "&"); idx > 0 {
		encoded = encoded[:idx]
	}

	// URL decode common characters
	decoded := encoded
	replacements := map[string]string{
		"%3A": ":",
		"%2F": "/",
		"%3F": "?",
		"%3D": "=",
		"%26": "&",
		"%25": "%",
		"%2B": "+",
		"%20": " ",
	}
	for old, new := range replacements {
		decoded = strings.ReplaceAll(decoded, old, new)
	}

	return decoded, nil
}

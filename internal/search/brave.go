package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"github.com/tsukumogami/tsuku/internal/log"
)

// BraveOptions configures the BraveProvider behavior.
type BraveOptions struct {
	// APIKey is required for Brave Search API access.
	APIKey string

	// MaxRetries is the maximum number of retry attempts for failed requests.
	// Default: 3
	MaxRetries int

	// Logger for debug output. If nil, uses log.Default().
	Logger log.Logger

	// HTTPClient for making requests. If nil, uses http.DefaultClient.
	HTTPClient *http.Client
}

// BraveProvider implements Provider using the Brave Web Search API.
type BraveProvider struct {
	apiKey     string
	client     *http.Client
	maxRetries int
	logger     log.Logger
}

// braveResponse is the JSON response from Brave Search API.
type braveResponse struct {
	Web braveWebResults `json:"web"`
}

// braveWebResults contains the web search results.
type braveWebResults struct {
	Results []braveResult `json:"results"`
}

// braveResult is a single result from Brave Search API.
type braveResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// NewBraveProvider creates a Brave search provider with default options.
// The APIKey must be provided.
func NewBraveProvider(apiKey string) *BraveProvider {
	return NewBraveProviderWithOptions(BraveOptions{APIKey: apiKey})
}

// NewBraveProviderWithOptions creates a Brave search provider with custom options.
func NewBraveProviderWithOptions(opts BraveOptions) *BraveProvider {
	p := &BraveProvider{
		apiKey:     opts.APIKey,
		client:     opts.HTTPClient,
		maxRetries: opts.MaxRetries,
		logger:     opts.Logger,
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

	return p
}

// Name returns the provider identifier.
func (p *BraveProvider) Name() string {
	return "brave"
}

// Search performs a web search using the Brave Search API.
// Implements retry logic with exponential backoff for transient failures.
func (p *BraveProvider) Search(ctx context.Context, query string) (*Response, error) {
	baseDelay := time.Second

	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter: base * 2^(attempt-1) * (0.75 + rand*0.5)
			baseWait := baseDelay * time.Duration(1<<(attempt-1))
			jitter := 0.75 + rand.Float64()*0.5
			delay := time.Duration(float64(baseWait) * jitter)

			p.logger.Debug("Brave search retry",
				"attempt", attempt,
				"max_retries", p.maxRetries,
				"delay", delay.String(),
				"query", query,
			)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err := p.doSearch(ctx, query)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// Retry on network errors
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("Brave search failed after %d retries: %w", p.maxRetries, lastErr)
}

// doSearch performs a single search request to the Brave Search API.
func (p *BraveProvider) doSearch(ctx context.Context, query string) (*Response, error) {
	reqURL := "https://api.search.brave.com/res/v1/web/search?q=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Brave returned status %d: %s", resp.StatusCode, string(body))
	}

	var braveResp braveResponse
	if err := json.Unmarshal(body, &braveResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// Convert to standard Response format
	results := make([]Result, len(braveResp.Web.Results))
	for i, r := range braveResp.Web.Results {
		results[i] = Result{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Description,
		}
	}

	return &Response{
		Query:   query,
		Results: results,
	}, nil
}

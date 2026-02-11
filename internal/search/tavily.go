package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"github.com/tsukumogami/tsuku/internal/log"
)

// TavilyOptions configures the TavilyProvider behavior.
type TavilyOptions struct {
	// APIKey is required for Tavily API access.
	APIKey string

	// MaxRetries is the maximum number of retry attempts for failed requests.
	// Default: 3
	MaxRetries int

	// Logger for debug output. If nil, uses log.Default().
	Logger log.Logger

	// HTTPClient for making requests. If nil, uses http.DefaultClient.
	HTTPClient *http.Client
}

// TavilyProvider implements Provider using the Tavily Search API.
type TavilyProvider struct {
	apiKey     string
	client     *http.Client
	maxRetries int
	logger     log.Logger
}

// tavilyRequest is the JSON request body for Tavily API.
type tavilyRequest struct {
	APIKey     string `json:"api_key"`
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

// tavilyResponse is the JSON response from Tavily API.
type tavilyResponse struct {
	Query   string         `json:"query"`
	Results []tavilyResult `json:"results"`
}

// tavilyResult is a single result from Tavily API.
type tavilyResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// NewTavilyProvider creates a Tavily search provider with default options.
// The APIKey must be provided.
func NewTavilyProvider(apiKey string) *TavilyProvider {
	return NewTavilyProviderWithOptions(TavilyOptions{APIKey: apiKey})
}

// NewTavilyProviderWithOptions creates a Tavily search provider with custom options.
func NewTavilyProviderWithOptions(opts TavilyOptions) *TavilyProvider {
	p := &TavilyProvider{
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
func (p *TavilyProvider) Name() string {
	return "tavily"
}

// Search performs a web search using the Tavily API.
// Implements retry logic with exponential backoff for transient failures.
func (p *TavilyProvider) Search(ctx context.Context, query string) (*Response, error) {
	baseDelay := time.Second

	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter: base * 2^(attempt-1) * (0.75 + rand*0.5)
			baseWait := baseDelay * time.Duration(1<<(attempt-1))
			jitter := 0.75 + rand.Float64()*0.5
			delay := time.Duration(float64(baseWait) * jitter)

			p.logger.Debug("Tavily search retry",
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

	return nil, fmt.Errorf("Tavily search failed after %d retries: %w", p.maxRetries, lastErr)
}

// doSearch performs a single search request to the Tavily API.
func (p *TavilyProvider) doSearch(ctx context.Context, query string) (*Response, error) {
	reqBody := tavilyRequest{
		APIKey:     p.apiKey,
		Query:      query,
		MaxResults: 10,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("Tavily returned status %d: %s", resp.StatusCode, string(body))
	}

	var tavilyResp tavilyResponse
	if err := json.Unmarshal(body, &tavilyResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// Convert to standard Response format
	results := make([]Result, len(tavilyResp.Results))
	for i, r := range tavilyResp.Results {
		results[i] = Result{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
		}
	}

	return &Response{
		Query:   query,
		Results: results,
	}, nil
}

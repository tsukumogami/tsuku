package search

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// DDGProvider implements Provider using DuckDuckGo's HTML interface.
type DDGProvider struct {
	client *http.Client
}

// NewDDGProvider creates a DuckDuckGo search provider.
func NewDDGProvider() *DDGProvider {
	return &DDGProvider{
		client: http.DefaultClient,
	}
}

// Name returns the provider identifier.
func (p *DDGProvider) Name() string {
	return "ddg"
}

// Search performs a web search using DDG's lite HTML interface.
func (p *DDGProvider) Search(ctx context.Context, query string) (*Response, error) {
	// Use the lite HTML interface which is more accessible
	url := "https://html.duckduckgo.com/html/?q=" + strings.ReplaceAll(query, " ", "+")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Browser-like headers to avoid bot detection
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DDG returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	results := parseHTMLResults(string(body))

	return &Response{
		Query:   query,
		Results: results,
	}, nil
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

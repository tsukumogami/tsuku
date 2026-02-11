// Package search provides web search capabilities for LLM tool use.
//
// This package defines a SearchProvider interface that can be implemented
// by different search backends (DDG, Tavily, Brave, etc.). The providers
// are designed to be used as tool handlers in LLM conversations.
package search

import (
	"context"
)

// Result represents a single search result.
type Result struct {
	Title   string // Page title
	URL     string // Page URL
	Snippet string // Text snippet/description
}

// Response contains search results and metadata.
type Response struct {
	Query   string   // The search query
	Results []Result // Search results
}

// Provider defines the interface for web search backends.
type Provider interface {
	// Name returns the provider identifier (e.g., "ddg", "tavily", "brave").
	Name() string

	// Search performs a web search and returns results.
	// The context can be used for cancellation and timeouts.
	Search(ctx context.Context, query string) (*Response, error)
}

// FormatForLLM formats search results for LLM consumption.
// Returns a human-readable string suitable for tool response.
func (r *Response) FormatForLLM(maxResults int) string {
	if len(r.Results) == 0 {
		return "No results found for: " + r.Query
	}

	var sb stringBuilder
	sb.WriteString("Search results for: ")
	sb.WriteString(r.Query)
	sb.WriteString("\n\n")

	count := len(r.Results)
	if maxResults > 0 && count > maxResults {
		count = maxResults
	}

	for i := 0; i < count; i++ {
		result := r.Results[i]
		sb.WriteString(itoa(i + 1))
		sb.WriteString(". ")
		sb.WriteString(result.Title)
		sb.WriteString("\n   ")
		sb.WriteString(result.URL)
		sb.WriteString("\n   ")
		sb.WriteString(result.Snippet)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// stringBuilder is a simple string builder to avoid fmt dependency.
type stringBuilder struct {
	buf []byte
}

func (b *stringBuilder) WriteString(s string) {
	b.buf = append(b.buf, s...)
}

func (b *stringBuilder) String() string {
	return string(b.buf)
}

func itoa(i int) string {
	if i < 10 {
		return string('0' + byte(i))
	}
	return itoa(i/10) + string('0'+byte(i%10))
}

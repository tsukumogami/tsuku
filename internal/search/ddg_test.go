package search

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestDDGProvider_Name(t *testing.T) {
	p := NewDDGProvider()
	if p.Name() != "ddg" {
		t.Errorf("expected name 'ddg', got %q", p.Name())
	}
}

func TestParseHTMLResults(t *testing.T) {
	html := `
<div class="result">
  <a class="result__a" href="https://github.com/stripe/stripe-cli">Stripe CLI</a>
  <a class="result__snippet">A command line tool for Stripe</a>
</div>
<div class="result">
  <a class="result__a" href="https://stripe.com/docs/cli">Stripe CLI Documentation</a>
  <a class="result__snippet">Official Stripe CLI documentation</a>
</div>
`
	results := parseHTMLResults(html)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	if len(results) > 0 {
		if results[0].Title != "Stripe CLI" {
			t.Errorf("expected title 'Stripe CLI', got %q", results[0].Title)
		}
		if results[0].URL != "https://github.com/stripe/stripe-cli" {
			t.Errorf("unexpected URL: %s", results[0].URL)
		}
	}
}

func TestDecodeRedirectURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "//duckduckgo.com/l/?uddg=https%3A%2F%2Fgithub.com%2Fstripe%2Fstripe-cli",
			expected: "https://github.com/stripe/stripe-cli",
		},
		{
			input:    "https://github.com/direct/link",
			expected: "https://github.com/direct/link",
		},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result, err := decodeRedirectURL(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tc.expected {
				t.Errorf("got %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestDDGProvider_Integration(t *testing.T) {
	if os.Getenv("INTEGRATION_TESTS") == "" {
		t.Skip("Set INTEGRATION_TESTS=1 to run integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	p := NewDDGProvider()
	resp, err := p.Search(ctx, "stripe cli github")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(resp.Results) == 0 {
		t.Error("expected results, got none")
	}

	t.Logf("Got %d results for 'stripe cli github'", len(resp.Results))
	for i, r := range resp.Results {
		if i >= 3 {
			break
		}
		t.Logf("  %d. %s - %s", i+1, r.Title, r.URL)
	}
}

func TestResponseFormatForLLM(t *testing.T) {
	resp := &Response{
		Query: "test query",
		Results: []Result{
			{Title: "Result 1", URL: "https://example.com/1", Snippet: "First result"},
			{Title: "Result 2", URL: "https://example.com/2", Snippet: "Second result"},
			{Title: "Result 3", URL: "https://example.com/3", Snippet: "Third result"},
		},
	}

	// Test with limit
	formatted := resp.FormatForLLM(2)
	if formatted == "" {
		t.Error("expected non-empty formatted output")
	}

	// Should contain first 2 results but not 3rd
	if !containsSubstr(formatted, "Result 1") || !containsSubstr(formatted, "Result 2") {
		t.Error("expected first two results in output")
	}

	// Test empty results
	emptyResp := &Response{Query: "empty query", Results: nil}
	emptyFormatted := emptyResp.FormatForLLM(10)
	if !containsSubstr(emptyFormatted, "No results found") {
		t.Errorf("expected 'No results found' message, got: %s", emptyFormatted)
	}
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

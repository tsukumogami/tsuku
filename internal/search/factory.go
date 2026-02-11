package search

import (
	"fmt"
	"os"
)

// NewSearchProvider creates a search provider based on configuration.
// If explicit is non-empty, it specifies the provider to use (ddg, tavily, brave).
// Otherwise, auto-selection checks environment variables in priority order:
//   - TAVILY_API_KEY -> Tavily
//   - BRAVE_API_KEY -> Brave
//   - Otherwise -> DDG (no key required)
func NewSearchProvider(explicit string) (Provider, error) {
	switch explicit {
	case "tavily":
		key := os.Getenv("TAVILY_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("--search-provider=tavily requires TAVILY_API_KEY environment variable")
		}
		return NewTavilyProvider(key), nil

	case "brave":
		key := os.Getenv("BRAVE_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("--search-provider=brave requires BRAVE_API_KEY environment variable")
		}
		return NewBraveProvider(key), nil

	case "ddg":
		return NewDDGProvider(), nil

	case "":
		// Auto-selection based on available API keys
		if key := os.Getenv("TAVILY_API_KEY"); key != "" {
			return NewTavilyProvider(key), nil
		}
		if key := os.Getenv("BRAVE_API_KEY"); key != "" {
			return NewBraveProvider(key), nil
		}
		return NewDDGProvider(), nil

	default:
		return nil, fmt.Errorf("invalid search provider %q: must be ddg, tavily, or brave", explicit)
	}
}

// ValidProviders returns the list of valid provider names.
func ValidProviders() []string {
	return []string{"ddg", "tavily", "brave"}
}

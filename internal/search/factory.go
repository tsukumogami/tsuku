package search

import (
	"fmt"

	"github.com/tsukumogami/tsuku/internal/secrets"
)

// NewSearchProvider creates a search provider based on configuration.
// If explicit is non-empty, it specifies the provider to use (ddg, tavily, brave).
// Otherwise, auto-selection checks for configured API keys in priority order:
//   - tavily_api_key -> Tavily
//   - brave_api_key -> Brave
//   - Otherwise -> DDG (no key required)
func NewSearchProvider(explicit string) (Provider, error) {
	switch explicit {
	case "tavily":
		key, err := secrets.Get("tavily_api_key")
		if err != nil {
			return nil, fmt.Errorf("--search-provider=tavily requires tavily_api_key: set TAVILY_API_KEY environment variable or add tavily_api_key to [secrets] in $TSUKU_HOME/config.toml")
		}
		return NewTavilyProvider(key), nil

	case "brave":
		key, err := secrets.Get("brave_api_key")
		if err != nil {
			return nil, fmt.Errorf("--search-provider=brave requires brave_api_key: set BRAVE_API_KEY environment variable or add brave_api_key to [secrets] in $TSUKU_HOME/config.toml")
		}
		return NewBraveProvider(key), nil

	case "ddg":
		return NewDDGProvider(), nil

	case "":
		// Auto-selection based on available API keys
		if key, err := secrets.Get("tavily_api_key"); err == nil {
			return NewTavilyProvider(key), nil
		}
		if key, err := secrets.Get("brave_api_key"); err == nil {
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

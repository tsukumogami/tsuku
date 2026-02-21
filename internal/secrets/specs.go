package secrets

// KeySpec defines how to resolve a specific secret.
type KeySpec struct {
	// EnvVars lists environment variables to check, in priority order.
	EnvVars []string

	// Desc is a human-readable description for error messages and CLI display.
	Desc string
}

// knownKeys maps secret names to their resolution specs.
// Adding a new secret is one entry here.
var knownKeys = map[string]KeySpec{
	"anthropic_api_key": {
		EnvVars: []string{"ANTHROPIC_API_KEY"},
		Desc:    "Anthropic API key for Claude",
	},
	"google_api_key": {
		EnvVars: []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"},
		Desc:    "Google API key for Gemini",
	},
	"github_token": {
		EnvVars: []string{"GITHUB_TOKEN"},
		Desc:    "GitHub personal access token",
	},
	"tavily_api_key": {
		EnvVars: []string{"TAVILY_API_KEY"},
		Desc:    "Tavily search API key",
	},
	"brave_api_key": {
		EnvVars: []string{"BRAVE_API_KEY"},
		Desc:    "Brave search API key",
	},
}

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Model is the Claude model used for recipe generation.
// Hardcoded for Slice 1 as per design doc.
const Model = "claude-sonnet-4-5-20250929"

// MaxTurns is the maximum number of conversation turns to prevent infinite loops.
const MaxTurns = 5

// Client wraps the Anthropic API for recipe generation.
type Client struct {
	anthropic  anthropic.Client
	model      anthropic.Model
	httpClient *http.Client // For executing fetch_file and inspect_archive tools
}

// NewClient creates a new LLM client using ANTHROPIC_API_KEY from environment.
// Returns an error if the API key is not set.
func NewClient() (*Client, error) {
	return NewClientWithHTTPClient(nil)
}

// NewClientWithHTTPClient creates a new LLM client with a custom HTTP client
// for tool execution (fetch_file, inspect_archive).
// If httpClient is nil, a default client with timeouts will be created.
func NewClientWithHTTPClient(httpClient *http.Client) (*Client, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable not set")
	}

	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 60 * time.Second,
		}
	}

	return &Client{
		anthropic:  anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:      anthropic.Model(Model),
		httpClient: httpClient,
	}, nil
}

// Release represents a GitHub release with its assets.
type Release struct {
	Tag    string   `json:"tag"`
	Assets []string `json:"assets"`
}

// GenerateRequest contains the input for recipe generation.
type GenerateRequest struct {
	Repo        string    `json:"repo"`
	Releases    []Release `json:"releases"`
	Description string    `json:"description"`
	README      string    `json:"readme"`
}

// AssetPattern contains the discovered pattern for matching release assets to platforms.
type AssetPattern struct {
	Mappings       []PlatformMapping `json:"mappings"`
	Executable     string            `json:"executable"`
	VerifyCommand  string            `json:"verify_command"`
	StripPrefix    string            `json:"strip_prefix,omitempty"`
	InstallSubpath string            `json:"install_subpath,omitempty"`
}

// GenerateRecipe runs a multi-turn conversation until extract_pattern is called.
// Returns the discovered asset pattern, token usage, and any error.
func (c *Client) GenerateRecipe(ctx context.Context, req *GenerateRequest) (*AssetPattern, *Usage, error) {
	systemPrompt := buildSystemPrompt()
	userMessage := buildUserMessage(req)

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(userMessage)),
	}

	tools := toolSchemas()
	var totalUsage Usage

	for turn := 0; turn < MaxTurns; turn++ {
		resp, err := c.anthropic.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     c.model,
			MaxTokens: 4096,
			System: []anthropic.TextBlockParam{
				{Text: systemPrompt},
			},
			Messages: messages,
			Tools:    tools,
		})
		if err != nil {
			return nil, &totalUsage, fmt.Errorf("anthropic API call failed: %w", err)
		}

		// Accumulate usage
		totalUsage.Add(Usage{
			InputTokens:  int(resp.Usage.InputTokens),
			OutputTokens: int(resp.Usage.OutputTokens),
		})

		// Add assistant response to conversation
		messages = append(messages, resp.ToParam())

		// Process content blocks looking for tool use
		var toolResults []anthropic.ContentBlockParamUnion
		var pattern *AssetPattern

		for _, block := range resp.Content {
			switch variant := block.AsAny().(type) {
			case anthropic.ToolUseBlock:
				result, extractedPattern, err := c.executeToolUse(ctx, variant)
				if err != nil {
					// Return error as tool result so Claude can try again
					toolResults = append(toolResults,
						anthropic.NewToolResultBlock(variant.ID, fmt.Sprintf("Error: %v", err), true))
					continue
				}

				if extractedPattern != nil {
					// extract_pattern was called - we're done
					pattern = extractedPattern
				} else {
					toolResults = append(toolResults,
						anthropic.NewToolResultBlock(variant.ID, result, false))
				}
			}
		}

		// If extract_pattern was called, return the pattern
		if pattern != nil {
			return pattern, &totalUsage, nil
		}

		// If there were tool calls, add results and continue
		if len(toolResults) > 0 {
			messages = append(messages, anthropic.NewUserMessage(toolResults...))
			continue
		}

		// No tool calls and no extract_pattern - Claude is done but didn't call the tool
		if resp.StopReason == "end_turn" {
			return nil, &totalUsage, fmt.Errorf("conversation ended without extract_pattern being called")
		}
	}

	return nil, &totalUsage, fmt.Errorf("max turns (%d) exceeded without completing recipe generation", MaxTurns)
}

// executeToolUse executes a tool call and returns the result.
// For extract_pattern, it returns the parsed pattern instead of a string result.
func (c *Client) executeToolUse(ctx context.Context, toolUse anthropic.ToolUseBlock) (string, *AssetPattern, error) {
	switch toolUse.Name {
	case ToolFetchFile:
		var input FetchFileInput
		if err := json.Unmarshal(toolUse.Input, &input); err != nil {
			return "", nil, fmt.Errorf("invalid fetch_file input: %w", err)
		}
		content, err := c.fetchFile(ctx, input.URL)
		if err != nil {
			return "", nil, err
		}
		return content, nil, nil

	case ToolInspectArchive:
		var input InspectArchiveInput
		if err := json.Unmarshal(toolUse.Input, &input); err != nil {
			return "", nil, fmt.Errorf("invalid inspect_archive input: %w", err)
		}
		listing, err := c.inspectArchive(ctx, input.URL)
		if err != nil {
			return "", nil, err
		}
		return listing, nil, nil

	case ToolExtractPattern:
		var input ExtractPatternInput
		if err := json.Unmarshal(toolUse.Input, &input); err != nil {
			return "", nil, fmt.Errorf("invalid extract_pattern input: %w", err)
		}
		pattern := &AssetPattern{
			Mappings:       input.Mappings,
			Executable:     input.Executable,
			VerifyCommand:  input.VerifyCommand,
			StripPrefix:    input.StripPrefix,
			InstallSubpath: input.InstallSubpath,
		}
		return "", pattern, nil

	default:
		return "", nil, fmt.Errorf("unknown tool: %s", toolUse.Name)
	}
}

// fetchFile fetches a file from a URL and returns its contents.
func (c *Client) fetchFile(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Limit response size to 1MB to prevent memory issues
	const maxSize = 1 * 1024 * 1024
	content := make([]byte, maxSize)
	n, err := resp.Body.Read(content)
	if err != nil && err.Error() != "EOF" {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return string(content[:n]), nil
}

// buildSystemPrompt creates the system prompt for recipe generation.
func buildSystemPrompt() string {
	return `You are an expert at analyzing GitHub releases to create installation recipes for tsuku, a package manager.

Your task is to analyze the provided release information and determine how to match release assets to different platforms (linux/darwin, amd64/arm64).

You have three tools available:
1. fetch_file: Fetch a file from a URL to examine its contents (useful for READMEs)
2. inspect_archive: Inspect the contents of an archive to find the executable
3. extract_pattern: Call this when you've determined the asset-to-platform mappings

Common patterns you should recognize:
- Rust-style targets: x86_64-unknown-linux-musl, aarch64-apple-darwin
- Go-style targets: linux_amd64, darwin_arm64
- Generic: linux-x64, macos-arm64, linux-amd64

When analyzing assets:
- Look for patterns in filenames that indicate OS and architecture
- Identify the archive format (tar.gz, zip, or bare binary)
- Determine the executable name inside the archive
- Consider common verification commands (tool --version, tool version)

Once you understand the pattern, call extract_pattern with the mappings.
Focus on linux (amd64, arm64) and darwin (amd64, arm64) platforms.`
}

// buildUserMessage creates the initial user message with release context.
func buildUserMessage(req *GenerateRequest) string {
	releasesJSON, _ := json.MarshalIndent(req.Releases, "", "  ")

	msg := fmt.Sprintf(`Please analyze this GitHub repository and its releases to create a recipe.

Repository: %s
Description: %s

Recent releases:
%s

`, req.Repo, req.Description, string(releasesJSON))

	if req.README != "" {
		// Truncate README if too long
		readme := req.README
		if len(readme) > 10000 {
			readme = readme[:10000] + "\n...(truncated)"
		}
		msg += fmt.Sprintf("README.md:\n%s\n", readme)
	}

	msg += "\nAnalyze the release assets and call extract_pattern with the platform mappings."

	return msg
}

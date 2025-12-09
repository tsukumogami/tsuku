package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Model is the Claude model used for recipe generation.
// Hardcoded for Slice 1 as per design doc.
const Model = "claude-sonnet-4-5-20250929"

// MaxTurns is the maximum number of conversation turns to prevent infinite loops.
const MaxTurns = 5

// Client wraps a Provider for recipe generation with tool execution.
// It manages multi-turn conversations and executes tools on behalf of the LLM.
type Client struct {
	provider   *ClaudeProvider
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
	provider, err := NewClaudeProvider()
	if err != nil {
		return nil, err
	}

	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 60 * time.Second,
		}
	}

	return &Client{
		provider:   provider,
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

// generationContext holds context needed during tool execution.
type generationContext struct {
	repo string // GitHub repository (owner/repo)
	tag  string // Release tag to use for file fetching
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

	// Build common message types for the conversation
	messages := []Message{
		{Role: RoleUser, Content: userMessage},
	}

	// Convert tool schemas to common ToolDef format
	tools := buildToolDefs()
	var totalUsage Usage

	// Create generation context for tool execution
	genCtx := &generationContext{
		repo: req.Repo,
	}
	// Use the first release tag for file fetching
	if len(req.Releases) > 0 {
		genCtx.tag = req.Releases[0].Tag
	}

	for turn := 0; turn < MaxTurns; turn++ {
		resp, err := c.provider.Complete(ctx, &CompletionRequest{
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Tools:        tools,
			MaxTokens:    4096,
		})
		if err != nil {
			return nil, &totalUsage, err
		}

		// Accumulate usage
		totalUsage.Add(resp.Usage)

		// Add assistant response to conversation
		messages = append(messages, Message{
			Role:      RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Process tool calls
		var toolResults []Message
		var pattern *AssetPattern

		for _, tc := range resp.ToolCalls {
			result, extractedPattern, err := c.executeToolCall(ctx, genCtx, tc)
			if err != nil {
				// Return error as tool result so Claude can try again
				toolResults = append(toolResults, Message{
					Role: RoleUser,
					ToolResult: &ToolResult{
						CallID:  tc.ID,
						Content: fmt.Sprintf("Error: %v", err),
						IsError: true,
					},
				})
				continue
			}

			if extractedPattern != nil {
				// extract_pattern was called - we're done
				pattern = extractedPattern
			} else {
				toolResults = append(toolResults, Message{
					Role: RoleUser,
					ToolResult: &ToolResult{
						CallID:  tc.ID,
						Content: result,
						IsError: false,
					},
				})
			}
		}

		// If extract_pattern was called, return the pattern
		if pattern != nil {
			return pattern, &totalUsage, nil
		}

		// If there were tool calls, add results and continue
		if len(toolResults) > 0 {
			messages = append(messages, toolResults...)
			continue
		}

		// No tool calls and no extract_pattern - Claude is done but didn't call the tool
		if resp.StopReason == "end_turn" {
			return nil, &totalUsage, fmt.Errorf("conversation ended without extract_pattern being called")
		}
	}

	return nil, &totalUsage, fmt.Errorf("max turns (%d) exceeded without completing recipe generation", MaxTurns)
}

// executeToolCall executes a tool call using common types and returns the result.
// For extract_pattern, it returns the parsed pattern instead of a string result.
func (c *Client) executeToolCall(ctx context.Context, genCtx *generationContext, tc ToolCall) (string, *AssetPattern, error) {
	switch tc.Name {
	case ToolFetchFile:
		path, _ := tc.Arguments["path"].(string)
		if path == "" {
			return "", nil, fmt.Errorf("invalid fetch_file input: missing path")
		}
		content, err := c.fetchFile(ctx, genCtx.repo, genCtx.tag, path)
		if err != nil {
			return "", nil, err
		}
		return content, nil, nil

	case ToolInspectArchive:
		url, _ := tc.Arguments["url"].(string)
		if url == "" {
			return "", nil, fmt.Errorf("invalid inspect_archive input: missing url")
		}
		listing, err := c.inspectArchive(ctx, url)
		if err != nil {
			return "", nil, err
		}
		return listing, nil, nil

	case ToolExtractPattern:
		// Convert Arguments map to ExtractPatternInput
		argsJSON, err := json.Marshal(tc.Arguments)
		if err != nil {
			return "", nil, fmt.Errorf("invalid extract_pattern input: %w", err)
		}
		var input ExtractPatternInput
		if err := json.Unmarshal(argsJSON, &input); err != nil {
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
		return "", nil, fmt.Errorf("unknown tool: %s", tc.Name)
	}
}

// fetchFile fetches a file from a GitHub repository using raw.githubusercontent.com.
// It constructs the URL from the repo, tag, and path parameters.
func (c *Client) fetchFile(ctx context.Context, repo, tag, path string) (string, error) {
	// Construct raw.githubusercontent.com URL
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", repo, tag, path)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("file not found: %s (check if the file exists in the repository at tag %s)", path, tag)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Check content type - reject binary files
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !isTextContentType(contentType) {
		return "", fmt.Errorf("file appears to be binary (Content-Type: %s), only text files are supported", contentType)
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

// isTextContentType checks if the content type indicates a text file.
func isTextContentType(contentType string) bool {
	// Common text content types
	textPrefixes := []string{
		"text/",
		"application/json",
		"application/xml",
		"application/javascript",
		"application/x-yaml",
		"application/toml",
	}
	for _, prefix := range textPrefixes {
		if len(contentType) >= len(prefix) && contentType[:len(prefix)] == prefix {
			return true
		}
	}
	return false
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

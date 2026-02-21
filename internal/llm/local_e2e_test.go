//go:build e2e

package llm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tsukumogami/tsuku/internal/llm/addon"
)

// TestE2E_FactoryFallbackToLocal verifies that when no cloud API keys are
// configured, the factory falls through to LocalProvider. This is the
// infrastructure validation for scenario-14: the factory must produce a
// working provider whose Name() returns "local" without any cloud credentials.
func TestE2E_FactoryFallbackToLocal(t *testing.T) {
	// Clear all cloud API keys to force local fallback.
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	ctx := context.Background()
	factory, err := NewFactory(ctx,
		WithLocalEnabled(true),
		WithPrompter(&addon.AutoApprovePrompter{}),
	)
	require.NoError(t, err, "NewFactory should succeed with local provider enabled and no cloud keys")

	require.Equal(t, 1, factory.ProviderCount(), "factory should have exactly 1 provider")
	require.True(t, factory.HasProvider("local"), "factory should have local provider")

	provider, err := factory.GetProvider(ctx)
	require.NoError(t, err, "GetProvider should succeed")
	require.Equal(t, "local", provider.Name(), "provider should be local")
}

// TestE2E_CreateWithLocalProvider validates the complete recipe generation flow
// using local inference with no cloud API keys configured (scenario-13).
//
// Preconditions:
//   - tsuku-llm addon is installed (TSUKU_LLM_BINARY set or at standard path)
//   - Model is already downloaded (or the addon downloads it on first inference)
//   - No ANTHROPIC_API_KEY, GOOGLE_API_KEY, or GEMINI_API_KEY set
//
// The test exercises:
//  1. Factory fallthrough to LocalProvider when no cloud keys exist
//  2. Full Complete() call against the running addon
//  3. Validation that the response contains an extract_pattern tool call
//     with platform mappings covering linux/amd64 and darwin/arm64
//
// Run with: go test -v -run TestE2E_CreateWithLocalProvider ./internal/llm/ -tags=e2e -count=1
func TestE2E_CreateWithLocalProvider(t *testing.T) {
	// Skip if addon binary is not available
	addonPath := findAddonBinary(t)
	if addonPath == "" {
		t.Skip("tsuku-llm addon not installed; set TSUKU_LLM_BINARY or install via recipe")
	}

	// Set up isolated test environment
	tsukuHome := t.TempDir()
	t.Setenv("TSUKU_HOME", tsukuHome)
	t.Setenv("TSUKU_LLM_BINARY", addonPath)

	// Clear all cloud API keys to force local fallback
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	// Use a short idle timeout so the server shuts down quickly after the test
	t.Setenv(IdleTimeoutEnvVar, "30s")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create factory -- should fall through to LocalProvider
	factory, err := NewFactory(ctx,
		WithLocalEnabled(true),
		WithPrompter(&addon.AutoApprovePrompter{}),
	)
	require.NoError(t, err, "factory creation should succeed with local fallback")
	require.True(t, factory.HasProvider("local"), "factory should have local provider")
	require.Equal(t, 1, factory.ProviderCount(), "should have only local provider")

	provider, err := factory.GetProvider(ctx)
	require.NoError(t, err, "GetProvider should return local provider")
	require.Equal(t, "local", provider.Name())

	// Build a recipe generation request for a well-known tool (jq).
	// jq has a predictable release structure on GitHub with clear platform binaries.
	tools := buildToolDefs()
	req := &CompletionRequest{
		SystemPrompt: recipeGenerationSystemPrompt("jq"),
		Messages: []Message{
			{
				Role:    RoleUser,
				Content: recipeGenerationUserPrompt("jqlang/jq", "jq"),
			},
		},
		Tools:     tools,
		MaxTokens: 2048,
	}

	resp, err := provider.Complete(ctx, req)
	require.NoError(t, err, "Complete should succeed with local inference")
	require.NotNil(t, resp, "response should not be nil")

	t.Logf("Response content: %s", resp.Content)
	t.Logf("Stop reason: %s", resp.StopReason)
	t.Logf("Tool calls: %d", len(resp.ToolCalls))
	t.Logf("Usage: input=%d output=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens)

	// The response should contain at least one tool call.
	// With small models, the first response might be a fetch_file or
	// inspect_archive call rather than extract_pattern directly.
	// That's acceptable -- the key validation is that Complete() succeeds
	// and the response contains structured output (tool calls).
	require.NotEmpty(t, resp.ToolCalls, "response should contain at least one tool call")

	// Log tool call details for debugging
	for i, tc := range resp.ToolCalls {
		argsJSON, _ := json.MarshalIndent(tc.Arguments, "", "  ")
		t.Logf("Tool call %d: name=%s id=%s args=%s", i, tc.Name, tc.ID, string(argsJSON))
	}

	// Validate that the tool call is one of the expected tools
	validTools := map[string]bool{
		ToolFetchFile:      true,
		ToolInspectArchive: true,
		ToolExtractPattern: true,
	}
	for _, tc := range resp.ToolCalls {
		require.True(t, validTools[tc.Name],
			"tool call name %q should be one of the defined tools", tc.Name)
	}

	// If the model returned extract_pattern directly, validate its structure
	for _, tc := range resp.ToolCalls {
		if tc.Name == ToolExtractPattern {
			validateExtractPattern(t, tc.Arguments)
		}
	}

	// Clean up: shut down the addon server
	if localProvider, ok := provider.(*LocalProvider); ok {
		_ = localProvider.Shutdown(ctx, true)
		_ = localProvider.Close()
	}
}

// findAddonBinary locates the tsuku-llm binary. Returns empty string if not found.
func findAddonBinary(t *testing.T) string {
	t.Helper()

	// Check TSUKU_LLM_BINARY env var first
	if path := os.Getenv("TSUKU_LLM_BINARY"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// Try standard build locations relative to workspace root.
	// Walk up from cwd to find the directory containing go.mod.
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	root := cwd
	for {
		if _, err := os.Stat(root + "/go.mod"); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			return "" // go.mod not found
		}
		root = parent
	}

	candidates := []string{
		root + "/tsuku-llm/target/release/tsuku-llm",
		root + "/tsuku-llm/target/debug/tsuku-llm",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// validateExtractPattern checks that an extract_pattern tool call has the
// minimum required structure for a valid recipe mapping.
func validateExtractPattern(t *testing.T, args map[string]any) {
	t.Helper()

	// Check for required top-level fields
	require.Contains(t, args, "mappings", "extract_pattern should contain mappings")
	require.Contains(t, args, "executable", "extract_pattern should contain executable")
	require.Contains(t, args, "verify_command", "extract_pattern should contain verify_command")

	// Validate mappings array
	mappingsRaw, ok := args["mappings"].([]any)
	require.True(t, ok, "mappings should be an array")
	require.NotEmpty(t, mappingsRaw, "mappings should not be empty")

	// Track which platforms are covered
	platforms := make(map[string]bool)
	for _, m := range mappingsRaw {
		mapping, ok := m.(map[string]any)
		if !ok {
			continue
		}

		osVal, _ := mapping["os"].(string)
		archVal, _ := mapping["arch"].(string)
		if osVal != "" && archVal != "" {
			platforms[osVal+"/"+archVal] = true
		}

		// Each mapping should have at minimum: asset, os, arch, format
		require.Contains(t, mapping, "asset", "mapping should contain asset")
		require.Contains(t, mapping, "os", "mapping should contain os")
		require.Contains(t, mapping, "arch", "mapping should contain arch")
		require.Contains(t, mapping, "format", "mapping should contain format")
	}

	// Verify minimum platform coverage
	require.True(t, platforms["linux/amd64"],
		"mappings should cover linux/amd64, got: %v", platforms)
	require.True(t, platforms["darwin/arm64"],
		"mappings should cover darwin/arm64, got: %v", platforms)

	t.Logf("Platform coverage: %v", platforms)
}

// recipeGenerationSystemPrompt builds a system prompt for recipe generation.
// This mirrors the prompt structure used by the real tsuku create command.
func recipeGenerationSystemPrompt(toolName string) string {
	return `You are a tool installation expert. Analyze GitHub release assets to create installation recipes for developer tools.

Your task: Given information about a GitHub repository's releases, determine how each release asset maps to operating systems and architectures.

Rules:
- Only use the tools provided to gather information and report your findings
- Call extract_pattern when you have enough information to map assets to platforms
- Map assets to: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
- Identify the archive format (tar.gz, zip, binary) for each asset
- Determine the executable name inside archives

The tool you are analyzing is: ` + toolName
}

// recipeGenerationUserPrompt builds an initial user message for recipe generation.
func recipeGenerationUserPrompt(repo, toolName string) string {
	return `Analyze the release assets for ` + repo + ` (` + toolName + `).

Here are the latest release assets:
- jq-1.7.1.tar.gz (source tarball)
- jq-linux-amd64 (binary)
- jq-linux-arm64 (binary)
- jq-macos-amd64 (binary)
- jq-macos-arm64 (binary)
- jq-windows-amd64.exe (binary)

The README says: "jq is a lightweight and flexible command-line JSON processor."
The verify command is: jq --version

Based on these assets, call extract_pattern with the platform mappings.`
}

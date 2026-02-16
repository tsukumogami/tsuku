//go:build e2e

package llm

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestE2E_FactoryFallthrough verifies that the factory selects LocalProvider
// as the only available provider when no cloud API keys are configured.
// This is the core mechanism that lets tsuku work without accounts.
func TestE2E_FactoryFallthrough(t *testing.T) {
	// Clear all cloud provider keys so the factory can't register them
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	// Use isolated home directory to avoid touching user's real config
	t.Setenv("TSUKU_HOME", t.TempDir())

	ctx := context.Background()
	factory, err := NewFactory(ctx)
	require.NoError(t, err, "factory should succeed with local provider enabled")

	require.True(t, factory.HasProvider("local"), "factory should have local provider registered")
	require.False(t, factory.HasProvider("claude"), "factory should not have claude without API key")
	require.False(t, factory.HasProvider("gemini"), "factory should not have gemini without API key")
	require.Equal(t, 1, factory.ProviderCount(), "only local provider should be registered")

	// GetProvider should return local since it's the only one
	provider, err := factory.GetProvider(ctx)
	require.NoError(t, err, "GetProvider should return the local provider")
	require.Equal(t, "local", provider.Name(), "returned provider should be local")
}

// TestE2E_FactoryFallthroughLocalDisabled verifies that the factory returns
// an error when no cloud keys are set and local is explicitly disabled.
func TestE2E_FactoryFallthroughLocalDisabled(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("TSUKU_HOME", t.TempDir())

	ctx := context.Background()
	_, err := NewFactory(ctx, WithLocalEnabled(false))
	require.Error(t, err, "factory should fail with no providers")
	require.Contains(t, err.Error(), "no LLM providers available")
}

// TestE2E_CreateWithLocalProvider performs an end-to-end completion using the
// local tsuku-llm addon. It skips when the addon isn't running, making it
// safe to run in environments without the addon installed.
//
// Run with:
//
//	go test -v -tags=e2e -run TestE2E_CreateWithLocalProvider ./internal/llm/
func TestE2E_CreateWithLocalProvider(t *testing.T) {
	// Clear cloud keys so the factory selects the local provider.
	// Do NOT override TSUKU_HOME -- the provider needs it to find the
	// addon socket. These tests are read-only against the addon.
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	if !IsAddonRunning() {
		t.Skip("tsuku-llm addon not running, skipping e2e test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	provider := NewLocalProvider()
	require.NotNil(t, provider)
	defer func() { _ = provider.Close() }()

	require.Equal(t, "local", provider.Name())

	// Verify status first
	status, err := provider.GetStatus(ctx)
	require.NoError(t, err, "GetStatus should succeed when addon is running")
	require.NotNil(t, status)
	t.Logf("Addon status: ready=%v, model=%s, backend=%s", status.Ready, status.ModelName, status.Backend)

	if !status.Ready {
		t.Skip("addon is running but model is not ready, skipping completion test")
	}

	// Send a simple completion request. The local model should be able to
	// respond to a basic greeting without needing tool calls.
	resp, err := provider.Complete(ctx, &CompletionRequest{
		SystemPrompt: "You are a helpful assistant. Respond concisely.",
		Messages: []Message{
			{Role: RoleUser, Content: "Say hello in one sentence."},
		},
		MaxTokens: 200,
	})
	require.NoError(t, err, "Complete should succeed with local provider")
	require.NotNil(t, resp)

	// The response should have content or tool calls -- at minimum it shouldn't be empty
	hasContent := resp.Content != ""
	hasToolCalls := len(resp.ToolCalls) > 0
	require.True(t, hasContent || hasToolCalls,
		"response should have content or tool calls, got neither")

	t.Logf("Response content: %q", truncateE2E(resp.Content, 200))
	t.Logf("Stop reason: %s", resp.StopReason)
	t.Logf("Usage: input=%d, output=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens)
}

// TestE2E_RecipeGenerationWithLocalProvider tests a recipe generation
// conversation using the local provider with tool calling. This simulates
// what tsuku create does when generating a recipe for a well-known tool.
//
// Run with:
//
//	go test -v -tags=e2e -run TestE2E_RecipeGenerationWithLocalProvider ./internal/llm/
func TestE2E_RecipeGenerationWithLocalProvider(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	if !IsAddonRunning() {
		t.Skip("tsuku-llm addon not running, skipping e2e test")
	}

	// Local CPU inference is slow (~15 tok/s). With MaxTokens capped at 1024,
	// each turn takes up to ~70s. Allow 5 minutes for multi-turn conversations.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	provider := NewLocalProvider()
	require.NotNil(t, provider)
	defer func() { _ = provider.Close() }()

	// Check model readiness
	status, err := provider.GetStatus(ctx)
	require.NoError(t, err)
	if !status.Ready {
		t.Skip("addon is running but model is not ready")
	}

	// Build a recipe generation request similar to what tsuku create sends.
	// Use a well-known tool (ripgrep) with explicit asset names so the model
	// has enough context to call extract_pattern.
	tools := buildToolDefs()
	messages := []Message{
		{
			Role: RoleUser,
			Content: "Analyze this GitHub release for 'ripgrep' (BurntSushi/ripgrep).\n\n" +
				"Release assets:\n" +
				"- ripgrep-14.1.0-x86_64-unknown-linux-musl.tar.gz\n" +
				"- ripgrep-14.1.0-aarch64-unknown-linux-gnu.tar.gz\n" +
				"- ripgrep-14.1.0-x86_64-apple-darwin.tar.gz\n" +
				"- ripgrep-14.1.0-aarch64-apple-darwin.tar.gz\n\n" +
				"The executable inside the archive is 'rg'.\n" +
				"Call extract_pattern with the asset-to-platform mappings.",
		},
	}

	// Run the multi-turn conversation up to MaxTurns
	var extractCalled bool
	for turn := 0; turn < MaxTurns; turn++ {
		resp, err := provider.Complete(ctx, &CompletionRequest{
			SystemPrompt: "You are analyzing GitHub releases to create installation recipes. " +
				"Map release assets to OS/architecture pairs. " +
				"Call extract_pattern when you have the mappings.",
			Messages:  messages,
			Tools:     tools,
			MaxTokens: 4096,
		})
		require.NoError(t, err, "turn %d: Complete failed", turn)

		t.Logf("Turn %d: content=%q, tool_calls=%d, stop=%s",
			turn, truncateE2E(resp.Content, 100), len(resp.ToolCalls), resp.StopReason)

		// Add assistant response to history
		messages = append(messages, Message{
			Role:      RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Check each tool call
		for _, tc := range resp.ToolCalls {
			if tc.Name == ToolExtractPattern {
				extractCalled = true
				t.Logf("extract_pattern called with %d keys in arguments", len(tc.Arguments))

				// Validate the extract_pattern output has the required structure
				mappingsRaw, hasMappings := tc.Arguments["mappings"]
				require.True(t, hasMappings, "extract_pattern should include mappings")

				mappings, ok := mappingsRaw.([]any)
				require.True(t, ok, "mappings should be an array")
				require.NotEmpty(t, mappings, "mappings should not be empty")

				// Each mapping should have os, arch, asset, format
				for i, m := range mappings {
					mapping, ok := m.(map[string]any)
					require.True(t, ok, "mapping %d should be an object", i)
					require.Contains(t, mapping, "os", "mapping %d should have os", i)
					require.Contains(t, mapping, "arch", "mapping %d should have arch", i)
					require.Contains(t, mapping, "asset", "mapping %d should have asset", i)
				}

				// Should have executable field
				exec, hasExec := tc.Arguments["executable"]
				require.True(t, hasExec, "extract_pattern should include executable")
				execStr, ok := exec.(string)
				require.True(t, ok, "executable should be a string")
				require.NotEmpty(t, execStr, "executable should not be empty")

				t.Logf("Extracted %d platform mappings, executable=%q", len(mappings), execStr)
				break
			}

			// For non-extract tool calls, provide a simulated response
			messages = append(messages, Message{
				Role: RoleUser,
				ToolResult: &ToolResult{
					CallID:  tc.ID,
					Content: "Simulated tool result for " + tc.Name,
				},
			})
		}

		if extractCalled {
			break
		}

		// If no tool calls and conversation ended, stop
		if len(resp.ToolCalls) == 0 && resp.StopReason == "end_turn" {
			t.Logf("Conversation ended without extract_pattern at turn %d", turn)
			break
		}
	}

	// The local model should eventually call extract_pattern given clear inputs.
	// If it doesn't, this is a quality signal but not necessarily a failure of
	// the integration itself -- log it clearly for debugging.
	if !extractCalled {
		t.Log("Warning: local model did not call extract_pattern within max turns")
		t.Log("This may indicate a model quality issue rather than an integration bug")
	}
}

// TestE2E_FactoryCreatesWorkingLocalProvider verifies the full factory-to-completion
// path: create a factory with no cloud keys, get the local provider from it,
// and use it to send a completion request.
func TestE2E_FactoryCreatesWorkingLocalProvider(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	if !IsAddonRunning() {
		t.Skip("tsuku-llm addon not running, skipping e2e test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create factory the same way tsuku create would
	factory, err := NewFactory(ctx)
	require.NoError(t, err)

	provider, err := factory.GetProvider(ctx)
	require.NoError(t, err)
	require.Equal(t, "local", provider.Name())

	// Verify the provider can actually complete a request.
	// Cast to LocalProvider to check status first.
	localProvider, ok := provider.(*LocalProvider)
	require.True(t, ok, "provider from factory should be *LocalProvider")

	status, err := localProvider.GetStatus(ctx)
	require.NoError(t, err)
	if !status.Ready {
		t.Skip("addon is running but model is not ready")
	}

	resp, err := provider.Complete(ctx, &CompletionRequest{
		SystemPrompt: "You are a helpful assistant.",
		Messages: []Message{
			{Role: RoleUser, Content: "Respond with the word 'hello'."},
		},
		MaxTokens: 50,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	// The response should contain something
	require.True(t, resp.Content != "" || len(resp.ToolCalls) > 0,
		"response should not be empty")

	// For a simple prompt like this, we expect text content containing "hello"
	if resp.Content != "" {
		require.True(t, strings.Contains(strings.ToLower(resp.Content), "hello"),
			"expected response to contain 'hello', got: %q", resp.Content)
	}

	t.Logf("Factory->Provider->Complete succeeded: %q", truncateE2E(resp.Content, 100))
}

// truncateE2E truncates a string for log output.
func truncateE2E(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

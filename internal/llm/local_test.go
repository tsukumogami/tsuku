package llm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSocketPath(t *testing.T) {
	t.Run("uses TSUKU_HOME when set", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		path := SocketPath()
		require.Equal(t, filepath.Join(tmpDir, "llm.sock"), path)
	})

	t.Run("defaults to ~/.tsuku when TSUKU_HOME not set", func(t *testing.T) {
		t.Setenv("TSUKU_HOME", "")

		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)

		path := SocketPath()
		require.Equal(t, filepath.Join(homeDir, ".tsuku", "llm.sock"), path)
	})
}

func TestIsAddonRunning(t *testing.T) {
	t.Run("returns false when socket does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		require.False(t, IsAddonRunning())
	})
}

func TestToProtoRole(t *testing.T) {
	tests := []struct {
		name     string
		role     Role
		expected int32
	}{
		{"user role", RoleUser, 1},
		{"assistant role", RoleAssistant, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toProtoRole(tt.role)
			require.Equal(t, tt.expected, int32(result))
		})
	}
}

// TestLocalProviderIntegration tests end-to-end communication with the tsuku-llm addon.
// Run with: go test -v -run TestLocalProviderIntegration ./internal/llm/
// Requires tsuku-llm to be running.
func TestLocalProviderIntegration(t *testing.T) {
	if !IsAddonRunning() {
		t.Skip("tsuku-llm addon not running, skipping integration test")
	}

	ctx := context.Background()

	provider, err := NewLocalProvider(ctx)
	require.NoError(t, err, "Failed to create local provider")
	defer func() { _ = provider.Close() }()

	t.Run("GetStatus", func(t *testing.T) {
		status, err := provider.GetStatus(ctx)
		require.NoError(t, err, "GetStatus failed")
		require.NotNil(t, status)
		t.Logf("Addon status: ready=%v, model=%s, backend=%s", status.Ready, status.ModelName, status.Backend)
	})

	t.Run("Complete", func(t *testing.T) {
		req := &CompletionRequest{
			SystemPrompt: "You are a helpful assistant.",
			Messages: []Message{
				{Role: RoleUser, Content: "Say hello"},
			},
			MaxTokens: 100,
		}

		resp, err := provider.Complete(ctx, req)
		require.NoError(t, err, "Complete failed")
		require.NotNil(t, resp)
		t.Logf("Response: content=%q, stop_reason=%s", resp.Content, resp.StopReason)
		t.Logf("Usage: input=%d, output=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	})
}

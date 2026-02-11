package llm

import (
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

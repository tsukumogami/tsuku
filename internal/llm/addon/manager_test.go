package addon

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddonPath(t *testing.T) {
	t.Run("uses TSUKU_HOME when set", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		path := AddonPath()

		binName := "tsuku-llm"
		if runtime.GOOS == "windows" {
			binName = "tsuku-llm.exe"
		}

		require.Equal(t, filepath.Join(tmpDir, "tools", "tsuku-llm", binName), path)
	})

	t.Run("defaults to ~/.tsuku when TSUKU_HOME not set", func(t *testing.T) {
		t.Setenv("TSUKU_HOME", "")

		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)

		path := AddonPath()

		binName := "tsuku-llm"
		if runtime.GOOS == "windows" {
			binName = "tsuku-llm.exe"
		}

		require.Equal(t, filepath.Join(homeDir, ".tsuku", "tools", "tsuku-llm", binName), path)
	})
}

func TestIsInstalled(t *testing.T) {
	t.Run("returns false when addon not installed", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		require.False(t, IsInstalled())
	})

	t.Run("returns true when addon exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		// Create the addon binary
		binName := "tsuku-llm"
		if runtime.GOOS == "windows" {
			binName = "tsuku-llm.exe"
		}
		addonDir := filepath.Join(tmpDir, "tools", "tsuku-llm")
		require.NoError(t, os.MkdirAll(addonDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(addonDir, binName), []byte("fake"), 0755))

		require.True(t, IsInstalled())
	})
}

func TestNewManager(t *testing.T) {
	m := NewManager()
	require.NotNil(t, m)
	require.False(t, m.IsRunning())
}

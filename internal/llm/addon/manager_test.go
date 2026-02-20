package addon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// mockInstaller is a test double for the Installer interface.
type mockInstaller struct {
	installCalls []installCall
	installErr   error
	// onInstall simulates installing by creating a binary in the tools dir
	onInstall func(homeDir string)
}

type installCall struct {
	recipeName  string
	gpuOverride string
}

func (m *mockInstaller) InstallRecipe(ctx context.Context, recipeName string, gpuOverride string) error {
	m.installCalls = append(m.installCalls, installCall{recipeName: recipeName, gpuOverride: gpuOverride})
	if m.onInstall != nil {
		// The test doesn't know the homeDir; callers set onInstall with closure over it
		m.onInstall("")
	}
	if m.installErr != nil {
		return m.installErr
	}
	return nil
}

// createFakeBinary creates a fake tsuku-llm binary at the recipe tools path.
func createFakeBinary(t *testing.T, homeDir, version string) string {
	t.Helper()
	binName := "tsuku-llm"
	if runtime.GOOS == "windows" {
		binName = "tsuku-llm.exe"
	}
	toolDir := filepath.Join(homeDir, "tools", "tsuku-llm-"+version, "bin")
	require.NoError(t, os.MkdirAll(toolDir, 0755))
	binPath := filepath.Join(toolDir, binName)
	require.NoError(t, os.WriteFile(binPath, []byte("#!/bin/sh\necho hello"), 0755))
	return binPath
}

func TestNewAddonManager(t *testing.T) {
	t.Run("uses provided home dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		m := NewAddonManager(tmpDir, nil, "")
		require.Equal(t, tmpDir, m.HomeDir())
	})

	t.Run("uses TSUKU_HOME when homeDir is empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)
		m := NewAddonManager("", nil, "")
		require.Equal(t, tmpDir, m.HomeDir())
	})
}

func TestEnsureAddon_ExplicitBinaryPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a binary at a custom location
	customBin := filepath.Join(tmpDir, "custom-tsuku-llm")
	require.NoError(t, os.WriteFile(customBin, []byte("#!/bin/sh\necho hello"), 0755))

	t.Setenv("TSUKU_LLM_BINARY", customBin)

	installer := &mockInstaller{}
	m := NewAddonManager(tmpDir, installer, "")

	path, err := m.EnsureAddon(context.Background())
	require.NoError(t, err)
	require.Equal(t, customBin, path)
	require.Empty(t, installer.installCalls, "should not call installer when TSUKU_LLM_BINARY is set")
}

func TestEnsureAddon_ExplicitBinaryPath_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("TSUKU_LLM_BINARY", filepath.Join(tmpDir, "nonexistent"))

	// Falls through to recipe installation when explicit path doesn't exist
	installer := &mockInstaller{
		onInstall: func(_ string) {
			createFakeBinary(t, tmpDir, "1.0.0")
		},
	}
	m := NewAddonManager(tmpDir, installer, "")

	path, err := m.EnsureAddon(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, path)
	require.Len(t, installer.installCalls, 1, "should fall through to installer when explicit binary missing")
}

func TestEnsureAddon_AlreadyInstalled(t *testing.T) {
	t.Setenv("TSUKU_LLM_BINARY", "")
	tmpDir := t.TempDir()
	binPath := createFakeBinary(t, tmpDir, "1.0.0")

	installer := &mockInstaller{}
	m := NewAddonManager(tmpDir, installer, "")

	path, err := m.EnsureAddon(context.Background())
	require.NoError(t, err)
	require.Equal(t, binPath, path)
	require.Empty(t, installer.installCalls, "should not call installer when binary exists")
}

func TestEnsureAddon_NotInstalled_InstallsViaRecipe(t *testing.T) {
	t.Setenv("TSUKU_LLM_BINARY", "")
	tmpDir := t.TempDir()

	installer := &mockInstaller{
		onInstall: func(_ string) {
			// Simulate recipe installation creating the binary
			createFakeBinary(t, tmpDir, "1.0.0")
		},
	}
	m := NewAddonManager(tmpDir, installer, "")

	path, err := m.EnsureAddon(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, path)

	// Verify installer was called correctly
	require.Len(t, installer.installCalls, 1)
	require.Equal(t, "tsuku-llm", installer.installCalls[0].recipeName)
	require.Equal(t, "", installer.installCalls[0].gpuOverride, "no override when backend is empty")
}

func TestEnsureAddon_InstallError(t *testing.T) {
	t.Setenv("TSUKU_LLM_BINARY", "")
	tmpDir := t.TempDir()

	installer := &mockInstaller{
		installErr: fmt.Errorf("network timeout"),
	}
	m := NewAddonManager(tmpDir, installer, "")

	_, err := m.EnsureAddon(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "installing tsuku-llm")
	require.Contains(t, err.Error(), "network timeout")
}

func TestEnsureAddon_NoInstaller(t *testing.T) {
	t.Setenv("TSUKU_LLM_BINARY", "")
	tmpDir := t.TempDir()
	m := NewAddonManager(tmpDir, nil, "")

	_, err := m.EnsureAddon(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "no installer configured")
}

func TestEnsureAddon_CPUOverride_SetsGPUToNone(t *testing.T) {
	t.Setenv("TSUKU_LLM_BINARY", "")
	tmpDir := t.TempDir()

	installer := &mockInstaller{
		onInstall: func(_ string) {
			createFakeBinary(t, tmpDir, "1.0.0")
		},
	}
	m := NewAddonManager(tmpDir, installer, "cpu")

	path, err := m.EnsureAddon(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, path)

	require.Len(t, installer.installCalls, 1)
	require.Equal(t, "none", installer.installCalls[0].gpuOverride, "should override GPU to 'none' when backend is 'cpu'")
}

func TestEnsureAddon_VariantMismatch_Reinstalls(t *testing.T) {
	t.Setenv("TSUKU_LLM_BINARY", "")
	tmpDir := t.TempDir()

	// Pre-install a binary (simulates GPU variant already installed)
	createFakeBinary(t, tmpDir, "1.0.0")

	installCount := 0
	installer := &mockInstaller{
		onInstall: func(_ string) {
			installCount++
			// Recipe system replaces the existing binary
			createFakeBinary(t, tmpDir, "1.0.0")
		},
	}

	// User sets llm.backend=cpu but there's already an installation
	m := NewAddonManager(tmpDir, installer, "cpu")

	path, err := m.EnsureAddon(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, path)
	require.Equal(t, 1, installCount, "should reinstall when variant mismatch detected")
	require.Equal(t, "none", installer.installCalls[0].gpuOverride)
}

func TestEnsureAddon_CachesPath(t *testing.T) {
	t.Setenv("TSUKU_LLM_BINARY", "")
	tmpDir := t.TempDir()
	createFakeBinary(t, tmpDir, "1.0.0")

	installer := &mockInstaller{}
	m := NewAddonManager(tmpDir, installer, "")

	// First call
	path1, err := m.EnsureAddon(context.Background())
	require.NoError(t, err)

	// Second call should use cached path
	path2, err := m.EnsureAddon(context.Background())
	require.NoError(t, err)
	require.Equal(t, path1, path2)
	require.Empty(t, installer.installCalls, "should not install twice")
}

func TestEnsureAddon_CacheClearedWhenBinaryRemoved(t *testing.T) {
	t.Setenv("TSUKU_LLM_BINARY", "")
	tmpDir := t.TempDir()
	binPath := createFakeBinary(t, tmpDir, "1.0.0")

	installer := &mockInstaller{
		onInstall: func(_ string) {
			createFakeBinary(t, tmpDir, "2.0.0")
		},
	}
	m := NewAddonManager(tmpDir, installer, "")

	// First call caches the path
	path1, err := m.EnsureAddon(context.Background())
	require.NoError(t, err)
	require.Equal(t, binPath, path1)

	// Remove the binary
	require.NoError(t, os.RemoveAll(filepath.Dir(filepath.Dir(binPath))))

	// Second call should detect removal and reinstall
	path2, err := m.EnsureAddon(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, path2)
	require.Len(t, installer.installCalls, 1)
}

func TestFindInstalledBinary_FlatLayout(t *testing.T) {
	tmpDir := t.TempDir()

	// Create binary in flat layout (no bin/ subdirectory)
	binName := "tsuku-llm"
	if runtime.GOOS == "windows" {
		binName = "tsuku-llm.exe"
	}
	toolDir := filepath.Join(tmpDir, "tools", "tsuku-llm-1.0.0")
	require.NoError(t, os.MkdirAll(toolDir, 0755))
	flatPath := filepath.Join(toolDir, binName)
	require.NoError(t, os.WriteFile(flatPath, []byte("fake"), 0755))

	m := NewAddonManager(tmpDir, nil, "")
	found := m.findInstalledBinary()
	require.Equal(t, flatPath, found)
}

func TestFindInstalledBinary_BinLayout(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := createFakeBinary(t, tmpDir, "2.0.0")

	m := NewAddonManager(tmpDir, nil, "")
	found := m.findInstalledBinary()
	require.Equal(t, binPath, found)
}

func TestFindInstalledBinary_NoTools(t *testing.T) {
	tmpDir := t.TempDir()

	m := NewAddonManager(tmpDir, nil, "")
	found := m.findInstalledBinary()
	require.Empty(t, found)
}

func TestFindInstalledBinary_OtherToolsOnly(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some other tool, not tsuku-llm
	otherDir := filepath.Join(tmpDir, "tools", "some-tool-1.0.0", "bin")
	require.NoError(t, os.MkdirAll(otherDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(otherDir, "some-tool"), []byte("fake"), 0755))

	m := NewAddonManager(tmpDir, nil, "")
	found := m.findInstalledBinary()
	require.Empty(t, found)
}

func TestCleanupLegacyPath(t *testing.T) {
	t.Run("removes legacy addons directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		legacyPath := filepath.Join(tmpDir, "addons", "tsuku-llm")
		require.NoError(t, os.MkdirAll(legacyPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(legacyPath, "tsuku-llm"), []byte("old"), 0755))

		m := NewAddonManager(tmpDir, nil, "")
		m.cleanupLegacyPath()

		_, err := os.Stat(legacyPath)
		require.True(t, os.IsNotExist(err), "legacy addons dir should be removed")
	})

	t.Run("removes old tools/tsuku-llm directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		legacyPath := filepath.Join(tmpDir, "tools", "tsuku-llm")
		require.NoError(t, os.MkdirAll(filepath.Join(legacyPath, "0.1.0"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(legacyPath, "0.1.0", "tsuku-llm"), []byte("old"), 0755))

		m := NewAddonManager(tmpDir, nil, "")
		m.cleanupLegacyPath()

		_, err := os.Stat(legacyPath)
		require.True(t, os.IsNotExist(err), "legacy tools/tsuku-llm dir should be removed")
	})

	t.Run("does nothing when no legacy paths exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		m := NewAddonManager(tmpDir, nil, "")
		// Should not panic
		m.cleanupLegacyPath()
	})
}

func TestBinaryName(t *testing.T) {
	name := binaryName()
	if runtime.GOOS == "windows" {
		require.Equal(t, "tsuku-llm.exe", name)
	} else {
		require.Equal(t, "tsuku-llm", name)
	}
}

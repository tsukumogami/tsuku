package addon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManifest(t *testing.T) {
	t.Run("parses embedded manifest", func(t *testing.T) {
		manifest, err := GetManifest()
		require.NoError(t, err)
		require.NotNil(t, manifest)
		require.NotEmpty(t, manifest.Version)
		require.NotEmpty(t, manifest.Platforms)
	})

	t.Run("has current platform", func(t *testing.T) {
		manifest, err := GetManifest()
		require.NoError(t, err)

		info, err := manifest.GetPlatformInfo(PlatformKey())
		require.NoError(t, err)
		require.NotEmpty(t, info.URL)
		require.NotEmpty(t, info.SHA256)
	})

	t.Run("returns error for unsupported platform", func(t *testing.T) {
		manifest, err := GetManifest()
		require.NoError(t, err)

		_, err = manifest.GetPlatformInfo("unsupported-platform")
		require.Error(t, err)
		require.Contains(t, err.Error(), "unsupported platform")
	})
}

// TestManifestURLsUseHTTPS validates that all platform URLs in the embedded
// manifest use the HTTPS scheme. This is a regression test for a bug where
// a developer's local file:// URL was committed for linux-amd64, which the
// HTTP downloader cannot handle.
func TestManifestURLsUseHTTPS(t *testing.T) {
	manifest, err := GetManifest()
	require.NoError(t, err)

	for platform, info := range manifest.Platforms {
		u, err := url.Parse(info.URL)
		require.NoError(t, err, "platform %s has unparseable URL: %s", platform, info.URL)
		require.Equal(t, "https", u.Scheme,
			"platform %s URL must use https, got %q: %s", platform, u.Scheme, info.URL)
	}
}

func TestPlatformKey(t *testing.T) {
	key := PlatformKey()
	require.Equal(t, runtime.GOOS+"-"+runtime.GOARCH, key)
}

func TestBinaryName(t *testing.T) {
	name := BinaryName()
	if runtime.GOOS == "windows" {
		require.Equal(t, "tsuku-llm.exe", name)
	} else {
		require.Equal(t, "tsuku-llm", name)
	}
}

func TestAddonPath(t *testing.T) {
	manifest, err := GetManifest()
	require.NoError(t, err)

	t.Run("uses TSUKU_HOME when set", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		path := AddonPath()

		binName := "tsuku-llm"
		if runtime.GOOS == "windows" {
			binName = "tsuku-llm.exe"
		}

		// Now includes version in path
		require.Equal(t, filepath.Join(tmpDir, "tools", "tsuku-llm", manifest.Version, binName), path)
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

		// Now includes version in path
		require.Equal(t, filepath.Join(homeDir, ".tsuku", "tools", "tsuku-llm", manifest.Version, binName), path)
	})
}

func TestIsInstalled(t *testing.T) {
	manifest, err := GetManifest()
	require.NoError(t, err)

	t.Run("returns false when addon not installed", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		require.False(t, IsInstalled())
	})

	t.Run("returns true when addon exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		// Create the addon binary at versioned path
		binName := "tsuku-llm"
		if runtime.GOOS == "windows" {
			binName = "tsuku-llm.exe"
		}
		addonDir := filepath.Join(tmpDir, "tools", "tsuku-llm", manifest.Version)
		require.NoError(t, os.MkdirAll(addonDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(addonDir, binName), []byte("fake"), 0755))

		require.True(t, IsInstalled())
	})
}

func TestAddonManager(t *testing.T) {
	t.Run("creates manager with custom home", func(t *testing.T) {
		tmpDir := t.TempDir()
		m := NewAddonManagerWithHome(tmpDir)
		require.Equal(t, tmpDir, m.HomeDir())
	})

	t.Run("creates manager with TSUKU_HOME env", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		m := NewAddonManager()
		require.Equal(t, tmpDir, m.HomeDir())
	})

	t.Run("returns correct addon dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		m := NewAddonManagerWithHome(tmpDir)

		manifest, err := GetManifest()
		require.NoError(t, err)

		dir, err := m.AddonDir()
		require.NoError(t, err)
		require.Equal(t, filepath.Join(tmpDir, "tools", "tsuku-llm", manifest.Version), dir)
	})

	t.Run("returns correct binary path", func(t *testing.T) {
		tmpDir := t.TempDir()
		m := NewAddonManagerWithHome(tmpDir)

		manifest, err := GetManifest()
		require.NoError(t, err)

		path, err := m.BinaryPath()
		require.NoError(t, err)
		require.Equal(t, filepath.Join(tmpDir, "tools", "tsuku-llm", manifest.Version, BinaryName()), path)
	})

	t.Run("IsInstalled returns false when not present", func(t *testing.T) {
		tmpDir := t.TempDir()
		m := NewAddonManagerWithHome(tmpDir)
		require.False(t, m.IsInstalled())
	})

	t.Run("IsInstalled returns true when present", func(t *testing.T) {
		tmpDir := t.TempDir()
		m := NewAddonManagerWithHome(tmpDir)

		// Create the binary
		dir, err := m.AddonDir()
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(dir, 0755))

		path, err := m.BinaryPath()
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, []byte("fake"), 0755))

		require.True(t, m.IsInstalled())
	})
}

func TestVerifyChecksum(t *testing.T) {
	t.Run("verifies correct checksum", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test")
		content := []byte("hello world")
		require.NoError(t, os.WriteFile(filePath, content, 0644))

		// Compute expected checksum
		h := sha256.Sum256(content)
		expected := hex.EncodeToString(h[:])

		err := VerifyChecksum(filePath, expected)
		require.NoError(t, err)
	})

	t.Run("rejects incorrect checksum", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test")
		require.NoError(t, os.WriteFile(filePath, []byte("hello world"), 0644))

		err := VerifyChecksum(filePath, "badchecksum")
		require.Error(t, err)
		require.Contains(t, err.Error(), "checksum mismatch")
	})

	t.Run("handles uppercase checksum", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test")
		content := []byte("hello world")
		require.NoError(t, os.WriteFile(filePath, content, 0644))

		h := sha256.Sum256(content)
		expected := hex.EncodeToString(h[:])

		// Pass uppercase version
		err := VerifyChecksum(filePath, "  "+expected+"  ") // With whitespace
		require.NoError(t, err)
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		err := VerifyChecksum("/nonexistent/file", "checksum")
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to open file")
	})
}

func TestComputeChecksum(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test")
	content := []byte("hello world")
	require.NoError(t, os.WriteFile(filePath, content, 0644))

	h := sha256.Sum256(content)
	expected := hex.EncodeToString(h[:])

	actual, err := ComputeChecksum(filePath)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestEnsureAddon(t *testing.T) {
	t.Run("downloads and verifies addon", func(t *testing.T) {
		// Create a fake addon binary
		fakeAddon := []byte("#!/bin/sh\necho hello")
		h := sha256.Sum256(fakeAddon)
		checksum := hex.EncodeToString(h[:])

		// Set up mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(fakeAddon)
		}))
		defer server.Close()

		// Create temp manifest with mock URL
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		// Temporarily override the manifest for testing
		oldManifest := cachedManifest
		cachedManifest = &Manifest{
			Version: "test",
			Platforms: map[string]PlatformInfo{
				PlatformKey(): {
					URL:    server.URL + "/tsuku-llm",
					SHA256: checksum,
				},
			},
		}
		defer func() { cachedManifest = oldManifest }()

		m := NewAddonManagerWithHome(tmpDir)
		path, err := m.EnsureAddon(context.Background())
		require.NoError(t, err)
		require.NotEmpty(t, path)

		// Verify the file exists and is executable
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.True(t, info.Mode()&0100 != 0, "binary should be executable")

		// Verify content
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, fakeAddon, content)
	})

	t.Run("uses existing verified addon", func(t *testing.T) {
		fakeAddon := []byte("fake addon content")
		h := sha256.Sum256(fakeAddon)
		checksum := hex.EncodeToString(h[:])

		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		// Set up manifest
		oldManifest := cachedManifest
		cachedManifest = &Manifest{
			Version: "test",
			Platforms: map[string]PlatformInfo{
				PlatformKey(): {
					URL:    "http://should-not-be-called/",
					SHA256: checksum,
				},
			},
		}
		defer func() { cachedManifest = oldManifest }()

		m := NewAddonManagerWithHome(tmpDir)

		// Pre-create the binary
		dir, err := m.AddonDir()
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(dir, 0755))

		binaryPath, err := m.BinaryPath()
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(binaryPath, fakeAddon, 0755))

		// EnsureAddon should return existing path without downloading
		path, err := m.EnsureAddon(context.Background())
		require.NoError(t, err)
		require.Equal(t, binaryPath, path)
	})

	t.Run("re-downloads on checksum mismatch", func(t *testing.T) {
		newAddon := []byte("new addon content")
		h := sha256.Sum256(newAddon)
		checksum := hex.EncodeToString(h[:])

		downloadCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			downloadCount++
			_, _ = w.Write(newAddon)
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		oldManifest := cachedManifest
		cachedManifest = &Manifest{
			Version: "test",
			Platforms: map[string]PlatformInfo{
				PlatformKey(): {
					URL:    server.URL + "/tsuku-llm",
					SHA256: checksum,
				},
			},
		}
		defer func() { cachedManifest = oldManifest }()

		m := NewAddonManagerWithHome(tmpDir)

		// Pre-create a binary with wrong content
		dir, err := m.AddonDir()
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(dir, 0755))

		binaryPath, err := m.BinaryPath()
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(binaryPath, []byte("wrong content"), 0755))

		// EnsureAddon should detect mismatch and re-download
		path, err := m.EnsureAddon(context.Background())
		require.NoError(t, err)
		require.Equal(t, binaryPath, path)
		require.Equal(t, 1, downloadCount)

		// Verify new content
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, newAddon, content)
	})
}

func TestEnsureAddonWithPrompter(t *testing.T) {
	t.Run("prompts before download and proceeds on approval", func(t *testing.T) {
		fakeAddon := []byte("#!/bin/sh\necho hello")
		h := sha256.Sum256(fakeAddon)
		checksum := hex.EncodeToString(h[:])

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(fakeAddon)
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		oldManifest := cachedManifest
		cachedManifest = &Manifest{
			Version: "test",
			Platforms: map[string]PlatformInfo{
				PlatformKey(): {
					URL:    server.URL + "/tsuku-llm",
					SHA256: checksum,
				},
			},
		}
		defer func() { cachedManifest = oldManifest }()

		prompted := false
		mockPrompter := &testPrompter{
			approve: true,
			onPrompt: func(desc string, size int64) {
				prompted = true
				require.Contains(t, desc, "tsuku-llm")
				require.Greater(t, size, int64(0))
			},
		}

		m := NewAddonManagerWithHome(tmpDir)
		m.SetPrompter(mockPrompter)

		path, err := m.EnsureAddon(context.Background())
		require.NoError(t, err)
		require.NotEmpty(t, path)
		require.True(t, prompted, "prompter should have been called")
	})

	t.Run("returns error when user declines download", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		oldManifest := cachedManifest
		cachedManifest = &Manifest{
			Version: "test",
			Platforms: map[string]PlatformInfo{
				PlatformKey(): {
					URL:    "http://should-not-be-called/",
					SHA256: "fake",
				},
			},
		}
		defer func() { cachedManifest = oldManifest }()

		mockPrompter := &testPrompter{approve: false}

		m := NewAddonManagerWithHome(tmpDir)
		m.SetPrompter(mockPrompter)

		_, err := m.EnsureAddon(context.Background())
		require.ErrorIs(t, err, ErrDownloadDeclined)
	})

	t.Run("skips prompt when addon already installed", func(t *testing.T) {
		fakeAddon := []byte("fake addon content")
		h := sha256.Sum256(fakeAddon)
		checksum := hex.EncodeToString(h[:])

		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		oldManifest := cachedManifest
		cachedManifest = &Manifest{
			Version: "test",
			Platforms: map[string]PlatformInfo{
				PlatformKey(): {
					URL:    "http://should-not-be-called/",
					SHA256: checksum,
				},
			},
		}
		defer func() { cachedManifest = oldManifest }()

		prompted := false
		mockPrompter := &testPrompter{
			approve:  true,
			onPrompt: func(_ string, _ int64) { prompted = true },
		}

		m := NewAddonManagerWithHome(tmpDir)
		m.SetPrompter(mockPrompter)

		// Pre-create the binary
		dir, err := m.AddonDir()
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(dir, 0755))

		binaryPath, err := m.BinaryPath()
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(binaryPath, fakeAddon, 0755))

		path, err := m.EnsureAddon(context.Background())
		require.NoError(t, err)
		require.Equal(t, binaryPath, path)
		require.False(t, prompted, "should not prompt when addon exists and is valid")
	})

	t.Run("downloads without prompt when no prompter set", func(t *testing.T) {
		fakeAddon := []byte("#!/bin/sh\necho hello")
		h := sha256.Sum256(fakeAddon)
		checksum := hex.EncodeToString(h[:])

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(fakeAddon)
		}))
		defer server.Close()

		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		oldManifest := cachedManifest
		cachedManifest = &Manifest{
			Version: "test",
			Platforms: map[string]PlatformInfo{
				PlatformKey(): {
					URL:    server.URL + "/tsuku-llm",
					SHA256: checksum,
				},
			},
		}
		defer func() { cachedManifest = oldManifest }()

		m := NewAddonManagerWithHome(tmpDir)
		// No prompter set -- should download silently (backward compatible)

		path, err := m.EnsureAddon(context.Background())
		require.NoError(t, err)
		require.NotEmpty(t, path)
	})
}

// testPrompter is a mock Prompter for testing.
type testPrompter struct {
	approve  bool
	onPrompt func(description string, size int64)
}

func (p *testPrompter) ConfirmDownload(_ context.Context, description string, size int64) (bool, error) {
	if p.onPrompt != nil {
		p.onPrompt(description, size)
	}
	return p.approve, nil
}

func TestVerifyBeforeExecution(t *testing.T) {
	t.Run("returns nil for valid binary", func(t *testing.T) {
		fakeAddon := []byte("fake addon")
		h := sha256.Sum256(fakeAddon)
		checksum := hex.EncodeToString(h[:])

		tmpDir := t.TempDir()

		oldManifest := cachedManifest
		cachedManifest = &Manifest{
			Version: "test",
			Platforms: map[string]PlatformInfo{
				PlatformKey(): {
					URL:    "http://unused/",
					SHA256: checksum,
				},
			},
		}
		defer func() { cachedManifest = oldManifest }()

		binaryPath := filepath.Join(tmpDir, "tsuku-llm")
		require.NoError(t, os.WriteFile(binaryPath, fakeAddon, 0755))

		m := NewAddonManagerWithHome(tmpDir)
		err := m.VerifyBeforeExecution(binaryPath)
		require.NoError(t, err)
	})

	t.Run("returns error for tampered binary", func(t *testing.T) {
		fakeAddon := []byte("fake addon")
		h := sha256.Sum256(fakeAddon)
		checksum := hex.EncodeToString(h[:])

		tmpDir := t.TempDir()

		oldManifest := cachedManifest
		cachedManifest = &Manifest{
			Version: "test",
			Platforms: map[string]PlatformInfo{
				PlatformKey(): {
					URL:    "http://unused/",
					SHA256: checksum,
				},
			},
		}
		defer func() { cachedManifest = oldManifest }()

		binaryPath := filepath.Join(tmpDir, "tsuku-llm")
		require.NoError(t, os.WriteFile(binaryPath, []byte("tampered"), 0755))

		m := NewAddonManagerWithHome(tmpDir)
		err := m.VerifyBeforeExecution(binaryPath)
		require.Error(t, err)
		require.Contains(t, err.Error(), "checksum mismatch")
	})
}

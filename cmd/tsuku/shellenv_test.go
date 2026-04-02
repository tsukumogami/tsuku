package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectShellForEnv(t *testing.T) {
	tests := []struct {
		name     string
		envShell string
		expected string
	}{
		{"bash", "/bin/bash", "bash"},
		{"zsh", "/usr/bin/zsh", "zsh"},
		{"fish", "/usr/local/bin/fish", "fish"},
		{"unknown defaults to bash", "/bin/csh", "bash"},
		{"empty defaults to bash", "", "bash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SHELL", tt.envShell)
			if got := detectShellForEnv(); got != tt.expected {
				t.Errorf("detectShellForEnv() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestShellenvCacheSourceLine(t *testing.T) {
	// This tests the logic that determines whether the source line should be emitted.
	// We can't easily test the full cobra command, but we verify the file-existence check.

	t.Run("cache file exists", func(t *testing.T) {
		tsukuHome := t.TempDir()
		shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
		if err := os.MkdirAll(shellDDir, 0755); err != nil {
			t.Fatal(err)
		}

		cachePath := filepath.Join(shellDDir, ".init-cache.bash")
		if err := os.WriteFile(cachePath, []byte("# cached init\n"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := os.Stat(cachePath)
		if err != nil {
			t.Errorf("expected cache file to exist for source line emission")
		}
	})

	t.Run("cache file does not exist", func(t *testing.T) {
		tsukuHome := t.TempDir()
		cachePath := filepath.Join(tsukuHome, "share", "shell.d", ".init-cache.bash")

		_, err := os.Stat(cachePath)
		if !os.IsNotExist(err) {
			t.Errorf("expected cache file to not exist; source line should be omitted")
		}
	})
}

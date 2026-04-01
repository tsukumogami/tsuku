package shellenv

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTestFile is a helper that writes a file and fails the test on error.
func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRebuildShellCache_ConcatenatesFiles(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "aaa.bash"), "# aaa\n")
	writeTestFile(t, filepath.Join(shellDDir, "bbb.bash"), "# bbb\n")

	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatalf("RebuildShellCache failed: %v", err)
	}

	cachePath := filepath.Join(shellDDir, ".init-cache.bash")
	content, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("expected cache file to exist: %v", err)
	}

	expected := "# aaa\n# bbb\n"
	if string(content) != expected {
		t.Errorf("expected cache content %q, got %q", expected, string(content))
	}
}

func TestRebuildShellCache_SortedAlphabetically(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "zzz.zsh"), "# zzz\n")
	writeTestFile(t, filepath.Join(shellDDir, "aaa.zsh"), "# aaa\n")

	if err := RebuildShellCache(tsukuHome, "zsh"); err != nil {
		t.Fatalf("RebuildShellCache failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.zsh"))
	if err != nil {
		t.Fatal(err)
	}

	expected := "# aaa\n# zzz\n"
	if string(content) != expected {
		t.Errorf("expected sorted content %q, got %q", expected, string(content))
	}
}

func TestRebuildShellCache_ExcludesCacheFile(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "tool.bash"), "# tool\n")
	writeTestFile(t, filepath.Join(shellDDir, ".init-cache.bash"), "# old cache\n")

	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.bash"))
	if err != nil {
		t.Fatal(err)
	}

	expected := "# tool\n"
	if string(content) != expected {
		t.Errorf("expected %q, got %q (cache file should not include itself)", expected, string(content))
	}
}

func TestRebuildShellCache_RemovesCacheWhenEmpty(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	cachePath := filepath.Join(shellDDir, ".init-cache.bash")
	writeTestFile(t, cachePath, "# stale\n")

	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Error("expected cache file to be removed when no source files exist")
	}
}

func TestRebuildShellCache_NoDirectoryNoop(t *testing.T) {
	tsukuHome := t.TempDir()

	err := RebuildShellCache(tsukuHome, "bash")
	if err != nil {
		t.Fatalf("expected no error for missing directory, got: %v", err)
	}
}

func TestRebuildShellCache_OnlyMatchesCorrectShell(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "tool.bash"), "# bash\n")
	writeTestFile(t, filepath.Join(shellDDir, "tool.zsh"), "# zsh\n")

	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.bash"))
	if err != nil {
		t.Fatal(err)
	}

	if string(content) != "# bash\n" {
		t.Errorf("expected only bash content, got %q", string(content))
	}
}

func TestRebuildShellCache_AddsTrailingNewline(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "tool.bash"), "# no newline")

	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.bash"))
	if err != nil {
		t.Fatal(err)
	}

	expected := "# no newline\n"
	if string(content) != expected {
		t.Errorf("expected trailing newline added: %q, got %q", expected, string(content))
	}
}

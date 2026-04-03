package shellenv

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestFile is a helper that writes a file and fails the test on error.
func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// testHash computes the SHA-256 hex digest of a string.
func testHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// wrapExpected wraps content in the error-isolated block format that
// RebuildShellCache produces. toolName is the filename without the shell
// extension (e.g., "aaa" for "aaa.bash").
func wrapExpected(toolName, content string) string {
	s := "# tsuku: " + toolName + "\n"
	s += "{ # begin " + toolName + "\n"
	s += content
	if len(content) > 0 && content[len(content)-1] != '\n' {
		s += "\n"
	}
	s += "} 2>/dev/null || true\n"
	return s
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

	expected := wrapExpected("aaa", "# aaa\n") + wrapExpected("bbb", "# bbb\n")
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

	expected := wrapExpected("aaa", "# aaa\n") + wrapExpected("zzz", "# zzz\n")
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

	expected := wrapExpected("tool", "# tool\n")
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

	expected := wrapExpected("tool", "# bash\n")
	if string(content) != expected {
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

	expected := wrapExpected("tool", "# no newline")
	if string(content) != expected {
		t.Errorf("expected trailing newline added: %q, got %q", expected, string(content))
	}
}

// --- New security hardening tests ---

func TestRebuildShellCache_RejectsSymlinks(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a real file
	writeTestFile(t, filepath.Join(shellDDir, "aaa.bash"), "# real\n")

	// Create a symlink posing as a shell init file
	targetFile := filepath.Join(t.TempDir(), "malicious.sh")
	writeTestFile(t, targetFile, "# injected\n")
	if err := os.Symlink(targetFile, filepath.Join(shellDDir, "zzz.bash")); err != nil {
		t.Fatal(err)
	}

	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatalf("RebuildShellCache failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.bash"))
	if err != nil {
		t.Fatalf("expected cache file to exist: %v", err)
	}

	// Only the real file should be included, symlink should be excluded
	expected := wrapExpected("aaa", "# real\n")
	if string(content) != expected {
		t.Errorf("expected %q (symlink excluded), got %q", expected, string(content))
	}
}

func TestRebuildShellCache_HashVerification_ValidHash(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	fileContent := "# tool init\n"
	writeTestFile(t, filepath.Join(shellDDir, "tool.bash"), fileContent)

	hashes := map[string]string{
		"share/shell.d/tool.bash": testHash(fileContent),
	}

	if err := RebuildShellCache(tsukuHome, "bash", hashes); err != nil {
		t.Fatalf("RebuildShellCache failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.bash"))
	if err != nil {
		t.Fatalf("expected cache file to exist: %v", err)
	}

	expected := wrapExpected("tool", fileContent)
	if string(content) != expected {
		t.Errorf("expected %q, got %q", expected, string(content))
	}
}

func TestRebuildShellCache_HashVerification_MismatchExcludesFile(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write the original file
	writeTestFile(t, filepath.Join(shellDDir, "good.bash"), "# good\n")
	// Write a tampered file
	writeTestFile(t, filepath.Join(shellDDir, "tampered.bash"), "# tampered content\n")

	hashes := map[string]string{
		"share/shell.d/good.bash":     testHash("# good\n"),
		"share/shell.d/tampered.bash": testHash("# original content\n"), // Wrong hash
	}

	if err := RebuildShellCache(tsukuHome, "bash", hashes); err != nil {
		t.Fatalf("RebuildShellCache failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.bash"))
	if err != nil {
		t.Fatalf("expected cache file to exist: %v", err)
	}

	// Only the good file should be included
	expected := wrapExpected("good", "# good\n")
	if string(content) != expected {
		t.Errorf("expected %q (tampered file excluded), got %q", expected, string(content))
	}
}

func TestRebuildShellCache_HashVerification_AllMismatchRemovesCache(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "tool.bash"), "# tampered\n")

	hashes := map[string]string{
		"share/shell.d/tool.bash": testHash("# original\n"),
	}

	// Create a pre-existing cache
	cachePath := filepath.Join(shellDDir, ".init-cache.bash")
	writeTestFile(t, cachePath, "# old cache\n")

	if err := RebuildShellCache(tsukuHome, "bash", hashes); err != nil {
		t.Fatalf("RebuildShellCache failed: %v", err)
	}

	// Cache should be removed when all files fail verification
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Error("expected cache file to be removed when all files fail hash verification")
	}
}

func TestRebuildShellCache_LegacyTolerance_NoHashIncludesFile(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	// File with a known hash
	writeTestFile(t, filepath.Join(shellDDir, "new-tool.bash"), "# new\n")
	// File without a stored hash (legacy install)
	writeTestFile(t, filepath.Join(shellDDir, "legacy-tool.bash"), "# legacy\n")

	hashes := map[string]string{
		"share/shell.d/new-tool.bash": testHash("# new\n"),
		// No entry for legacy-tool.bash -- should be included without verification
	}

	if err := RebuildShellCache(tsukuHome, "bash", hashes); err != nil {
		t.Fatalf("RebuildShellCache failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.bash"))
	if err != nil {
		t.Fatalf("expected cache file to exist: %v", err)
	}

	// Both files should be included (legacy without hash, new with valid hash)
	expected := wrapExpected("legacy-tool", "# legacy\n") + wrapExpected("new-tool", "# new\n")
	if string(content) != expected {
		t.Errorf("expected %q (both files included), got %q", expected, string(content))
	}
}

func TestRebuildShellCache_LegacyTolerance_NilHashMapIncludesAll(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "tool.bash"), "# tool\n")

	// No hash map at all -- backward compatible
	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatalf("RebuildShellCache failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.bash"))
	if err != nil {
		t.Fatalf("expected cache file to exist: %v", err)
	}

	expected := wrapExpected("tool", "# tool\n")
	if string(content) != expected {
		t.Errorf("expected %q, got %q", expected, string(content))
	}
}

func TestRebuildShellCache_CacheFilePermissions(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "tool.bash"), "# tool\n")

	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatalf("RebuildShellCache failed: %v", err)
	}

	cachePath := filepath.Join(shellDDir, ".init-cache.bash")
	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatal(err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected cache file permissions 0600, got %04o", perm)
	}
}

func TestRebuildShellCache_LockFileCreated(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "tool.bash"), "# tool\n")

	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatalf("RebuildShellCache failed: %v", err)
	}

	lockPath := filepath.Join(shellDDir, ".lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("expected lock file to be created")
	}
}

func TestRebuildShellCache_ExcludesLockFile(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "tool.bash"), "# tool\n")
	// Pre-create a lock file that ends with the shell suffix (shouldn't happen
	// but test the exclusion logic)
	writeTestFile(t, filepath.Join(shellDDir, ".lock"), "")

	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatalf("RebuildShellCache failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.bash"))
	if err != nil {
		t.Fatal(err)
	}

	expected := wrapExpected("tool", "# tool\n")
	if string(content) != expected {
		t.Errorf("expected only tool content, got %q", string(content))
	}
}

func TestRebuildShellCache_ErrorIsolation_SubshellWrapping(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "starship.bash"), "eval \"$(starship init bash)\"\n")

	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatalf("RebuildShellCache failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.bash"))
	if err != nil {
		t.Fatal(err)
	}

	got := string(content)

	// Verify the wrapping structure
	if !strings.Contains(got, "# tsuku: starship") {
		t.Error("expected tool identifier comment")
	}
	if !strings.Contains(got, "{ # begin starship") {
		t.Error("expected brace open with tool name")
	}
	if !strings.Contains(got, "} 2>/dev/null || true") {
		t.Error("expected error-suppressing brace close")
	}
	if !strings.Contains(got, "eval \"$(starship init bash)\"") {
		t.Error("expected original content preserved inside wrapper")
	}
}

func TestRebuildShellCache_ErrorIsolation_MultipleTools(t *testing.T) {
	tsukuHome := t.TempDir()
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
	if err := os.MkdirAll(shellDDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(shellDDir, "alpha.bash"), "# alpha init\n")
	writeTestFile(t, filepath.Join(shellDDir, "beta.bash"), "# beta init\n")

	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatalf("RebuildShellCache failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(shellDDir, ".init-cache.bash"))
	if err != nil {
		t.Fatal(err)
	}

	got := string(content)

	// Each tool should have its own isolated block
	if !strings.Contains(got, "# tsuku: alpha") {
		t.Error("expected alpha tool identifier")
	}
	if !strings.Contains(got, "# tsuku: beta") {
		t.Error("expected beta tool identifier")
	}

	// Count brace wrappers -- should be exactly 2
	if strings.Count(got, "} 2>/dev/null || true") != 2 {
		t.Errorf("expected 2 error-isolation brace groups, got %d", strings.Count(got, "} 2>/dev/null || true"))
	}
}

func TestRebuildShellCache_DirectoryPermissions(t *testing.T) {
	tsukuHome := t.TempDir()

	// Don't pre-create the directory -- let RebuildShellCache create it
	shellDDir := filepath.Join(tsukuHome, "share", "shell.d")

	// RebuildShellCache will create the directory for the lock
	if err := RebuildShellCache(tsukuHome, "bash"); err != nil {
		t.Fatalf("RebuildShellCache failed: %v", err)
	}

	info, err := os.Stat(shellDDir)
	if err != nil {
		t.Fatal(err)
	}

	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("expected shell.d directory permissions 0700, got %04o", perm)
	}
}

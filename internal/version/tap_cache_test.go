package version

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTapCache_SetAndGet(t *testing.T) {
	dir := t.TempDir()
	cache := NewTapCache(dir, time.Hour)

	info := &tapFormulaInfo{
		Version: "1.7.0",
		RootURL: "https://github.com/hashicorp/homebrew-tap/releases/download/v1.7.0",
		Checksums: map[string]string{
			"arm64_sonoma": "abc123",
			"sonoma":       "def456",
		},
	}

	// Set the cache entry
	err := cache.Set("hashicorp/tap", "terraform", info)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Verify the file was created with correct path
	expectedPath := filepath.Join(dir, "hashicorp-tap", "terraform.json")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Cache file not created at expected path: %s", expectedPath)
	}

	// Get the cache entry
	got := cache.Get("hashicorp/tap", "terraform")
	if got == nil {
		t.Fatal("Get() returned nil, expected cached entry")
	}

	// Verify the retrieved data
	if got.Version != info.Version {
		t.Errorf("Version = %q, want %q", got.Version, info.Version)
	}
	if got.RootURL != info.RootURL {
		t.Errorf("RootURL = %q, want %q", got.RootURL, info.RootURL)
	}
	if len(got.Checksums) != len(info.Checksums) {
		t.Errorf("Checksums count = %d, want %d", len(got.Checksums), len(info.Checksums))
	}
	for k, v := range info.Checksums {
		if got.Checksums[k] != v {
			t.Errorf("Checksums[%s] = %q, want %q", k, got.Checksums[k], v)
		}
	}
}

func TestTapCache_CacheMiss(t *testing.T) {
	dir := t.TempDir()
	cache := NewTapCache(dir, time.Hour)

	// Try to get a non-existent entry
	got := cache.Get("nonexistent/tap", "formula")
	if got != nil {
		t.Error("Get() should return nil for cache miss")
	}
}

func TestTapCache_CacheExpiry(t *testing.T) {
	dir := t.TempDir()
	// Use a very short TTL for testing
	cache := NewTapCache(dir, 1*time.Millisecond)

	info := &tapFormulaInfo{
		Version: "1.0.0",
		RootURL: "https://example.com",
		Checksums: map[string]string{
			"sonoma": "abc123",
		},
	}

	err := cache.Set("test/tap", "formula", info)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Wait for expiry
	time.Sleep(5 * time.Millisecond)

	// Should return nil due to expiry
	got := cache.Get("test/tap", "formula")
	if got != nil {
		t.Error("Get() should return nil for expired cache entry")
	}
}

func TestTapCache_CorruptedFile(t *testing.T) {
	dir := t.TempDir()
	cache := NewTapCache(dir, time.Hour)

	// Create a corrupted cache file
	tapDir := filepath.Join(dir, "corrupt-tap")
	if err := os.MkdirAll(tapDir, 0755); err != nil {
		t.Fatalf("Failed to create tap directory: %v", err)
	}

	corruptPath := filepath.Join(tapDir, "formula.json")
	if err := os.WriteFile(corruptPath, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("Failed to write corrupted file: %v", err)
	}

	// Should return nil for corrupted file
	got := cache.Get("corrupt/tap", "formula")
	if got != nil {
		t.Error("Get() should return nil for corrupted cache file")
	}

	// The corrupted file should be cleaned up
	if _, err := os.Stat(corruptPath); !os.IsNotExist(err) {
		t.Error("Corrupted cache file should be removed")
	}
}

func TestTapCache_Invalidate(t *testing.T) {
	dir := t.TempDir()
	cache := NewTapCache(dir, time.Hour)

	info := &tapFormulaInfo{
		Version: "1.0.0",
		RootURL: "https://example.com",
		Checksums: map[string]string{
			"sonoma": "abc123",
		},
	}

	err := cache.Set("test/tap", "formula", info)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Verify it exists
	if got := cache.Get("test/tap", "formula"); got == nil {
		t.Fatal("Entry should exist before invalidation")
	}

	// Invalidate
	err = cache.Invalidate("test/tap", "formula")
	if err != nil {
		t.Fatalf("Invalidate() error = %v", err)
	}

	// Should no longer exist
	if got := cache.Get("test/tap", "formula"); got != nil {
		t.Error("Entry should not exist after invalidation")
	}
}

func TestTapCache_InvalidateNonexistent(t *testing.T) {
	dir := t.TempDir()
	cache := NewTapCache(dir, time.Hour)

	// Should not error when invalidating non-existent entry
	err := cache.Invalidate("nonexistent/tap", "formula")
	if err != nil {
		t.Errorf("Invalidate() error = %v, expected nil", err)
	}
}

func TestTapCache_Clear(t *testing.T) {
	dir := t.TempDir()
	cache := NewTapCache(dir, time.Hour)

	info := &tapFormulaInfo{
		Version: "1.0.0",
		RootURL: "https://example.com",
		Checksums: map[string]string{
			"sonoma": "abc123",
		},
	}

	// Add multiple entries
	_ = cache.Set("tap1/repo", "formula1", info)
	_ = cache.Set("tap2/repo", "formula2", info)

	// Clear all
	err := cache.Clear()
	if err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	// Verify directory is gone
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("Cache directory should be removed after Clear()")
	}
}

func TestTapCache_CacheFileFormat(t *testing.T) {
	dir := t.TempDir()
	cache := NewTapCache(dir, time.Hour)

	info := &tapFormulaInfo{
		Version: "1.7.0",
		RootURL: "https://example.com/releases",
		Checksums: map[string]string{
			"arm64_sonoma": "abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
		},
	}

	err := cache.Set("hashicorp/tap", "terraform", info)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Read and verify file format
	path := filepath.Join(dir, "hashicorp-tap", "terraform.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read cache file: %v", err)
	}

	var entry struct {
		CachedAt  string `json:"cached_at"`
		ExpiresAt string `json:"expires_at"`
		Info      struct {
			Version   string            `json:"version"`
			RootURL   string            `json:"root_url"`
			Checksums map[string]string `json:"checksums"`
		} `json:"info"`
	}

	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("Failed to unmarshal cache file: %v", err)
	}

	// Verify required fields exist
	if entry.CachedAt == "" {
		t.Error("Cache file missing 'cached_at' field")
	}
	if entry.ExpiresAt == "" {
		t.Error("Cache file missing 'expires_at' field")
	}
	if entry.Info.Version != "1.7.0" {
		t.Errorf("Version = %q, want %q", entry.Info.Version, "1.7.0")
	}
}

func TestTapCache_DirectoryCreation(t *testing.T) {
	dir := t.TempDir()
	// Use a nested path that doesn't exist
	nestedDir := filepath.Join(dir, "nested", "cache", "taps")
	cache := NewTapCache(nestedDir, time.Hour)

	info := &tapFormulaInfo{
		Version: "1.0.0",
		RootURL: "https://example.com",
		Checksums: map[string]string{
			"sonoma": "abc123",
		},
	}

	err := cache.Set("test/tap", "formula", info)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Verify the nested directory structure was created
	expectedPath := filepath.Join(nestedDir, "test-tap", "formula.json")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Cache file not created at expected path: %s", expectedPath)
	}
}

package version

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTapCache_GetSet(t *testing.T) {
	// Create temp directory for cache
	tmpDir := t.TempDir()
	cache := NewTapCache(tmpDir, time.Hour)

	tap := "hashicorp/tap"
	formula := "terraform"

	// Test cache miss (empty cache)
	entry := cache.Get(tap, formula)
	if entry != nil {
		t.Error("expected nil for cache miss")
	}

	// Create test VersionInfo
	info := &VersionInfo{
		Tag:     "1.7.0",
		Version: "1.7.0",
		Metadata: map[string]string{
			"formula":    "terraform",
			"bottle_url": "https://example.com/terraform--1.7.0.bottle.tar.gz",
			"checksum":   "sha256:abc123",
			"tap":        "hashicorp/tap",
		},
	}

	// Test set
	err := cache.Set(tap, formula, info)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Verify cache file was created
	cachePath := filepath.Join(tmpDir, "hashicorp", "tap", "terraform.json")
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Errorf("cache file not created at %s", cachePath)
	}

	// Test cache hit
	entry = cache.Get(tap, formula)
	if entry == nil {
		t.Fatal("expected cache hit, got nil")
	}

	if entry.Version != "1.7.0" {
		t.Errorf("Version = %q, want %q", entry.Version, "1.7.0")
	}

	if entry.Formula != "terraform" {
		t.Errorf("Formula = %q, want %q", entry.Formula, "terraform")
	}

	if entry.BottleURL != "https://example.com/terraform--1.7.0.bottle.tar.gz" {
		t.Errorf("BottleURL = %q, want correct URL", entry.BottleURL)
	}

	if entry.Checksum != "sha256:abc123" {
		t.Errorf("Checksum = %q, want %q", entry.Checksum, "sha256:abc123")
	}

	if entry.Tap != "hashicorp/tap" {
		t.Errorf("Tap = %q, want %q", entry.Tap, "hashicorp/tap")
	}
}

func TestTapCache_Expiry(t *testing.T) {
	tmpDir := t.TempDir()
	// Use very short TTL for testing
	cache := NewTapCache(tmpDir, 50*time.Millisecond)

	tap := "test/tap"
	formula := "test-formula"

	info := &VersionInfo{
		Tag:     "1.0.0",
		Version: "1.0.0",
		Metadata: map[string]string{
			"formula":    "test-formula",
			"bottle_url": "https://example.com/test.tar.gz",
			"checksum":   "sha256:def456",
			"tap":        "test/tap",
		},
	}

	// Set cache entry
	err := cache.Set(tap, formula, info)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Verify cache hit
	entry := cache.Get(tap, formula)
	if entry == nil {
		t.Fatal("expected cache hit immediately after set")
	}

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Verify cache miss due to expiry
	entry = cache.Get(tap, formula)
	if entry != nil {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestTapCache_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	cache := NewTapCache(tmpDir, time.Hour)

	tap := "test/tap"
	formula := "corrupted"

	// Create corrupted cache file
	cachePath := filepath.Join(tmpDir, "test", "tap", "corrupted.json")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte("not valid json{{{"), 0644); err != nil {
		t.Fatalf("failed to write corrupted file: %v", err)
	}

	// Get should return nil for corrupted file (treated as cache miss)
	entry := cache.Get(tap, formula)
	if entry != nil {
		t.Error("expected nil for corrupted cache file")
	}
}

func TestTapCache_VersionCheck(t *testing.T) {
	tmpDir := t.TempDir()
	cache := NewTapCache(tmpDir, time.Hour)

	tap := "hashicorp/tap"
	formula := "terraform"

	info := &VersionInfo{
		Tag:     "1.7.0",
		Version: "1.7.0",
		Metadata: map[string]string{
			"formula":    "terraform",
			"bottle_url": "https://example.com/terraform--1.7.0.bottle.tar.gz",
			"checksum":   "sha256:abc123",
			"tap":        "hashicorp/tap",
		},
	}

	err := cache.Set(tap, formula, info)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Test with matching version
	entry := cache.GetWithVersionCheck(tap, formula, "1.7.0")
	if entry == nil {
		t.Error("expected cache hit for matching version")
	}

	// Test with empty version (should match any)
	entry = cache.GetWithVersionCheck(tap, formula, "")
	if entry == nil {
		t.Error("expected cache hit for empty version constraint")
	}

	// Test with different version (should miss)
	entry = cache.GetWithVersionCheck(tap, formula, "1.6.0")
	if entry != nil {
		t.Error("expected cache miss for version mismatch")
	}
}

func TestTapCache_ToVersionInfo(t *testing.T) {
	entry := &TapCacheEntry{
		Version:   "1.7.0",
		Formula:   "terraform",
		BottleURL: "https://example.com/terraform.tar.gz",
		Checksum:  "sha256:abc123",
		Tap:       "hashicorp/tap",
		Extra: map[string]string{
			"custom_key": "custom_value",
		},
	}

	info := entry.ToVersionInfo()

	if info.Version != "1.7.0" {
		t.Errorf("Version = %q, want %q", info.Version, "1.7.0")
	}

	if info.Tag != "1.7.0" {
		t.Errorf("Tag = %q, want %q", info.Tag, "1.7.0")
	}

	if info.Metadata["formula"] != "terraform" {
		t.Errorf("formula metadata = %q, want %q", info.Metadata["formula"], "terraform")
	}

	if info.Metadata["custom_key"] != "custom_value" {
		t.Errorf("custom_key metadata = %q, want %q", info.Metadata["custom_key"], "custom_value")
	}
}

func TestTapCache_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	cache := NewTapCache(tmpDir, time.Hour)

	// Add some entries
	taps := []struct {
		tap     string
		formula string
	}{
		{"hashicorp/tap", "terraform"},
		{"hashicorp/tap", "vault"},
		{"github/gh", "gh"},
	}

	for _, tc := range taps {
		info := &VersionInfo{
			Tag:     "1.0.0",
			Version: "1.0.0",
			Metadata: map[string]string{
				"formula":    tc.formula,
				"bottle_url": "https://example.com/" + tc.formula + ".tar.gz",
				"checksum":   "sha256:abc123",
				"tap":        tc.tap,
			},
		}
		if err := cache.Set(tc.tap, tc.formula, info); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	// Verify entries exist
	for _, tc := range taps {
		if entry := cache.Get(tc.tap, tc.formula); entry == nil {
			t.Errorf("expected entry for %s/%s before clear", tc.tap, tc.formula)
		}
	}

	// Clear cache
	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	// Verify entries are gone
	for _, tc := range taps {
		if entry := cache.Get(tc.tap, tc.formula); entry != nil {
			t.Errorf("expected nil for %s/%s after clear", tc.tap, tc.formula)
		}
	}
}

func TestTapCache_Info(t *testing.T) {
	tmpDir := t.TempDir()
	cache := NewTapCache(tmpDir, time.Hour)

	// Check empty cache
	info, err := cache.Info()
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.EntryCount != 0 {
		t.Errorf("EntryCount = %d, want 0", info.EntryCount)
	}

	// Add entries
	taps := []struct {
		tap     string
		formula string
	}{
		{"hashicorp/tap", "terraform"},
		{"hashicorp/tap", "vault"},
	}

	for _, tc := range taps {
		vinfo := &VersionInfo{
			Tag:     "1.0.0",
			Version: "1.0.0",
			Metadata: map[string]string{
				"formula":    tc.formula,
				"bottle_url": "https://example.com/" + tc.formula + ".tar.gz",
				"checksum":   "sha256:abc123",
				"tap":        tc.tap,
			},
		}
		if err := cache.Set(tc.tap, tc.formula, vinfo); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	// Check cache info
	info, err = cache.Info()
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.EntryCount != 2 {
		t.Errorf("EntryCount = %d, want 2", info.EntryCount)
	}
	if info.TotalSize == 0 {
		t.Error("TotalSize should be > 0")
	}
}

func TestTapCache_PathSanitization(t *testing.T) {
	tmpDir := t.TempDir()
	cache := NewTapCache(tmpDir, time.Hour)

	// Test with potentially problematic characters
	testCases := []struct {
		tap     string
		formula string
	}{
		{"owner/repo", "normal-formula"},
		{"owner-with-dash/repo", "formula_underscore"},
	}

	for _, tc := range testCases {
		info := &VersionInfo{
			Tag:     "1.0.0",
			Version: "1.0.0",
			Metadata: map[string]string{
				"formula":    tc.formula,
				"bottle_url": "https://example.com/test.tar.gz",
				"checksum":   "sha256:abc123",
				"tap":        tc.tap,
			},
		}

		err := cache.Set(tc.tap, tc.formula, info)
		if err != nil {
			t.Errorf("Set(%q, %q) error = %v", tc.tap, tc.formula, err)
			continue
		}

		entry := cache.Get(tc.tap, tc.formula)
		if entry == nil {
			t.Errorf("Get(%q, %q) returned nil after Set", tc.tap, tc.formula)
		}
	}
}

func TestTapCache_NilVersionInfo(t *testing.T) {
	tmpDir := t.TempDir()
	cache := NewTapCache(tmpDir, time.Hour)

	err := cache.Set("test/tap", "formula", nil)
	if err == nil {
		t.Error("expected error when setting nil VersionInfo")
	}
}

package actions

import (
	"os"
	"path/filepath"
	"testing"
)

// createSecureCacheDir creates a cache directory with proper 0700 permissions for testing
func createSecureCacheDir(t *testing.T) string {
	t.Helper()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatalf("failed to create secure cache dir: %v", err)
	}
	return cacheDir
}

func TestDownloadCache_CacheMiss(t *testing.T) {
	t.Parallel()
	cacheDir := createSecureCacheDir(t)
	cache := NewDownloadCache(cacheDir)

	destPath := filepath.Join(t.TempDir(), "output.txt")
	found, err := cache.Check("https://example.com/file.tar.gz", destPath, "", "")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if found {
		t.Error("expected cache miss, got hit")
	}
}

func TestDownloadCache_SaveAndCheck(t *testing.T) {
	t.Parallel()
	cacheDir := createSecureCacheDir(t)
	cache := NewDownloadCache(cacheDir)

	// Create a source file to cache
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "source.txt")
	content := []byte("test content for caching")
	if err := os.WriteFile(sourcePath, content, 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	url := "https://example.com/file.tar.gz"

	// Save to cache
	if err := cache.Save(url, sourcePath, ""); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Check cache - should hit
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "output.txt")
	found, err := cache.Check(url, destPath, "", "")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !found {
		t.Error("expected cache hit, got miss")
	}

	// Verify content was copied
	gotContent, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read dest file: %v", err)
	}
	if string(gotContent) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", gotContent, content)
	}
}

func TestDownloadCache_ChecksumVerification(t *testing.T) {
	t.Parallel()
	cacheDir := createSecureCacheDir(t)
	cache := NewDownloadCache(cacheDir)

	// Create source file
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "source.txt")
	content := []byte("test content")
	if err := os.WriteFile(sourcePath, content, 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	url := "https://example.com/file.tar.gz"

	// Compute correct checksum
	correctChecksum, err := computeSHA256(sourcePath)
	if err != nil {
		t.Fatalf("failed to compute checksum: %v", err)
	}

	// Save to cache
	if err := cache.Save(url, sourcePath, correctChecksum); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Check with correct checksum - should hit
	destPath := filepath.Join(t.TempDir(), "output.txt")
	found, err := cache.Check(url, destPath, correctChecksum, "sha256")
	if err != nil {
		t.Fatalf("Check() with correct checksum error = %v", err)
	}
	if !found {
		t.Error("expected cache hit with correct checksum")
	}

	// Check with wrong checksum - should miss (invalidates cache)
	destPath2 := filepath.Join(t.TempDir(), "output2.txt")
	found, err = cache.Check(url, destPath2, "wrongchecksum", "sha256")
	if err != nil {
		t.Fatalf("Check() with wrong checksum error = %v", err)
	}
	if found {
		t.Error("expected cache miss with wrong checksum")
	}

	// Subsequent check should also miss (cache was invalidated)
	destPath3 := filepath.Join(t.TempDir(), "output3.txt")
	found, err = cache.Check(url, destPath3, "", "")
	if err != nil {
		t.Fatalf("Check() after invalidation error = %v", err)
	}
	if found {
		t.Error("expected cache miss after invalidation")
	}
}

func TestDownloadCache_Clear(t *testing.T) {
	t.Parallel()
	cacheDir := createSecureCacheDir(t)
	cache := NewDownloadCache(cacheDir)

	// Create source file and cache it
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "source.txt")
	if err := os.WriteFile(sourcePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	url1 := "https://example.com/file1.tar.gz"
	url2 := "https://example.com/file2.tar.gz"

	if err := cache.Save(url1, sourcePath, ""); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := cache.Save(url2, sourcePath, ""); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify both are cached
	info, err := cache.Info()
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.EntryCount != 2 {
		t.Errorf("expected 2 entries before clear, got %d", info.EntryCount)
	}

	// Clear cache
	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	// Verify cache is empty
	info, err = cache.Info()
	if err != nil {
		t.Fatalf("Info() after clear error = %v", err)
	}
	if info.EntryCount != 0 {
		t.Errorf("expected 0 entries after clear, got %d", info.EntryCount)
	}
}

func TestDownloadCache_Info(t *testing.T) {
	t.Parallel()
	cacheDir := createSecureCacheDir(t)
	cache := NewDownloadCache(cacheDir)

	// Empty cache
	info, err := cache.Info()
	if err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if info.EntryCount != 0 {
		t.Errorf("expected 0 entries, got %d", info.EntryCount)
	}
	if info.TotalSize != 0 {
		t.Errorf("expected 0 size, got %d", info.TotalSize)
	}

	// Add an entry
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "source.txt")
	content := []byte("test content for size calculation")
	if err := os.WriteFile(sourcePath, content, 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	if err := cache.Save("https://example.com/file.tar.gz", sourcePath, ""); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	info, err = cache.Info()
	if err != nil {
		t.Fatalf("Info() after save error = %v", err)
	}
	if info.EntryCount != 1 {
		t.Errorf("expected 1 entry, got %d", info.EntryCount)
	}
	if info.TotalSize != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), info.TotalSize)
	}
}

func TestDownloadCache_DifferentURLsDifferentCache(t *testing.T) {
	t.Parallel()
	cacheDir := createSecureCacheDir(t)
	cache := NewDownloadCache(cacheDir)

	// Create two source files with different content
	sourceDir := t.TempDir()
	source1 := filepath.Join(sourceDir, "source1.txt")
	source2 := filepath.Join(sourceDir, "source2.txt")
	content1 := []byte("content for file 1")
	content2 := []byte("content for file 2")

	if err := os.WriteFile(source1, content1, 0644); err != nil {
		t.Fatalf("failed to create source1: %v", err)
	}
	if err := os.WriteFile(source2, content2, 0644); err != nil {
		t.Fatalf("failed to create source2: %v", err)
	}

	url1 := "https://example.com/file1.tar.gz"
	url2 := "https://example.com/file2.tar.gz"

	// Cache both
	if err := cache.Save(url1, source1, ""); err != nil {
		t.Fatalf("Save() url1 error = %v", err)
	}
	if err := cache.Save(url2, source2, ""); err != nil {
		t.Fatalf("Save() url2 error = %v", err)
	}

	// Retrieve and verify each returns correct content
	dest1 := filepath.Join(t.TempDir(), "out1.txt")
	dest2 := filepath.Join(t.TempDir(), "out2.txt")

	found1, _ := cache.Check(url1, dest1, "", "")
	found2, _ := cache.Check(url2, dest2, "", "")

	if !found1 || !found2 {
		t.Fatal("expected both cache hits")
	}

	got1, _ := os.ReadFile(dest1)
	got2, _ := os.ReadFile(dest2)

	if string(got1) != string(content1) {
		t.Errorf("url1 content mismatch: got %q, want %q", got1, content1)
	}
	if string(got2) != string(content2) {
		t.Errorf("url2 content mismatch: got %q, want %q", got2, content2)
	}
}

func TestDownloadCache_CorruptedFile(t *testing.T) {
	t.Parallel()
	cacheDir := createSecureCacheDir(t)
	cache := NewDownloadCache(cacheDir)

	// Create and cache a file
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "source.txt")
	content := []byte("original content")
	if err := os.WriteFile(sourcePath, content, 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	url := "https://example.com/file.tar.gz"
	if err := cache.Save(url, sourcePath, ""); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Corrupt the cached file by changing its size
	filePath, _ := cache.cachePaths(url)
	if err := os.WriteFile(filePath, []byte("corrupted"), 0644); err != nil {
		t.Fatalf("failed to corrupt cache file: %v", err)
	}

	// Check should detect corruption and return miss
	destPath := filepath.Join(t.TempDir(), "output.txt")
	found, err := cache.Check(url, destPath, "", "")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if found {
		t.Error("expected cache miss due to corruption")
	}
}

func TestDownloadCache_ClearEmpty(t *testing.T) {
	t.Parallel()
	cacheDir := createSecureCacheDir(t)
	cache := NewDownloadCache(cacheDir)

	// Clear on empty cache should not error
	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear() on empty cache error = %v", err)
	}
}

func TestDownloadCache_ClearNonexistentDir(t *testing.T) {
	t.Parallel()
	cache := NewDownloadCache("/nonexistent/path/cache")

	// Clear on nonexistent directory should not error
	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear() on nonexistent dir error = %v", err)
	}
}

func TestContainsSymlink(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		wantSymlink bool
		wantErr     bool
	}{
		{
			name: "regular directory",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			wantSymlink: false,
			wantErr:     false,
		},
		{
			name: "symlink to directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				targetDir := filepath.Join(tmpDir, "target")
				if err := os.MkdirAll(targetDir, 0755); err != nil {
					t.Fatalf("failed to create target dir: %v", err)
				}
				symlinkDir := filepath.Join(tmpDir, "symlink")
				if err := os.Symlink(targetDir, symlinkDir); err != nil {
					t.Fatalf("failed to create symlink: %v", err)
				}
				return symlinkDir
			},
			wantSymlink: true,
			wantErr:     false,
		},
		{
			name: "nested path with symlink parent",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				targetDir := filepath.Join(tmpDir, "target")
				if err := os.MkdirAll(targetDir, 0755); err != nil {
					t.Fatalf("failed to create target dir: %v", err)
				}
				symlinkDir := filepath.Join(tmpDir, "symlink")
				if err := os.Symlink(targetDir, symlinkDir); err != nil {
					t.Fatalf("failed to create symlink: %v", err)
				}
				// Return a nested path under the symlink
				return filepath.Join(symlinkDir, "nested", "path")
			},
			wantSymlink: true,
			wantErr:     false,
		},
		{
			name: "nonexistent path (no symlink)",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "does", "not", "exist")
			},
			wantSymlink: false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			gotSymlink, err := containsSymlink(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("containsSymlink() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotSymlink != tt.wantSymlink {
				t.Errorf("containsSymlink() = %v, want %v", gotSymlink, tt.wantSymlink)
			}
		})
	}
}

func TestValidateCacheDirPermissions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr bool
		errMsg  string
	}{
		{
			name: "nonexistent directory (ok)",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "does-not-exist")
			},
			wantErr: false,
		},
		{
			name: "directory with 0700 permissions (ok)",
			setup: func(t *testing.T) string {
				dir := filepath.Join(t.TempDir(), "secure")
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				return dir
			},
			wantErr: false,
		},
		{
			name: "directory with 0755 permissions (insecure)",
			setup: func(t *testing.T) string {
				dir := filepath.Join(t.TempDir(), "insecure")
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				return dir
			},
			wantErr: true,
			errMsg:  "insecure permissions",
		},
		{
			name: "directory with 0750 permissions (insecure)",
			setup: func(t *testing.T) string {
				dir := filepath.Join(t.TempDir(), "group-readable")
				if err := os.MkdirAll(dir, 0750); err != nil {
					t.Fatalf("failed to create dir: %v", err)
				}
				return dir
			},
			wantErr: true,
			errMsg:  "insecure permissions",
		},
		{
			name: "symlink instead of directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				targetDir := filepath.Join(tmpDir, "target")
				if err := os.MkdirAll(targetDir, 0700); err != nil {
					t.Fatalf("failed to create target dir: %v", err)
				}
				symlinkDir := filepath.Join(tmpDir, "symlink")
				if err := os.Symlink(targetDir, symlinkDir); err != nil {
					t.Fatalf("failed to create symlink: %v", err)
				}
				return symlinkDir
			},
			wantErr: true,
			errMsg:  "is a symlink",
		},
		{
			name: "file instead of directory",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "file")
				if err := os.WriteFile(filePath, []byte("test"), 0700); err != nil {
					t.Fatalf("failed to create file: %v", err)
				}
				return filePath
			},
			wantErr: true,
			errMsg:  "not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			err := validateCacheDirPermissions(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCacheDirPermissions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !containsString(err.Error(), tt.errMsg) {
					t.Errorf("validateCacheDirPermissions() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestDownloadCache_SaveRejectsSymlink(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a target directory and a symlink to it
	targetDir := filepath.Join(tmpDir, "target")
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}
	symlinkDir := filepath.Join(tmpDir, "symlink")
	if err := os.Symlink(targetDir, symlinkDir); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Create a source file
	sourcePath := filepath.Join(tmpDir, "source.txt")
	if err := os.WriteFile(sourcePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// Try to save to cache with symlink path
	cache := NewDownloadCache(symlinkDir)
	err := cache.Save("https://example.com/file.tar.gz", sourcePath, "")
	if err == nil {
		t.Error("expected Save() to reject symlink cache path")
	}
	if err != nil && !containsString(err.Error(), "symlink") {
		t.Errorf("expected symlink error, got: %v", err)
	}
}

func TestDownloadCache_CheckRejectsSymlink(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a target directory and a symlink to it
	targetDir := filepath.Join(tmpDir, "target")
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}
	symlinkDir := filepath.Join(tmpDir, "symlink")
	if err := os.Symlink(targetDir, symlinkDir); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Try to check cache with symlink path
	cache := NewDownloadCache(symlinkDir)
	destPath := filepath.Join(tmpDir, "output.txt")
	_, err := cache.Check("https://example.com/file.tar.gz", destPath, "", "")
	if err == nil {
		t.Error("expected Check() to reject symlink cache path")
	}
	if err != nil && !containsString(err.Error(), "symlink") {
		t.Errorf("expected symlink error, got: %v", err)
	}
}

func TestDownloadCache_SaveRejectsInsecurePermissions(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create cache directory with insecure permissions
	cacheDir := filepath.Join(tmpDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	// Create a source file
	sourcePath := filepath.Join(tmpDir, "source.txt")
	if err := os.WriteFile(sourcePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// Try to save to cache with insecure permissions
	cache := NewDownloadCache(cacheDir)
	err := cache.Save("https://example.com/file.tar.gz", sourcePath, "")
	if err == nil {
		t.Error("expected Save() to reject insecure permissions")
	}
	if err != nil && !containsString(err.Error(), "insecure permissions") {
		t.Errorf("expected insecure permissions error, got: %v", err)
	}
}

func TestDownloadCache_CheckRejectsInsecurePermissions(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create cache directory with insecure permissions
	cacheDir := filepath.Join(tmpDir, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	// Try to check cache with insecure permissions
	cache := NewDownloadCache(cacheDir)
	destPath := filepath.Join(tmpDir, "output.txt")
	_, err := cache.Check("https://example.com/file.tar.gz", destPath, "", "")
	if err == nil {
		t.Error("expected Check() to reject insecure permissions")
	}
	if err != nil && !containsString(err.Error(), "insecure permissions") {
		t.Errorf("expected insecure permissions error, got: %v", err)
	}
}

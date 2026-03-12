package verify

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBatchError_Error(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		err := &BatchError{
			Batch:     []string{"/lib/a.so", "/lib/b.so"},
			IsTimeout: true,
		}
		msg := err.Error()
		if msg == "" {
			t.Error("expected non-empty error message")
		}
		if !containsStr(msg, "timed out") {
			t.Errorf("expected 'timed out' in message, got: %s", msg)
		}
	})

	t.Run("non-timeout", func(t *testing.T) {
		cause := &testErr{msg: "segfault"}
		err := &BatchError{
			Batch: []string{"/lib/a.so"},
			Cause: cause,
		}
		msg := err.Error()
		if !containsStr(msg, "failed") {
			t.Errorf("expected 'failed' in message, got: %s", msg)
		}
	})
}

func TestSplitIntoBatches(t *testing.T) {
	tests := []struct {
		name      string
		paths     []string
		batchSize int
		wantCount int
	}{
		{"empty", nil, 50, 0},
		{"single batch", []string{"a", "b", "c"}, 50, 1},
		{"exact split", []string{"a", "b", "c", "d"}, 2, 2},
		{"uneven split", []string{"a", "b", "c"}, 2, 2},
		{"one per batch", []string{"a", "b", "c"}, 1, 3},
		{"zero batch size uses default", []string{"a", "b"}, 0, 1},
		{"negative batch size uses default", []string{"a", "b"}, -1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batches := splitIntoBatches(tt.paths, tt.batchSize)
			if len(batches) != tt.wantCount {
				t.Errorf("got %d batches, want %d", len(batches), tt.wantCount)
			}
			// Verify all paths are present
			var all []string
			for _, b := range batches {
				all = append(all, b...)
			}
			if len(all) != len(tt.paths) {
				t.Errorf("total paths = %d, want %d", len(all), len(tt.paths))
			}
		})
	}
}

func TestSanitizeEnvForHelper(t *testing.T) {
	// Set a dangerous variable
	if err := os.Setenv("LD_PRELOAD", "/evil/lib.so"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("LD_PRELOAD") }()

	env := sanitizeEnvForHelper("/home/user/.tsuku")

	// Verify LD_PRELOAD is stripped
	for _, e := range env {
		if len(e) > 10 && e[:11] == "LD_PRELOAD=" {
			t.Error("LD_PRELOAD should be stripped from env")
		}
	}

	// Verify LD_LIBRARY_PATH is set with tsuku libs
	foundLDPath := false
	for _, e := range env {
		if len(e) > 16 && e[:16] == "LD_LIBRARY_PATH=" {
			foundLDPath = true
			if !containsStr(e, "/home/user/.tsuku/libs") {
				t.Errorf("LD_LIBRARY_PATH should contain tsuku libs dir, got: %s", e)
			}
		}
	}
	if !foundLDPath {
		t.Error("expected LD_LIBRARY_PATH in env")
	}
}

func TestValidateLibraryPaths(t *testing.T) {
	tmpDir := t.TempDir()
	libsDir := filepath.Join(tmpDir, "libs")
	if err := os.MkdirAll(libsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a valid library file
	validLib := filepath.Join(libsDir, "libtest.so")
	if err := os.WriteFile(validLib, []byte("lib"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("valid path", func(t *testing.T) {
		err := validateLibraryPaths([]string{validLib}, libsDir)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("path outside libs", func(t *testing.T) {
		outsidePath := filepath.Join(tmpDir, "outside.so")
		if err := os.WriteFile(outsidePath, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
		err := validateLibraryPaths([]string{outsidePath}, libsDir)
		if err == nil {
			t.Error("expected error for path outside libs dir")
		}
	})

	t.Run("nonexistent path", func(t *testing.T) {
		err := validateLibraryPaths([]string{filepath.Join(libsDir, "nonexistent.so")}, libsDir)
		if err == nil {
			t.Error("expected error for nonexistent path")
		}
	})

	t.Run("nonexistent libs dir", func(t *testing.T) {
		err := validateLibraryPaths([]string{validLib}, "/nonexistent/libs")
		if err == nil {
			t.Error("expected error for nonexistent libs dir")
		}
	})
}

func TestRunDlopenVerification_SkipDlopen(t *testing.T) {
	result, err := RunDlopenVerification(context.Background(), nil, []string{"/lib/test.so"}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Skipped {
		t.Error("expected Skipped=true when skipDlopen is set")
	}
	if result.Warning != "" {
		t.Errorf("expected empty warning for user skip, got: %s", result.Warning)
	}
}

// containsStr checks if s contains substr.
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// testErr is a simple error type for testing.
type testErr struct {
	msg string
}

func (e *testErr) Error() string {
	return e.msg
}

package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicSymlink_CreateNew(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")

	// Create target file
	if err := os.WriteFile(target, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}

	// Create symlink
	if err := AtomicSymlink(target, link); err != nil {
		t.Fatalf("AtomicSymlink() error = %v", err)
	}

	// Verify symlink exists and points to target
	linkTarget, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}
	if linkTarget != target {
		t.Errorf("symlink target = %q, want %q", linkTarget, target)
	}
}

func TestAtomicSymlink_ReplaceExisting(t *testing.T) {
	dir := t.TempDir()
	target1 := filepath.Join(dir, "target1")
	target2 := filepath.Join(dir, "target2")
	link := filepath.Join(dir, "link")

	// Create target files
	if err := os.WriteFile(target1, []byte("content1"), 0644); err != nil {
		t.Fatalf("failed to create target1: %v", err)
	}
	if err := os.WriteFile(target2, []byte("content2"), 0644); err != nil {
		t.Fatalf("failed to create target2: %v", err)
	}

	// Create initial symlink
	if err := AtomicSymlink(target1, link); err != nil {
		t.Fatalf("initial AtomicSymlink() error = %v", err)
	}

	// Replace with new target
	if err := AtomicSymlink(target2, link); err != nil {
		t.Fatalf("replacement AtomicSymlink() error = %v", err)
	}

	// Verify symlink now points to target2
	linkTarget, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}
	if linkTarget != target2 {
		t.Errorf("symlink target = %q, want %q", linkTarget, target2)
	}
}

func TestAtomicSymlink_ReplaceRegularFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")

	// Create target file
	if err := os.WriteFile(target, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}

	// Create a regular file where the symlink will be
	if err := os.WriteFile(link, []byte("regular file"), 0644); err != nil {
		t.Fatalf("failed to create regular file: %v", err)
	}

	// AtomicSymlink should replace the regular file
	if err := AtomicSymlink(target, link); err != nil {
		t.Fatalf("AtomicSymlink() error = %v", err)
	}

	// Verify it's now a symlink
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("failed to stat link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink, got regular file")
	}
}

func TestAtomicSymlink_RelativeTarget(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	target := filepath.Join(subdir, "target")
	link := filepath.Join(dir, "link")

	// Create target file
	if err := os.WriteFile(target, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}

	// Create symlink with relative target
	relTarget := filepath.Join("subdir", "target")
	if err := AtomicSymlink(relTarget, link); err != nil {
		t.Fatalf("AtomicSymlink() error = %v", err)
	}

	// Verify symlink has relative target
	linkTarget, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}
	if linkTarget != relTarget {
		t.Errorf("symlink target = %q, want %q", linkTarget, relTarget)
	}
}

func TestAtomicSymlink_TempCleanup(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")

	// Create target file
	if err := os.WriteFile(target, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}

	// Create symlink
	if err := AtomicSymlink(target, link); err != nil {
		t.Fatalf("AtomicSymlink() error = %v", err)
	}

	// Verify no temp file exists
	tempPath := filepath.Join(dir, ".link.tmp")
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Error("temporary symlink should be cleaned up")
	}
}

func TestValidateSymlinkTarget_Valid(t *testing.T) {
	dir := t.TempDir()
	toolsDir := filepath.Join(dir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("failed to create tools dir: %v", err)
	}

	validTargets := []string{
		filepath.Join(toolsDir, "kubectl-1.29.0", "bin", "kubectl"),
		filepath.Join(toolsDir, "java-21.0.1", "bin", "java"),
		filepath.Join(toolsDir, "tool"),
	}

	for _, target := range validTargets {
		t.Run(target, func(t *testing.T) {
			err := ValidateSymlinkTarget(target, toolsDir)
			if err != nil {
				t.Errorf("ValidateSymlinkTarget(%q, %q) = %v, want nil", target, toolsDir, err)
			}
		})
	}
}

func TestValidateSymlinkTarget_Invalid(t *testing.T) {
	dir := t.TempDir()
	toolsDir := filepath.Join(dir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("failed to create tools dir: %v", err)
	}

	tests := []struct {
		name   string
		target string
	}{
		{
			name:   "path traversal",
			target: filepath.Join(toolsDir, "..", "etc", "passwd"),
		},
		{
			name:   "outside tools directory",
			target: filepath.Join(dir, "other", "binary"),
		},
		{
			name:   "absolute path outside",
			target: "/usr/bin/ls",
		},
		{
			name:   "partial directory name match",
			target: filepath.Join(dir, "tools-malicious", "binary"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSymlinkTarget(tt.target, toolsDir)
			if err == nil {
				t.Errorf("ValidateSymlinkTarget(%q, %q) = nil, want error", tt.target, toolsDir)
			}
		})
	}
}

func TestValidateSymlinkTarget_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	toolsDir := filepath.Join(dir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatalf("failed to create tools dir: %v", err)
	}

	// This is a path traversal attempt that tries to escape
	traversalTarget := filepath.Join(toolsDir, "kubectl-1.29.0", "..", "..", "etc", "passwd")

	err := ValidateSymlinkTarget(traversalTarget, toolsDir)
	if err == nil {
		t.Error("ValidateSymlinkTarget should reject path traversal attempts")
	}
}

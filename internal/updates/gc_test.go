package updates

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGarbageCollectVersions_RemovesOldVersions(t *testing.T) {
	dir := t.TempDir()
	// Create version directories
	os.MkdirAll(filepath.Join(dir, "node-18.0.0"), 0755)
	os.MkdirAll(filepath.Join(dir, "node-20.0.0"), 0755)
	os.MkdirAll(filepath.Join(dir, "node-20.1.0"), 0755)

	// Set old mtime on 18.0.0
	old := time.Now().Add(-10 * 24 * time.Hour)
	os.Chtimes(filepath.Join(dir, "node-18.0.0"), old, old)

	err := GarbageCollectVersions(dir, "node", "20.1.0", "20.0.0", 7*24*time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// 18.0.0 should be removed (old, not active, not previous)
	if _, err := os.Stat(filepath.Join(dir, "node-18.0.0")); !os.IsNotExist(err) {
		t.Error("expected node-18.0.0 to be removed")
	}
	// 20.0.0 should remain (previous/rollback target)
	if _, err := os.Stat(filepath.Join(dir, "node-20.0.0")); err != nil {
		t.Error("expected node-20.0.0 to remain (rollback target)")
	}
	// 20.1.0 should remain (active)
	if _, err := os.Stat(filepath.Join(dir, "node-20.1.0")); err != nil {
		t.Error("expected node-20.1.0 to remain (active)")
	}
}

func TestGarbageCollectVersions_ProtectsActive(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "rg-14.0.0"), 0755)

	// Even if old, active is never deleted
	old := time.Now().Add(-30 * 24 * time.Hour)
	os.Chtimes(filepath.Join(dir, "rg-14.0.0"), old, old)

	err := GarbageCollectVersions(dir, "rg", "14.0.0", "", 7*24*time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "rg-14.0.0")); err != nil {
		t.Error("active version should never be removed")
	}
}

func TestGarbageCollectVersions_ProtectsPrevious(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "rg-13.0.0"), 0755)
	os.MkdirAll(filepath.Join(dir, "rg-14.0.0"), 0755)

	old := time.Now().Add(-30 * 24 * time.Hour)
	os.Chtimes(filepath.Join(dir, "rg-13.0.0"), old, old)

	err := GarbageCollectVersions(dir, "rg", "14.0.0", "13.0.0", 7*24*time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "rg-13.0.0")); err != nil {
		t.Error("previous version should never be removed")
	}
}

func TestGarbageCollectVersions_RetentionBoundary(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "jq-1.6"), 0755)

	// Set mtime to 6 days ago (within 7-day retention)
	recent := time.Now().Add(-6 * 24 * time.Hour)
	os.Chtimes(filepath.Join(dir, "jq-1.6"), recent, recent)

	err := GarbageCollectVersions(dir, "jq", "1.7", "", 7*24*time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "jq-1.6")); err != nil {
		t.Error("version within retention period should not be removed")
	}
}

func TestGarbageCollectVersions_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	err := GarbageCollectVersions(dir, "node", "20.0.0", "", 7*24*time.Hour, time.Now())
	if err != nil {
		t.Fatal("should not error on empty directory")
	}
}

func TestGarbageCollectVersions_IgnoresOtherTools(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ripgrep-14.0.0"), 0755)

	old := time.Now().Add(-30 * 24 * time.Hour)
	os.Chtimes(filepath.Join(dir, "ripgrep-14.0.0"), old, old)

	// GC for "node" shouldn't touch "ripgrep" dirs
	err := GarbageCollectVersions(dir, "node", "20.0.0", "", 7*24*time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "ripgrep-14.0.0")); err != nil {
		t.Error("GC should not touch other tools' directories")
	}
}

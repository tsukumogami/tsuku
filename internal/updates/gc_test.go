package updates

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGarbageCollectVersions_RemovesOldVersions(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "node-18.0.0"))
	mkdir(t, filepath.Join(dir, "node-20.0.0"))
	mkdir(t, filepath.Join(dir, "node-20.1.0"))

	setMtime(t, filepath.Join(dir, "node-18.0.0"), time.Now().Add(-10*24*time.Hour))

	if err := GarbageCollectVersions(dir, "node", "20.1.0", "20.0.0", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "node-18.0.0")); !os.IsNotExist(err) {
		t.Error("expected node-18.0.0 to be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "node-20.0.0")); err != nil {
		t.Error("expected node-20.0.0 to remain (rollback target)")
	}
	if _, err := os.Stat(filepath.Join(dir, "node-20.1.0")); err != nil {
		t.Error("expected node-20.1.0 to remain (active)")
	}
}

func TestGarbageCollectVersions_ProtectsActive(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "rg-14.0.0"))
	setMtime(t, filepath.Join(dir, "rg-14.0.0"), time.Now().Add(-30*24*time.Hour))

	if err := GarbageCollectVersions(dir, "rg", "14.0.0", "", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "rg-14.0.0")); err != nil {
		t.Error("active version should never be removed")
	}
}

func TestGarbageCollectVersions_ProtectsPrevious(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "rg-13.0.0"))
	mkdir(t, filepath.Join(dir, "rg-14.0.0"))
	setMtime(t, filepath.Join(dir, "rg-13.0.0"), time.Now().Add(-30*24*time.Hour))

	if err := GarbageCollectVersions(dir, "rg", "14.0.0", "13.0.0", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "rg-13.0.0")); err != nil {
		t.Error("previous version should never be removed")
	}
}

func TestGarbageCollectVersions_RetentionBoundary(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "jq-1.6"))
	setMtime(t, filepath.Join(dir, "jq-1.6"), time.Now().Add(-6*24*time.Hour))

	if err := GarbageCollectVersions(dir, "jq", "1.7", "", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "jq-1.6")); err != nil {
		t.Error("version within retention period should not be removed")
	}
}

func TestGarbageCollectVersions_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := GarbageCollectVersions(dir, "node", "20.0.0", "", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal("should not error on empty directory")
	}
}

func TestGarbageCollectVersions_IgnoresOtherTools(t *testing.T) {
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "ripgrep-14.0.0"))
	setMtime(t, filepath.Join(dir, "ripgrep-14.0.0"), time.Now().Add(-30*24*time.Hour))

	if err := GarbageCollectVersions(dir, "node", "20.0.0", "", 7*24*time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "ripgrep-14.0.0")); err != nil {
		t.Error("GC should not touch other tools' directories")
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func setMtime(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

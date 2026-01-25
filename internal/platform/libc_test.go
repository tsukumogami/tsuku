package platform

import (
	"path/filepath"
	"testing"
)

func TestDetectLibcWithRoot_Musl(t *testing.T) {
	root := filepath.Join("testdata", "libc", "musl")
	libc := DetectLibcWithRoot(root)
	if libc != "musl" {
		t.Errorf("DetectLibcWithRoot(%q) = %q, want %q", root, libc, "musl")
	}
}

func TestDetectLibcWithRoot_MuslArm64(t *testing.T) {
	root := filepath.Join("testdata", "libc", "musl-arm64")
	libc := DetectLibcWithRoot(root)
	if libc != "musl" {
		t.Errorf("DetectLibcWithRoot(%q) = %q, want %q", root, libc, "musl")
	}
}

func TestDetectLibcWithRoot_Glibc(t *testing.T) {
	root := filepath.Join("testdata", "libc", "glibc")
	libc := DetectLibcWithRoot(root)
	if libc != "glibc" {
		t.Errorf("DetectLibcWithRoot(%q) = %q, want %q", root, libc, "glibc")
	}
}

func TestDetectLibcWithRoot_EmptyRoot(t *testing.T) {
	// Empty root with no /lib directory should default to glibc
	root := filepath.Join("testdata", "libc", "empty")
	libc := DetectLibcWithRoot(root)
	if libc != "glibc" {
		t.Errorf("DetectLibcWithRoot(%q) = %q, want %q", root, libc, "glibc")
	}
}

func TestDetectLibc(t *testing.T) {
	// DetectLibc uses real filesystem; just verify it returns a valid value
	libc := DetectLibc()
	if libc != "glibc" && libc != "musl" {
		t.Errorf("DetectLibc() = %q, want either %q or %q", libc, "glibc", "musl")
	}
}

func TestValidLibcTypes(t *testing.T) {
	expected := []string{"glibc", "musl"}
	if len(ValidLibcTypes) != len(expected) {
		t.Errorf("ValidLibcTypes has %d entries, want %d", len(ValidLibcTypes), len(expected))
	}
	for i, libc := range expected {
		if ValidLibcTypes[i] != libc {
			t.Errorf("ValidLibcTypes[%d] = %q, want %q", i, ValidLibcTypes[i], libc)
		}
	}
}

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

func TestDetectLibcFromBinary(t *testing.T) {
	// Test with /bin/sh which should exist on all Linux systems
	libc := detectLibcFromBinary("/bin/sh")

	// On Linux, should return a valid libc type
	// On other systems or if /bin/sh isn't ELF, returns ""
	if libc != "" && libc != "glibc" && libc != "musl" {
		t.Errorf("detectLibcFromBinary(/bin/sh) = %q, want empty or valid libc", libc)
	}
}

func TestDetectLibcFromBinary_NonExistent(t *testing.T) {
	libc := detectLibcFromBinary("/nonexistent/binary")
	if libc != "" {
		t.Errorf("detectLibcFromBinary(nonexistent) = %q, want empty", libc)
	}
}

func TestDetectLibcFromBinary_NotELF(t *testing.T) {
	// Test with a non-ELF file (this test file itself)
	libc := detectLibcFromBinary("libc_test.go")
	if libc != "" {
		t.Errorf("detectLibcFromBinary(non-ELF) = %q, want empty", libc)
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

func TestLibcForFamily(t *testing.T) {
	tests := []struct {
		family string
		want   string
	}{
		{"alpine", "musl"},
		{"debian", "glibc"},
		{"rhel", "glibc"},
		{"arch", "glibc"},
		{"suse", "glibc"},
		{"", "glibc"},
		{"unknown", "glibc"},
	}

	for _, tt := range tests {
		t.Run(tt.family, func(t *testing.T) {
			got := LibcForFamily(tt.family)
			if got != tt.want {
				t.Errorf("LibcForFamily(%q) = %q, want %q", tt.family, got, tt.want)
			}
		})
	}
}

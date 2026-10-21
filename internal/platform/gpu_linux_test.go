package platform

// These tests cover the sysfs scan in gpu_linux.go. DetectGPUWithRoot returns
// "none" on every other platform, and isDisplayController exists only on
// Linux, so the _linux suffix keeps them to the build they test.

import (
	"path/filepath"
	"testing"
)

func TestDetectGPUWithRoot_Nvidia(t *testing.T) {
	root := filepath.Join("testdata", "gpu", "nvidia")
	gpu := DetectGPUWithRoot(root)
	if gpu != "nvidia" {
		t.Errorf("DetectGPUWithRoot(%q) = %q, want %q", root, gpu, "nvidia")
	}
}

func TestDetectGPUWithRoot_AMD(t *testing.T) {
	root := filepath.Join("testdata", "gpu", "amd")
	gpu := DetectGPUWithRoot(root)
	if gpu != "amd" {
		t.Errorf("DetectGPUWithRoot(%q) = %q, want %q", root, gpu, "amd")
	}
}

func TestDetectGPUWithRoot_Intel(t *testing.T) {
	root := filepath.Join("testdata", "gpu", "intel")
	gpu := DetectGPUWithRoot(root)
	if gpu != "intel" {
		t.Errorf("DetectGPUWithRoot(%q) = %q, want %q", root, gpu, "intel")
	}
}

func TestDetectGPUWithRoot_NvidiaIntel(t *testing.T) {
	root := filepath.Join("testdata", "gpu", "nvidia-intel")
	gpu := DetectGPUWithRoot(root)
	if gpu != "nvidia" {
		t.Errorf("DetectGPUWithRoot(%q) = %q, want %q (nvidia should win over intel)", root, gpu, "nvidia")
	}
}

func TestDetectGPUWithRoot_AMDIntel(t *testing.T) {
	root := filepath.Join("testdata", "gpu", "amd-intel")
	gpu := DetectGPUWithRoot(root)
	if gpu != "amd" {
		t.Errorf("DetectGPUWithRoot(%q) = %q, want %q (amd should win over intel)", root, gpu, "amd")
	}
}

func TestDetectGPUWithRoot_None(t *testing.T) {
	root := filepath.Join("testdata", "gpu", "none")
	gpu := DetectGPUWithRoot(root)
	if gpu != "none" {
		t.Errorf("DetectGPUWithRoot(%q) = %q, want %q", root, gpu, "none")
	}
}

func TestDetectGPUWithRoot_EmptyRoot(t *testing.T) {
	// Non-existent root should return "none"
	root := filepath.Join("testdata", "gpu", "nonexistent")
	gpu := DetectGPUWithRoot(root)
	if gpu != "none" {
		t.Errorf("DetectGPUWithRoot(%q) = %q, want %q", root, gpu, "none")
	}
}

func TestIsDisplayController(t *testing.T) {
	tests := []struct {
		classStr string
		want     bool
	}{
		{"0x030000", true},  // VGA compatible controller
		{"0x030200", true},  // 3D controller
		{"0x030100", false}, // XGA controller (not VGA or 3D)
		{"0x060000", false}, // Host bridge
		{"0x020000", false}, // Ethernet controller
		{"0x0300", true},    // VGA prefix without prog-if byte (still matches)
		{"0x03", false},     // Too short
		{"", false},         // Empty
	}

	for _, tt := range tests {
		t.Run(tt.classStr, func(t *testing.T) {
			if got := isDisplayController(tt.classStr); got != tt.want {
				t.Errorf("isDisplayController(%q) = %v, want %v", tt.classStr, got, tt.want)
			}
		})
	}
}

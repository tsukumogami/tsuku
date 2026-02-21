package platform

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

func TestDetectGPU(t *testing.T) {
	// DetectGPU uses real filesystem; just verify it returns a valid value
	gpu := DetectGPU()
	valid := false
	for _, v := range ValidGPUTypes {
		if gpu == v {
			valid = true
			break
		}
	}
	if !valid {
		t.Errorf("DetectGPU() = %q, want one of %v", gpu, ValidGPUTypes)
	}
}

func TestValidGPUTypes(t *testing.T) {
	expected := []string{"nvidia", "amd", "intel", "apple", "none"}
	if len(ValidGPUTypes) != len(expected) {
		t.Errorf("ValidGPUTypes has %d entries, want %d", len(ValidGPUTypes), len(expected))
	}
	for i, gpu := range expected {
		if ValidGPUTypes[i] != gpu {
			t.Errorf("ValidGPUTypes[%d] = %q, want %q", i, ValidGPUTypes[i], gpu)
		}
	}
}

func TestTarget_GPU(t *testing.T) {
	tests := []struct {
		name    string
		target  Target
		wantGPU string
	}{
		{
			name:    "nvidia gpu",
			target:  NewTarget("linux/amd64", "debian", "glibc", "nvidia"),
			wantGPU: "nvidia",
		},
		{
			name:    "amd gpu",
			target:  NewTarget("linux/amd64", "debian", "glibc", "amd"),
			wantGPU: "amd",
		},
		{
			name:    "intel gpu",
			target:  NewTarget("linux/amd64", "debian", "glibc", "intel"),
			wantGPU: "intel",
		},
		{
			name:    "apple gpu on darwin",
			target:  NewTarget("darwin/arm64", "", "", "apple"),
			wantGPU: "apple",
		},
		{
			name:    "no gpu",
			target:  NewTarget("linux/amd64", "debian", "glibc", "none"),
			wantGPU: "none",
		},
		{
			name:    "empty gpu",
			target:  NewTarget("linux/amd64", "debian", "glibc", ""),
			wantGPU: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.target.GPU(); got != tt.wantGPU {
				t.Errorf("Target.GPU() = %q, want %q", got, tt.wantGPU)
			}
		})
	}
}

func TestTarget_SetGPU(t *testing.T) {
	target := NewTarget("linux/amd64", "debian", "glibc", "")
	if target.GPU() != "" {
		t.Errorf("initial GPU() = %q, want empty", target.GPU())
	}

	updated := target.SetGPU("nvidia")
	if updated.GPU() != "nvidia" {
		t.Errorf("SetGPU(nvidia).GPU() = %q, want %q", updated.GPU(), "nvidia")
	}

	// Original should be unchanged (value receiver)
	if target.GPU() != "" {
		t.Errorf("original GPU() = %q after SetGPU, want empty (should not mutate)", target.GPU())
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

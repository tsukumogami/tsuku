package platform

import (
	"testing"
)

// The sysfs-scan tests (DetectGPUWithRoot, isDisplayController) are in
// gpu_linux_test.go, since that code exists only on Linux.

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

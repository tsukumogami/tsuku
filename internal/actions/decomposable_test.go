package actions

import (
	"sort"
	"testing"
)

func TestIsPrimitive(t *testing.T) {
	// Tier 1 primitives should return true
	tier1Primitives := []string{
		"download",
		"extract",
		"chmod",
		"install_binaries",
		"set_env",
		"set_rpath",
		"link_dependencies",
		"install_libraries",
	}

	for _, name := range tier1Primitives {
		if !IsPrimitive(name) {
			t.Errorf("IsPrimitive(%q) = false, want true", name)
		}
	}

	// Composite actions should return false
	compositeActions := []string{
		"github_archive",
		"github_file",
		"download_archive",
		"hashicorp_release",
		"homebrew_bottle",
	}

	for _, name := range compositeActions {
		if IsPrimitive(name) {
			t.Errorf("IsPrimitive(%q) = true, want false", name)
		}
	}

	// Unknown actions should return false
	if IsPrimitive("nonexistent_action") {
		t.Error("IsPrimitive(\"nonexistent_action\") = true, want false")
	}
}

func TestPrimitives(t *testing.T) {
	prims := Primitives()

	// Should have exactly 8 primitives
	if len(prims) != 8 {
		t.Errorf("len(Primitives()) = %d, want 8", len(prims))
	}

	// Sort for deterministic comparison
	sort.Strings(prims)

	expected := []string{
		"chmod",
		"download",
		"extract",
		"install_binaries",
		"install_libraries",
		"link_dependencies",
		"set_env",
		"set_rpath",
	}

	for i, name := range expected {
		if prims[i] != name {
			t.Errorf("Primitives()[%d] = %q, want %q", i, prims[i], name)
		}
	}
}

func TestRegisterPrimitive(t *testing.T) {
	// Register a new primitive
	RegisterPrimitive("test_primitive")

	if !IsPrimitive("test_primitive") {
		t.Error("IsPrimitive(\"test_primitive\") = false after RegisterPrimitive")
	}

	// Clean up by removing the test primitive (restore original state)
	delete(primitives, "test_primitive")
}

func TestStepStruct(t *testing.T) {
	// Verify Step struct can be instantiated with all fields
	step := Step{
		Action: "download",
		Params: map[string]interface{}{
			"url":  "https://example.com/file.tar.gz",
			"dest": "file.tar.gz",
		},
		Checksum: "sha256:abc123",
		Size:     1024,
	}

	if step.Action != "download" {
		t.Errorf("step.Action = %q, want %q", step.Action, "download")
	}
	if step.Checksum != "sha256:abc123" {
		t.Errorf("step.Checksum = %q, want %q", step.Checksum, "sha256:abc123")
	}
	if step.Size != 1024 {
		t.Errorf("step.Size = %d, want %d", step.Size, 1024)
	}
}

func TestEvalContextStruct(t *testing.T) {
	// Verify EvalContext struct can be instantiated with all fields
	ctx := EvalContext{
		Context:    nil, // Can be nil for testing
		Version:    "1.29.3",
		VersionTag: "v1.29.3",
		OS:         "linux",
		Arch:       "amd64",
		Recipe:     nil, // Can be nil for testing
		Resolver:   nil, // Can be nil for testing
		Downloader: nil, // Can be nil for testing
	}

	if ctx.Version != "1.29.3" {
		t.Errorf("ctx.Version = %q, want %q", ctx.Version, "1.29.3")
	}
	if ctx.VersionTag != "v1.29.3" {
		t.Errorf("ctx.VersionTag = %q, want %q", ctx.VersionTag, "v1.29.3")
	}
	if ctx.OS != "linux" {
		t.Errorf("ctx.OS = %q, want %q", ctx.OS, "linux")
	}
	if ctx.Arch != "amd64" {
		t.Errorf("ctx.Arch = %q, want %q", ctx.Arch, "amd64")
	}
}

func TestDownloadResultStruct(t *testing.T) {
	// Verify DownloadResult struct can be instantiated with all fields
	result := DownloadResult{
		AssetPath: "/tmp/file.tar.gz",
		Checksum:  "abc123def456",
		Size:      1024,
	}

	if result.AssetPath != "/tmp/file.tar.gz" {
		t.Errorf("result.AssetPath = %q, want %q", result.AssetPath, "/tmp/file.tar.gz")
	}
	if result.Checksum != "abc123def456" {
		t.Errorf("result.Checksum = %q, want %q", result.Checksum, "abc123def456")
	}
	if result.Size != 1024 {
		t.Errorf("result.Size = %d, want %d", result.Size, 1024)
	}
}

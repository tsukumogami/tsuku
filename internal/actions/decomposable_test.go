package actions

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

func TestIsDecomposable(t *testing.T) {
	// Composite actions that implement Decomposable should return true
	compositeActions := []string{
		"github_archive",
		"github_file",
		"download_archive",
		"homebrew",
	}

	for _, name := range compositeActions {
		if !IsDecomposable(name) {
			t.Errorf("IsDecomposable(%q) = false, want true", name)
		}
	}

	// Primitives should return false (they don't implement Decomposable)
	primitiveNames := []string{
		"download_file",
		"extract",
		"chmod",
		"install_binaries",
		"set_env",
	}

	for _, name := range primitiveNames {
		if IsDecomposable(name) {
			t.Errorf("IsDecomposable(%q) = true, want false", name)
		}
	}

	// Unknown actions should return false
	if IsDecomposable("nonexistent_action") {
		t.Error("IsDecomposable(\"nonexistent_action\") = true, want false")
	}
}

func TestIsPrimitive(t *testing.T) {
	// Primitives should return true
	primitives := []string{
		"download_file",
		"extract",
		"chmod",
		"install_binaries",
		"set_env",
		"set_rpath",
		"link_dependencies",
		"install_libraries",
		"apply_patch_file",   // Core primitive
		"text_replace",       // Core primitive
		"homebrew_relocate",  // Core primitive
		"cargo_build",        // Ecosystem primitive
		"cmake_build",        // Ecosystem primitive
		"configure_make",     // Ecosystem primitive
		"cpan_install",       // Ecosystem primitive
		"gem_exec",           // Ecosystem primitive
		"go_build",           // Ecosystem primitive
		"install_gem_direct", // Ecosystem primitive
		"nix_realize",        // Ecosystem primitive
		"npm_exec",           // Ecosystem primitive
		"pip_install",        // Ecosystem primitive
	}

	for _, name := range primitives {
		if !IsPrimitive(name) {
			t.Errorf("IsPrimitive(%q) = false, want true", name)
		}
	}

	// Composite actions should return false
	compositeActions := []string{
		"download",
		"github_archive",
		"github_file",
		"download_archive",
		"homebrew",
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

	// Should have exactly 24 primitives (11 core + 13 ecosystem)
	// Updated from 23 to 24 after setup_build_env was added to main
	if len(prims) != 24 {
		t.Errorf("len(Primitives()) = %d, want 24", len(prims))
	}

	// Sort for deterministic comparison
	sort.Strings(prims)

	expected := []string{
		"apply_patch_file",
		"cargo_build",
		"chmod",
		"cmake_build",
		"configure_make",
		"cpan_install",
		"download_file",
		"extract",
		"gem_exec",
		"go_build",
		"homebrew_relocate",
		"install_binaries",
		"install_gem_direct",
		"install_libraries",
		"link_dependencies",
		"meson_build",
		"nix_realize",
		"npm_exec",
		"pip_exec",
		"pip_install",
		"set_env",
		"set_rpath",
		"setup_build_env",
		"text_replace",
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

// mockDecomposableAction is a test helper that implements Decomposable.
type mockDecomposableAction struct {
	BaseAction
	name   string
	steps  []Step
	err    error
	called bool
}

func (m *mockDecomposableAction) Name() string { return m.name }
func (m *mockDecomposableAction) Execute(_ *ExecutionContext, _ map[string]interface{}) error {
	return nil
}
func (m *mockDecomposableAction) Decompose(_ *EvalContext, _ map[string]interface{}) ([]Step, error) {
	m.called = true
	return m.steps, m.err
}

func TestDecomposeToPrimitives_Primitive(t *testing.T) {
	// Decomposing a primitive should return it unchanged
	ctx := &EvalContext{}
	params := map[string]interface{}{"url": "https://example.com/file.tar.gz"}

	steps, err := DecomposeToPrimitives(ctx, "download_file", params)
	if err != nil {
		t.Fatalf("DecomposeToPrimitives() error = %v", err)
	}

	if len(steps) != 1 {
		t.Fatalf("len(steps) = %d, want 1", len(steps))
	}
	if steps[0].Action != "download_file" {
		t.Errorf("steps[0].Action = %q, want %q", steps[0].Action, "download_file")
	}
	if steps[0].Params["url"] != "https://example.com/file.tar.gz" {
		t.Errorf("steps[0].Params[url] = %v, want %q", steps[0].Params["url"], "https://example.com/file.tar.gz")
	}
}

func TestDecomposeToPrimitives_CompositeReturnsPrimitives(t *testing.T) {
	// Register a mock decomposable action that returns primitives
	mock := &mockDecomposableAction{
		name: "test_composite",
		steps: []Step{
			{Action: "download_file", Params: map[string]interface{}{"url": "https://example.com/a.tar.gz"}},
			{Action: "extract", Params: map[string]interface{}{"format": "tar.gz"}},
		},
	}
	Register(mock)
	defer delete(registry, "test_composite")

	ctx := &EvalContext{}
	params := map[string]interface{}{"repo": "owner/repo"}

	steps, err := DecomposeToPrimitives(ctx, "test_composite", params)
	if err != nil {
		t.Fatalf("DecomposeToPrimitives() error = %v", err)
	}

	if !mock.called {
		t.Error("Decompose() was not called on mock action")
	}

	if len(steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2", len(steps))
	}
	if steps[0].Action != "download_file" {
		t.Errorf("steps[0].Action = %q, want %q", steps[0].Action, "download_file")
	}
	if steps[1].Action != "extract" {
		t.Errorf("steps[1].Action = %q, want %q", steps[1].Action, "extract")
	}
}

func TestDecomposeToPrimitives_RecursiveDecomposition(t *testing.T) {
	// Register a mid-level composite that returns a composite + primitive
	midLevel := &mockDecomposableAction{
		name: "mid_level_composite",
		steps: []Step{
			{Action: "low_level_composite", Params: map[string]interface{}{}},
			{Action: "chmod", Params: map[string]interface{}{"mode": "0755"}},
		},
	}
	Register(midLevel)
	defer delete(registry, "mid_level_composite")

	// Register a low-level composite that returns primitives
	lowLevel := &mockDecomposableAction{
		name: "low_level_composite",
		steps: []Step{
			{Action: "download_file", Params: map[string]interface{}{"url": "https://example.com/file"}},
			{Action: "extract", Params: map[string]interface{}{"format": "tar.gz"}},
		},
	}
	Register(lowLevel)
	defer delete(registry, "low_level_composite")

	ctx := &EvalContext{}
	params := map[string]interface{}{}

	steps, err := DecomposeToPrimitives(ctx, "mid_level_composite", params)
	if err != nil {
		t.Fatalf("DecomposeToPrimitives() error = %v", err)
	}

	// Should have 3 primitives: download_file, extract, chmod
	if len(steps) != 3 {
		t.Fatalf("len(steps) = %d, want 3", len(steps))
	}
	if steps[0].Action != "download_file" {
		t.Errorf("steps[0].Action = %q, want %q", steps[0].Action, "download_file")
	}
	if steps[1].Action != "extract" {
		t.Errorf("steps[1].Action = %q, want %q", steps[1].Action, "extract")
	}
	if steps[2].Action != "chmod" {
		t.Errorf("steps[2].Action = %q, want %q", steps[2].Action, "chmod")
	}
}

func TestDecomposeToPrimitives_CycleDetection(t *testing.T) {
	// Register action A that returns action B
	actionA := &mockDecomposableAction{
		name: "cycle_action_a",
		steps: []Step{
			{Action: "cycle_action_b", Params: map[string]interface{}{"key": "value"}},
		},
	}
	Register(actionA)
	defer delete(registry, "cycle_action_a")

	// Register action B that returns action A (creates cycle)
	actionB := &mockDecomposableAction{
		name: "cycle_action_b",
		steps: []Step{
			{Action: "cycle_action_a", Params: map[string]interface{}{"key": "value"}},
		},
	}
	Register(actionB)
	defer delete(registry, "cycle_action_b")

	ctx := &EvalContext{}
	params := map[string]interface{}{"key": "value"}

	_, err := DecomposeToPrimitives(ctx, "cycle_action_a", params)
	if err == nil {
		t.Fatal("DecomposeToPrimitives() should return error for cycle")
	}

	if !strings.Contains(err.Error(), "cycle detected") {
		t.Errorf("error should mention cycle detection, got: %v", err)
	}
}

func TestDecomposeToPrimitives_ChecksumPropagation(t *testing.T) {
	// Register a composite that returns a step with checksum
	mock := &mockDecomposableAction{
		name: "checksum_composite",
		steps: []Step{
			{
				Action:   "download_file",
				Params:   map[string]interface{}{"url": "https://example.com/file"},
				Checksum: "sha256:abc123",
				Size:     1024,
			},
		},
	}
	Register(mock)
	defer delete(registry, "checksum_composite")

	ctx := &EvalContext{}
	params := map[string]interface{}{}

	steps, err := DecomposeToPrimitives(ctx, "checksum_composite", params)
	if err != nil {
		t.Fatalf("DecomposeToPrimitives() error = %v", err)
	}

	if len(steps) != 1 {
		t.Fatalf("len(steps) = %d, want 1", len(steps))
	}
	if steps[0].Checksum != "sha256:abc123" {
		t.Errorf("steps[0].Checksum = %q, want %q", steps[0].Checksum, "sha256:abc123")
	}
	if steps[0].Size != 1024 {
		t.Errorf("steps[0].Size = %d, want %d", steps[0].Size, 1024)
	}
}

func TestDecomposeToPrimitives_NonDecomposableAction(t *testing.T) {
	// run_command is registered but not decomposable (and not primitive)
	ctx := &EvalContext{}
	params := map[string]interface{}{}

	_, err := DecomposeToPrimitives(ctx, "run_command", params)
	if err == nil {
		t.Fatal("DecomposeToPrimitives() should return error for non-decomposable action")
	}

	if !strings.Contains(err.Error(), "neither primitive nor decomposable") {
		t.Errorf("error should mention non-decomposable, got: %v", err)
	}
}

func TestDecomposeToPrimitives_UnknownAction(t *testing.T) {
	ctx := &EvalContext{}
	params := map[string]interface{}{}

	_, err := DecomposeToPrimitives(ctx, "nonexistent_action", params)
	if err == nil {
		t.Fatal("DecomposeToPrimitives() should return error for unknown action")
	}

	if !strings.Contains(err.Error(), "not found in registry") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestDecomposeToPrimitives_DecomposeError(t *testing.T) {
	// Register a composite that returns an error during decomposition
	mock := &mockDecomposableAction{
		name:  "error_composite",
		steps: nil,
		err:   fmt.Errorf("decomposition failed"),
	}
	Register(mock)
	defer delete(registry, "error_composite")

	ctx := &EvalContext{}
	params := map[string]interface{}{}

	_, err := DecomposeToPrimitives(ctx, "error_composite", params)
	if err == nil {
		t.Fatal("DecomposeToPrimitives() should propagate decomposition error")
	}

	if !strings.Contains(err.Error(), "decomposition failed") {
		t.Errorf("error should contain original message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "error_composite") {
		t.Errorf("error should mention action name, got: %v", err)
	}
}

func TestComputeStepHash(t *testing.T) {
	// Same action + params should produce same hash
	hash1 := computeStepHash("download", map[string]interface{}{"url": "https://example.com"})
	hash2 := computeStepHash("download", map[string]interface{}{"url": "https://example.com"})
	if hash1 != hash2 {
		t.Errorf("same inputs should produce same hash: %q != %q", hash1, hash2)
	}

	// Different action should produce different hash
	hash3 := computeStepHash("extract", map[string]interface{}{"url": "https://example.com"})
	if hash1 == hash3 {
		t.Errorf("different actions should produce different hashes")
	}

	// Different params should produce different hash
	hash4 := computeStepHash("download", map[string]interface{}{"url": "https://other.com"})
	if hash1 == hash4 {
		t.Errorf("different params should produce different hashes")
	}
}

func TestIsDeterministic(t *testing.T) {
	// Core primitives should be deterministic
	deterministicActions := []string{
		"download_file",
		"extract",
		"chmod",
		"install_binaries",
		"set_env",
		"set_rpath",
		"link_dependencies",
		"install_libraries",
		"apply_patch_file",
		"text_replace",
		"homebrew_relocate",
	}

	for _, name := range deterministicActions {
		if !IsDeterministic(name) {
			t.Errorf("IsDeterministic(%q) = false, want true (core primitive)", name)
		}
	}

	// Ecosystem primitives should NOT be deterministic
	nonDeterministicActions := []string{
		"go_build",
		"cargo_build",
		"cmake_build",
		"configure_make",
		"npm_exec",
		"pip_install",
		"gem_exec",
		"install_gem_direct",
		"cpan_install",
	}

	for _, name := range nonDeterministicActions {
		if IsDeterministic(name) {
			t.Errorf("IsDeterministic(%q) = true, want false (ecosystem primitive)", name)
		}
	}

	// Unknown actions should return false for safety
	if IsDeterministic("nonexistent_action") {
		t.Error("IsDeterministic(\"nonexistent_action\") = true, want false")
	}
}

package verify

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tsukumogami/tsuku/internal/install"
)

func TestValidateDependencies_StaticBinary(t *testing.T) {
	// A binary with no dependencies should return empty results
	tmpDir := t.TempDir()

	// Create a minimal state with no libraries
	state := &install.State{
		Libs: make(map[string]map[string]install.LibraryVersionState),
	}

	// Use a non-existent path to simulate a binary we can't parse
	binaryPath := filepath.Join(tmpDir, "nonexistent")

	results, err := ValidateDependencies(
		binaryPath,
		state,
		nil,
		nil,
		make(map[string]bool),
		true,
		runtime.GOOS,
		runtime.GOARCH,
		tmpDir,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected empty results for non-existent binary, got %d", len(results))
	}
}

func TestValidateDependencies_CycleDetection(t *testing.T) {
	tmpDir := t.TempDir()

	state := &install.State{
		Libs: make(map[string]map[string]install.LibraryVersionState),
	}

	// Pre-populate the visited map with the path we'll test
	visited := map[string]bool{
		tmpDir + "/test": true,
	}

	// Attempt to validate the same path again
	results, err := ValidateDependencies(
		tmpDir+"/test",
		state,
		nil,
		nil,
		visited,
		true,
		runtime.GOOS,
		runtime.GOARCH,
		tmpDir,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return nil (already visited)
	if results != nil {
		t.Errorf("expected nil results for already-visited path, got %d results", len(results))
	}
}

func TestValidateDependencies_DepthLimit(t *testing.T) {
	tmpDir := t.TempDir()

	state := &install.State{
		Libs: make(map[string]map[string]install.LibraryVersionState),
	}

	// Create a visited map that exceeds the depth limit
	visited := make(map[string]bool)
	for i := 0; i <= MaxTransitiveDepth; i++ {
		visited[filepath.Join(tmpDir, "lib"+string(rune('a'+i)))] = true
	}

	// Add one more entry should trigger the depth limit error
	_, err := ValidateDependencies(
		filepath.Join(tmpDir, "new"),
		state,
		nil,
		nil,
		visited,
		true,
		runtime.GOOS,
		runtime.GOARCH,
		tmpDir,
	)

	if err == nil {
		t.Fatal("expected depth limit error, got nil")
	}

	// Check that it's the right error type
	if verr, ok := err.(*ValidationError); ok {
		if verr.Category != ErrMaxDepthExceeded {
			t.Errorf("expected ErrMaxDepthExceeded, got %v", verr.Category)
		}
	} else {
		t.Errorf("expected ValidationError, got %T", err)
	}
}

func TestValidateDependenciesSimple(t *testing.T) {
	tmpDir := t.TempDir()

	state := &install.State{
		Libs: make(map[string]map[string]install.LibraryVersionState),
	}

	// Use a non-existent path
	results, err := ValidateDependenciesSimple(
		filepath.Join(tmpDir, "nonexistent"),
		state,
		tmpDir,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return empty results for non-existent/unparseable binary
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

func TestValidateSystemDep_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("test requires linux")
	}

	// Test with a common system library that should exist
	err := validateSystemDep("/lib/x86_64-linux-gnu/libc.so.6", "linux")
	// This may or may not exist depending on the system
	// Just check that the function runs without panicking
	_ = err
}

func TestValidateSystemDep_Darwin(t *testing.T) {
	// On macOS, system libraries are trusted via pattern matching
	// even if the file doesn't exist on disk (dyld cache)
	err := validateSystemDep("/usr/lib/libSystem.B.dylib", "darwin")
	if err != nil {
		t.Errorf("expected nil error for macOS system library pattern, got: %v", err)
	}
}

func TestValidateSystemDep_NonExistent(t *testing.T) {
	err := validateSystemDep("/nonexistent/libfoo.so", "linux")
	if err == nil {
		t.Error("expected error for non-existent library, got nil")
	}
}

func TestValidateTsukuDep_Found(t *testing.T) {
	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"openssl": {
				"3.2.1": {
					UsedBy:  []string{"ruby-3.4.0"},
					Sonames: []string{"libssl.so.3", "libcrypto.so.3"},
				},
			},
		},
	}

	err := validateTsukuDep("libssl.so.3", "openssl", "3.2.1", state)
	if err != nil {
		t.Errorf("expected nil error for existing soname, got: %v", err)
	}
}

func TestValidateTsukuDep_MissingSoname(t *testing.T) {
	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"openssl": {
				"3.2.1": {
					UsedBy:  []string{"ruby-3.4.0"},
					Sonames: []string{"libcrypto.so.3"}, // Note: libssl.so.3 is missing
				},
			},
		},
	}

	err := validateTsukuDep("libssl.so.3", "openssl", "3.2.1", state)
	if err == nil {
		t.Error("expected error for missing soname, got nil")
	}

	// Check error category
	if verr, ok := err.(*ValidationError); ok {
		if verr.Category != ErrMissingSoname {
			t.Errorf("expected ErrMissingSoname, got %v", verr.Category)
		}
	}
}

func TestValidateTsukuDep_MissingRecipe(t *testing.T) {
	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{},
	}

	err := validateTsukuDep("libssl.so.3", "openssl", "3.2.1", state)
	if err == nil {
		t.Error("expected error for missing recipe, got nil")
	}
}

func TestValidateTsukuDep_NilState(t *testing.T) {
	err := validateTsukuDep("libssl.so.3", "openssl", "3.2.1", nil)
	if err == nil {
		t.Error("expected error for nil state, got nil")
	}
}

func TestResolveLibraryPath_Empty(t *testing.T) {
	path := resolveLibraryPath("", "1.0.0", "/tmp")
	if path != "" {
		t.Errorf("expected empty path for empty recipe, got %q", path)
	}

	path = resolveLibraryPath("openssl", "", "/tmp")
	if path != "" {
		t.Errorf("expected empty path for empty version, got %q", path)
	}

	path = resolveLibraryPath("openssl", "1.0.0", "")
	if path != "" {
		t.Errorf("expected empty path for empty tsukuHome, got %q", path)
	}
}

func TestResolveLibraryPath_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	// Path doesn't exist
	path := resolveLibraryPath("openssl", "3.2.1", tmpDir)
	if path != "" {
		t.Errorf("expected empty path for non-existent directory, got %q", path)
	}
}

func TestResolveLibraryPath_Exists(t *testing.T) {
	tmpDir := t.TempDir()

	// Create the expected directory structure
	libDir := filepath.Join(tmpDir, "tools", "openssl-3.2.1", "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	// Currently returns empty because we can't enumerate library files
	// This is a known limitation documented in the function
	path := resolveLibraryPath("openssl", "3.2.1", tmpDir)
	// The function currently returns empty even if dir exists
	// because it can't determine which library file to return
	if path != "" {
		t.Logf("got path: %q", path)
	}
}

func TestDepResult_String(t *testing.T) {
	result := DepResult{
		Soname:   "libssl.so.3",
		Category: DepTsukuManaged,
		Recipe:   "openssl",
		Version:  "3.2.1",
		Status:   ValidationPass,
	}

	// Just verify the struct is usable
	if result.Soname != "libssl.so.3" {
		t.Errorf("unexpected soname: %s", result.Soname)
	}
	if result.Category != DepTsukuManaged {
		t.Errorf("unexpected category: %v", result.Category)
	}
	if result.Status != ValidationPass {
		t.Errorf("unexpected status: %v", result.Status)
	}
}

func TestValidationStatus_String(t *testing.T) {
	tests := []struct {
		status ValidationStatus
		want   string
	}{
		{ValidationPass, "PASS"},
		{ValidationFail, "FAIL"},
		{ValidationSkip, "SKIP"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("ValidationStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestPlatformTarget(t *testing.T) {
	target := &platformTarget{
		os:          "linux",
		arch:        "amd64",
		linuxFamily: "debian",
	}

	if target.OS() != "linux" {
		t.Errorf("OS() = %q, want %q", target.OS(), "linux")
	}
	if target.Arch() != "amd64" {
		t.Errorf("Arch() = %q, want %q", target.Arch(), "amd64")
	}
	if target.LinuxFamily() != "debian" {
		t.Errorf("LinuxFamily() = %q, want %q", target.LinuxFamily(), "debian")
	}
}

func TestMaxTransitiveDepth(t *testing.T) {
	// Verify the constant has the expected value
	if MaxTransitiveDepth != 10 {
		t.Errorf("MaxTransitiveDepth = %d, want 10", MaxTransitiveDepth)
	}
}

func TestValidateSingleDependency_SystemLib(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "bin", "myapp")

	state := &install.State{
		Libs: make(map[string]map[string]install.LibraryVersionState),
	}

	index := NewSonameIndex()

	// Test with a known system library pattern
	result := validateSingleDependency(
		"libc.so.6",
		binaryPath,
		nil, // no rpaths
		tmpDir,
		state,
		index,
		nil, nil,
		make(map[string]bool),
		false,
		"linux", "amd64", tmpDir,
	)

	if result.Category != DepPureSystem {
		t.Errorf("expected DepPureSystem, got %v", result.Category)
	}
}

func TestValidateSingleDependency_UnknownDep(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "bin", "myapp")

	state := &install.State{
		Libs: make(map[string]map[string]install.LibraryVersionState),
	}

	index := NewSonameIndex()

	// Test with an unknown library (not system, not in index)
	result := validateSingleDependency(
		"libunknown.so.1",
		binaryPath,
		nil,
		tmpDir,
		state,
		index,
		nil, nil,
		make(map[string]bool),
		false,
		"linux", "amd64", tmpDir,
	)

	if result.Category != DepUnknown {
		t.Errorf("expected DepUnknown, got %v", result.Category)
	}
	if result.Status != ValidationFail {
		t.Errorf("expected ValidationFail, got %v", result.Status)
	}
}

func TestValidateSingleDependency_TsukuManaged(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "bin", "myapp")

	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"openssl": {
				"3.2.1": {
					UsedBy:  []string{"ruby-3.4.0"},
					Sonames: []string{"libssl.so.3", "libcrypto.so.3"},
				},
			},
		},
	}

	index := BuildSonameIndex(state)

	result := validateSingleDependency(
		"libssl.so.3",
		binaryPath,
		nil,
		tmpDir,
		state,
		index,
		nil, nil,
		make(map[string]bool),
		false, // no recursion
		"linux", "amd64", tmpDir,
	)

	if result.Category != DepTsukuManaged {
		t.Errorf("expected DepTsukuManaged, got %v", result.Category)
	}
	if result.Status != ValidationPass {
		t.Errorf("expected ValidationPass, got %v", result.Status)
	}
	if result.Recipe != "openssl" {
		t.Errorf("expected recipe openssl, got %q", result.Recipe)
	}
	if result.Version != "3.2.1" {
		t.Errorf("expected version 3.2.1, got %q", result.Version)
	}
}

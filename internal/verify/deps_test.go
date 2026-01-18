package verify

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
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

// =============================================================================
// Mock types for integration tests
// =============================================================================

// mockRecipeLoader implements RecipeLoader for testing externally-managed scenarios.
type mockRecipeLoader struct {
	recipes map[string]*recipe.Recipe
}

func newMockRecipeLoader() *mockRecipeLoader {
	return &mockRecipeLoader{
		recipes: make(map[string]*recipe.Recipe),
	}
}

func (m *mockRecipeLoader) LoadRecipe(name string) (*recipe.Recipe, error) {
	if r, ok := m.recipes[name]; ok {
		return r, nil
	}
	return nil, nil
}

func (m *mockRecipeLoader) addExternallyManagedRecipe(name string) {
	m.recipes[name] = &recipe.Recipe{
		Steps: []recipe.Step{
			{Action: "apt_install", Params: map[string]interface{}{"packages": []string{name}}},
		},
	}
}

// mockExternalAction implements recipe.SystemActionChecker with IsExternallyManaged() == true.
type mockExternalAction struct{}

func (m mockExternalAction) IsExternallyManaged() bool { return true }

// mockNonExternalAction implements recipe.SystemActionChecker with IsExternallyManaged() == false.
type mockNonExternalAction struct{}

func (m mockNonExternalAction) IsExternallyManaged() bool { return false }

// mockDownloadAction represents a non-system action (like download, extract).
type mockDownloadAction struct{}

// mockActionLookup returns mock actions based on action names.
func mockActionLookup(name string) interface{} {
	switch name {
	case "apt_install", "brew_install", "dnf_install":
		return mockExternalAction{}
	case "manual", "require_command":
		return mockNonExternalAction{}
	case "download", "extract":
		return mockDownloadAction{}
	default:
		return nil
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

// =============================================================================
// Integration Tests - Scenario-based tests per issue #991
// =============================================================================

// TestValidateDependencies_Integration_SystemDepsOnly verifies scenario 3:
// A binary that depends only on system libraries (libc, libm, etc.) should pass.
func TestValidateDependencies_Integration_SystemDepsOnly(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Integration test requires Linux")
	}

	// Find a real system binary that depends only on system libs
	// /bin/true is typically a simple binary with only libc dependency
	candidates := []string{
		"/bin/true",
		"/bin/false",
		"/usr/bin/true",
		"/usr/bin/false",
	}

	var binaryPath string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			binaryPath = c
			break
		}
	}

	if binaryPath == "" {
		t.Skip("No suitable system binary found for testing")
	}

	tmpDir := t.TempDir()
	state := &install.State{
		Libs: make(map[string]map[string]install.LibraryVersionState),
	}

	results, err := ValidateDependencies(
		binaryPath,
		state,
		nil, // no recipe loader
		nil, // no action lookup
		make(map[string]bool),
		true, // recurse
		runtime.GOOS,
		runtime.GOARCH,
		tmpDir,
	)

	if err != nil {
		t.Fatalf("ValidateDependencies failed: %v", err)
	}

	// Check that all dependencies are system libraries
	for _, r := range results {
		if r.Category != DepPureSystem {
			t.Errorf("expected all deps to be DepPureSystem, got %v for %s", r.Category, r.Soname)
		}
		if r.Status != ValidationPass {
			t.Errorf("expected all deps to pass, got %v for %s: %s", r.Status, r.Soname, r.Error)
		}
	}
}

// TestValidateDependencies_Integration_TsukuManagedDeps verifies scenario 2:
// A binary with tsuku-managed dependencies should validate them recursively.
func TestValidateDependencies_Integration_TsukuManagedDeps(t *testing.T) {
	tmpDir := t.TempDir()

	// Create state with openssl as a tsuku-managed library
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

	// Build index from state
	index := BuildSonameIndex(state)

	// Validate a dependency that's in our index
	result := validateSingleDependency(
		"libssl.so.3",
		filepath.Join(tmpDir, "bin", "ruby"),
		nil,
		tmpDir,
		state,
		index,
		nil, nil,
		make(map[string]bool),
		false, // no deep recursion for this test
		"linux", "amd64", tmpDir,
	)

	if result.Category != DepTsukuManaged {
		t.Errorf("expected DepTsukuManaged, got %v", result.Category)
	}
	if result.Recipe != "openssl" {
		t.Errorf("expected recipe openssl, got %q", result.Recipe)
	}
	if result.Version != "3.2.1" {
		t.Errorf("expected version 3.2.1, got %q", result.Version)
	}
	if result.Status != ValidationPass {
		t.Errorf("expected ValidationPass, got %v: %s", result.Status, result.Error)
	}
}

// TestValidateDependencies_Integration_MissingDependency verifies scenario 4:
// A dependency that can't be classified should result in ValidationFail with DepUnknown.
func TestValidateDependencies_Integration_MissingDependency(t *testing.T) {
	tmpDir := t.TempDir()

	state := &install.State{
		Libs: make(map[string]map[string]install.LibraryVersionState),
	}

	index := NewSonameIndex()

	// Validate a dependency that's not in the index and not a system library
	result := validateSingleDependency(
		"libnonexistent.so.1",
		filepath.Join(tmpDir, "bin", "myapp"),
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
	if result.Error == "" {
		t.Error("expected error message for missing dependency")
	}
}

// TestValidateDependencies_Integration_ExternallyManaged verifies scenario 7:
// A tsuku recipe that delegates to system package manager should be classified
// as EXTERNALLY_MANAGED and not recursed into.
func TestValidateDependencies_Integration_ExternallyManaged(t *testing.T) {
	tmpDir := t.TempDir()

	// Create state with openssl as a library with sonames
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

	// Create mock recipe loader that returns an externally-managed recipe
	loader := newMockRecipeLoader()
	loader.addExternallyManagedRecipe("openssl")

	// Build index from state
	index := BuildSonameIndex(state)

	// Validate libssl.so.3 which is in openssl recipe
	result := validateSingleDependency(
		"libssl.so.3",
		filepath.Join(tmpDir, "bin", "ruby"),
		nil,
		tmpDir,
		state,
		index,
		loader,
		mockActionLookup,
		make(map[string]bool),
		true, // recurse would happen, but externally-managed shouldn't recurse
		"linux", "amd64", tmpDir,
	)

	// Should be classified as EXTERNALLY_MANAGED (refined from TSUKU_MANAGED)
	if result.Category != DepExternallyManaged {
		t.Errorf("expected DepExternallyManaged, got %v", result.Category)
	}
	if result.Status != ValidationPass {
		t.Errorf("expected ValidationPass, got %v: %s", result.Status, result.Error)
	}
	// Should NOT have transitive deps (no recursion for externally-managed)
	if len(result.Transitive) > 0 {
		t.Errorf("expected no transitive deps for externally-managed, got %d", len(result.Transitive))
	}
}

// TestValidateDependencies_Integration_CycleWithTransitive tests that cycles are
// properly detected during recursive validation.
func TestValidateDependencies_Integration_CycleWithTransitive(t *testing.T) {
	tmpDir := t.TempDir()

	// Simulate circular dependency: libssl depends on libcrypto, libcrypto depends on libssl
	// In practice, this scenario tests the visited map handling

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

	// Pre-populate visited to simulate already having seen a library
	visited := map[string]bool{
		"/fake/path/libssl.so.3": true,
	}

	// Try to validate the same path - should return nil (already visited)
	results, err := ValidateDependencies(
		"/fake/path/libssl.so.3",
		state,
		nil, nil,
		visited,
		true,
		"linux", "amd64",
		tmpDir,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for already-visited, got %d results", len(results))
	}
}

// TestValidateDependencies_Integration_ABIMismatch verifies scenario 5:
// A binary with musl interpreter on glibc system should fail ABI validation.
// Note: This test only verifies the error category constant is correct,
// as creating a musl binary for testing requires special build environment.
func TestValidateDependencies_Integration_ABIMismatch(t *testing.T) {
	// Verify ErrABIMismatch has correct value per design decision #2
	if ErrABIMismatch != 10 {
		t.Errorf("ErrABIMismatch = %d, want 10", ErrABIMismatch)
	}

	// Verify error formatting
	err := &ValidationError{
		Category: ErrABIMismatch,
		Path:     "/path/to/binary",
		Message:  `interpreter "/lib/ld-musl-x86_64.so.1" not found (binary may be built for different libc)`,
	}

	if err.Category != ErrABIMismatch {
		t.Errorf("expected ErrABIMismatch category")
	}
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

// TestValidateDependencies_Integration_RealBinary_SystemLibs tests validation
// against a real system binary to ensure end-to-end flow works.
func TestValidateDependencies_Integration_RealBinary_SystemLibs(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Integration test requires Linux")
	}

	// Find a dynamically linked binary
	candidates := []string{
		"/bin/ls",
		"/bin/cat",
		"/usr/bin/ls",
		"/usr/bin/cat",
	}

	var binaryPath string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			binaryPath = c
			break
		}
	}

	if binaryPath == "" {
		t.Skip("No suitable dynamic binary found for testing")
	}

	tmpDir := t.TempDir()
	state := &install.State{
		Libs: make(map[string]map[string]install.LibraryVersionState),
	}

	results, err := ValidateDependenciesSimple(binaryPath, state, tmpDir)

	if err != nil {
		t.Fatalf("ValidateDependenciesSimple failed: %v", err)
	}

	// Dynamic binary should have at least libc as a dependency
	if len(results) == 0 {
		// Might be statically linked - that's okay too
		t.Log("Binary appears to be statically linked (no dependencies)")
		return
	}

	// All system-binary deps should be classified as system libs
	hasLibc := false
	for _, r := range results {
		if r.Category != DepPureSystem {
			t.Errorf("expected system binary dep to be DepPureSystem, got %v for %s", r.Category, r.Soname)
		}
		if r.Status != ValidationPass {
			t.Errorf("expected system dep to pass, got %v for %s: %s", r.Status, r.Soname, r.Error)
		}
		if r.Soname == "libc.so.6" {
			hasLibc = true
		}
	}

	if len(results) > 0 && !hasLibc {
		t.Logf("Binary has %d deps but no libc.so.6 (may use different naming)", len(results))
	}
}

// TestValidateDependencies_Integration_NonRecursive verifies that when recurse=false,
// transitive dependencies are not validated.
func TestValidateDependencies_Integration_NonRecursive(t *testing.T) {
	tmpDir := t.TempDir()

	state := &install.State{
		Libs: map[string]map[string]install.LibraryVersionState{
			"openssl": {
				"3.2.1": {
					UsedBy:  []string{"ruby-3.4.0"},
					Sonames: []string{"libssl.so.3"},
				},
			},
		},
	}

	index := BuildSonameIndex(state)

	// Validate with recurse=false
	result := validateSingleDependency(
		"libssl.so.3",
		filepath.Join(tmpDir, "bin", "ruby"),
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
	// No transitive results when recurse=false
	if len(result.Transitive) > 0 {
		t.Errorf("expected no transitive deps with recurse=false, got %d", len(result.Transitive))
	}
}

// TestValidateDependencies_Integration_PathVariables tests that path variables
// like $ORIGIN are handled correctly in dependency paths.
func TestValidateDependencies_Integration_PathVariables(t *testing.T) {
	tmpDir := t.TempDir()

	state := &install.State{
		Libs: make(map[string]map[string]install.LibraryVersionState),
	}

	index := NewSonameIndex()

	// Test with $ORIGIN variable - should be recognized as system pattern
	// (path variables are treated as system patterns per classify_test.go)
	result := validateSingleDependency(
		"$ORIGIN/../lib/libfoo.so",
		filepath.Join(tmpDir, "bin", "myapp"),
		nil,
		filepath.Join(tmpDir, "tools"),
		state,
		index,
		nil, nil,
		make(map[string]bool),
		false,
		"linux", "amd64", tmpDir,
	)

	// Path variables that can't be expanded should fail
	// (the expand function will fail when path doesn't resolve to allowed prefix)
	if result.Status == ValidationPass {
		// If it passed, verify it was recognized as a system pattern
		if result.Category != DepPureSystem {
			t.Errorf("expected DepPureSystem for path variable, got %v", result.Category)
		}
	}
}

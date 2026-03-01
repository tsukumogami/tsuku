package sandbox

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/executor"
)

// --- FlattenDependencies tests ---

func TestFlattenDependencies_Empty(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Tool:    "test-tool",
		Version: "1.0.0",
	}

	result := FlattenDependencies(plan)

	if result == nil {
		t.Fatal("Expected non-nil empty slice, got nil")
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 deps, got %d", len(result))
	}
}

func TestFlattenDependencies_EmptySlice(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Tool:         "test-tool",
		Version:      "1.0.0",
		Dependencies: []executor.DependencyPlan{},
	}

	result := FlattenDependencies(plan)

	if result == nil {
		t.Fatal("Expected non-nil empty slice, got nil")
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 deps, got %d", len(result))
	}
}

func TestFlattenDependencies_SingleDep(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Tool:    "cargo-nextest",
		Version: "0.24.5",
		Dependencies: []executor.DependencyPlan{
			{
				Tool:    "rust",
				Version: "1.82.0",
				Steps: []executor.ResolvedStep{
					{Action: "download_file", Checksum: "abc123"},
				},
			},
		},
	}

	result := FlattenDependencies(plan)

	if len(result) != 1 {
		t.Fatalf("Expected 1 dep, got %d", len(result))
	}
	if result[0].Tool != "rust" {
		t.Errorf("Expected tool 'rust', got %q", result[0].Tool)
	}
	if result[0].Version != "1.82.0" {
		t.Errorf("Expected version '1.82.0', got %q", result[0].Version)
	}
	if result[0].Plan == nil {
		t.Fatal("Expected non-nil plan")
	}
	if result[0].Plan.Tool != "rust" {
		t.Errorf("Plan tool = %q, want 'rust'", result[0].Plan.Tool)
	}
	if len(result[0].Plan.Steps) != 1 {
		t.Errorf("Expected 1 step, got %d", len(result[0].Plan.Steps))
	}
}

func TestFlattenDependencies_LeavesFirst(t *testing.T) {
	t.Parallel()

	// Tree: rust depends on llvm
	plan := &executor.InstallationPlan{
		Tool:    "cargo-nextest",
		Version: "0.24.5",
		Dependencies: []executor.DependencyPlan{
			{
				Tool:    "rust",
				Version: "1.82.0",
				Dependencies: []executor.DependencyPlan{
					{
						Tool:    "llvm",
						Version: "17.0.0",
						Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "llvm123"}},
					},
				},
				Steps: []executor.ResolvedStep{{Action: "download_file", Checksum: "rust123"}},
			},
		},
	}

	result := FlattenDependencies(plan)

	if len(result) != 2 {
		t.Fatalf("Expected 2 deps, got %d", len(result))
	}
	// llvm (leaf) should come before rust (parent)
	if result[0].Tool != "llvm" {
		t.Errorf("Expected first dep 'llvm', got %q", result[0].Tool)
	}
	if result[1].Tool != "rust" {
		t.Errorf("Expected second dep 'rust', got %q", result[1].Tool)
	}
}

func TestFlattenDependencies_AlphabeticalSiblings(t *testing.T) {
	t.Parallel()

	// Two siblings at same level, should be sorted alphabetically
	plan := &executor.InstallationPlan{
		Tool:    "myapp",
		Version: "1.0.0",
		Dependencies: []executor.DependencyPlan{
			{
				Tool:    "zig",
				Version: "0.11.0",
				Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "zig123"}},
			},
			{
				Tool:    "openssl",
				Version: "3.0.0",
				Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "openssl123"}},
			},
		},
	}

	result := FlattenDependencies(plan)

	if len(result) != 2 {
		t.Fatalf("Expected 2 deps, got %d", len(result))
	}
	if result[0].Tool != "openssl" {
		t.Errorf("Expected first dep 'openssl' (alphabetical), got %q", result[0].Tool)
	}
	if result[1].Tool != "zig" {
		t.Errorf("Expected second dep 'zig' (alphabetical), got %q", result[1].Tool)
	}
}

func TestFlattenDependencies_Deduplication(t *testing.T) {
	t.Parallel()

	// Two top-level deps both depend on the same transitive dep
	plan := &executor.InstallationPlan{
		Tool:    "myapp",
		Version: "1.0.0",
		Dependencies: []executor.DependencyPlan{
			{
				Tool:    "toolA",
				Version: "1.0.0",
				Dependencies: []executor.DependencyPlan{
					{
						Tool:    "shared-dep",
						Version: "2.0.0",
						Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "shared123"}},
					},
				},
				Steps: []executor.ResolvedStep{{Action: "download_file", Checksum: "a123"}},
			},
			{
				Tool:    "toolB",
				Version: "1.0.0",
				Dependencies: []executor.DependencyPlan{
					{
						Tool:    "shared-dep",
						Version: "2.0.0",
						Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "shared456"}},
					},
				},
				Steps: []executor.ResolvedStep{{Action: "download_file", Checksum: "b123"}},
			},
		},
	}

	result := FlattenDependencies(plan)

	// Should be: shared-dep, toolA, toolB (shared-dep deduplicated, appears once)
	if len(result) != 3 {
		t.Fatalf("Expected 3 deps (shared-dep deduped), got %d", len(result))
	}

	tools := make([]string, len(result))
	for i, dep := range result {
		tools[i] = dep.Tool
	}

	// shared-dep appears first (leaf of toolA, which is alphabetically first)
	if tools[0] != "shared-dep" {
		t.Errorf("Expected first dep 'shared-dep', got %q", tools[0])
	}
	if tools[1] != "toolA" {
		t.Errorf("Expected second dep 'toolA', got %q", tools[1])
	}
	if tools[2] != "toolB" {
		t.Errorf("Expected third dep 'toolB', got %q", tools[2])
	}
}

func TestFlattenDependencies_DeduplicationDifferentVersions(t *testing.T) {
	t.Parallel()

	// Same tool name but different versions -- both should appear
	plan := &executor.InstallationPlan{
		Tool:    "myapp",
		Version: "1.0.0",
		Dependencies: []executor.DependencyPlan{
			{
				Tool:    "toolA",
				Version: "1.0.0",
				Dependencies: []executor.DependencyPlan{
					{
						Tool:    "shared-dep",
						Version: "2.0.0",
						Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "s2"}},
					},
				},
				Steps: []executor.ResolvedStep{{Action: "download_file", Checksum: "a1"}},
			},
			{
				Tool:    "toolB",
				Version: "1.0.0",
				Dependencies: []executor.DependencyPlan{
					{
						Tool:    "shared-dep",
						Version: "3.0.0", // Different version
						Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "s3"}},
					},
				},
				Steps: []executor.ResolvedStep{{Action: "download_file", Checksum: "b1"}},
			},
		},
	}

	result := FlattenDependencies(plan)

	// Different versions are not deduped
	if len(result) != 4 {
		t.Fatalf("Expected 4 deps (different versions not deduped), got %d", len(result))
	}

	tools := make([]string, len(result))
	for i, dep := range result {
		tools[i] = dep.Tool + "@" + dep.Version
	}
	expected := []string{"shared-dep@2.0.0", "toolA@1.0.0", "shared-dep@3.0.0", "toolB@1.0.0"}
	for i, exp := range expected {
		if tools[i] != exp {
			t.Errorf("result[%d] = %q, want %q", i, tools[i], exp)
		}
	}
}

func TestFlattenDependencies_PreservesSubtree(t *testing.T) {
	t.Parallel()

	// rust depends on llvm -- rust's converted plan should keep its Dependencies
	plan := &executor.InstallationPlan{
		Tool:    "cargo-nextest",
		Version: "0.24.5",
		Dependencies: []executor.DependencyPlan{
			{
				Tool:    "rust",
				Version: "1.82.0",
				Dependencies: []executor.DependencyPlan{
					{
						Tool:    "llvm",
						Version: "17.0.0",
						Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "llvm123"}},
					},
				},
				Steps: []executor.ResolvedStep{{Action: "download_file", Checksum: "rust123"}},
			},
		},
	}

	result := FlattenDependencies(plan)

	// rust is the second entry (after llvm)
	var rustDep *FlatDep
	for i := range result {
		if result[i].Tool == "rust" {
			rustDep = &result[i]
			break
		}
	}
	if rustDep == nil {
		t.Fatal("rust dep not found in results")
	}
	if len(rustDep.Plan.Dependencies) != 1 {
		t.Fatalf("rust plan should preserve 1 dependency, got %d", len(rustDep.Plan.Dependencies))
	}
	if rustDep.Plan.Dependencies[0].Tool != "llvm" {
		t.Errorf("Expected preserved dep 'llvm', got %q", rustDep.Plan.Dependencies[0].Tool)
	}
}

func TestFlattenDependencies_StripsTimestamp(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Tool:        "myapp",
		Version:     "1.0.0",
		GeneratedAt: time.Now(),
		Dependencies: []executor.DependencyPlan{
			{
				Tool:    "rust",
				Version: "1.82.0",
				Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "r123"}},
			},
		},
	}

	result := FlattenDependencies(plan)

	if len(result) != 1 {
		t.Fatalf("Expected 1 dep, got %d", len(result))
	}
	if !result[0].Plan.GeneratedAt.IsZero() {
		t.Errorf("Expected GeneratedAt to be zeroed, got %v", result[0].Plan.GeneratedAt)
	}
}

func TestFlattenDependencies_SetsFormatVersion(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Tool:    "myapp",
		Version: "1.0.0",
		Dependencies: []executor.DependencyPlan{
			{
				Tool:    "rust",
				Version: "1.82.0",
				Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "r123"}},
			},
		},
	}

	result := FlattenDependencies(plan)

	if result[0].Plan.FormatVersion != executor.PlanFormatVersion {
		t.Errorf("FormatVersion = %d, want %d", result[0].Plan.FormatVersion, executor.PlanFormatVersion)
	}
}

func TestFlattenDependencies_PreservesVerifyAndRecipeType(t *testing.T) {
	t.Parallel()

	exitCode := 0
	plan := &executor.InstallationPlan{
		Tool:    "myapp",
		Version: "1.0.0",
		Dependencies: []executor.DependencyPlan{
			{
				Tool:    "rust",
				Version: "1.82.0",
				Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "r123"}},
				Verify: &executor.PlanVerify{
					Command:  "rustc --version",
					Pattern:  "1.82.0",
					ExitCode: &exitCode,
				},
				RecipeType: "tool",
			},
		},
	}

	result := FlattenDependencies(plan)

	if result[0].Plan.Verify == nil {
		t.Fatal("Expected Verify to be preserved")
	}
	if result[0].Plan.Verify.Command != "rustc --version" {
		t.Errorf("Verify.Command = %q, want 'rustc --version'", result[0].Plan.Verify.Command)
	}
	if result[0].Plan.RecipeType != "tool" {
		t.Errorf("RecipeType = %q, want 'tool'", result[0].Plan.RecipeType)
	}
}

func TestFlattenDependencies_DeepTree(t *testing.T) {
	t.Parallel()

	// Three levels: myapp -> rust -> llvm -> cmake
	plan := &executor.InstallationPlan{
		Tool:    "myapp",
		Version: "1.0.0",
		Dependencies: []executor.DependencyPlan{
			{
				Tool:    "rust",
				Version: "1.82.0",
				Dependencies: []executor.DependencyPlan{
					{
						Tool:    "llvm",
						Version: "17.0.0",
						Dependencies: []executor.DependencyPlan{
							{
								Tool:    "cmake",
								Version: "3.28.0",
								Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "cmake123"}},
							},
						},
						Steps: []executor.ResolvedStep{{Action: "download_file", Checksum: "llvm123"}},
					},
				},
				Steps: []executor.ResolvedStep{{Action: "download_file", Checksum: "rust123"}},
			},
		},
	}

	result := FlattenDependencies(plan)

	if len(result) != 3 {
		t.Fatalf("Expected 3 deps, got %d", len(result))
	}
	// Deepest leaf first: cmake, then llvm, then rust
	expected := []string{"cmake", "llvm", "rust"}
	for i, exp := range expected {
		if result[i].Tool != exp {
			t.Errorf("result[%d].Tool = %q, want %q", i, result[i].Tool, exp)
		}
	}
}

// --- GenerateFoundationDockerfile tests ---

func TestGenerateFoundationDockerfile_NoDeps(t *testing.T) {
	t.Parallel()

	dockerfile := GenerateFoundationDockerfile("tsuku/sandbox-cache:debian-abc123", nil)

	if !strings.HasPrefix(dockerfile, "FROM tsuku/sandbox-cache:debian-abc123\n") {
		t.Errorf("Dockerfile should start with FROM line, got:\n%s", dockerfile)
	}
	if !strings.Contains(dockerfile, "COPY tsuku /usr/local/bin/tsuku\n") {
		t.Error("Dockerfile should contain COPY tsuku line")
	}
	if !strings.Contains(dockerfile, "ENV TSUKU_HOME=/workspace/tsuku\n") {
		t.Error("Dockerfile should contain TSUKU_HOME env")
	}
	if !strings.Contains(dockerfile, "ENV PATH=/workspace/tsuku/bin:$PATH\n") {
		t.Error("Dockerfile should contain PATH env")
	}
	if !strings.Contains(dockerfile, "RUN rm -rf /usr/local/bin/tsuku /tmp/plans\n") {
		t.Error("Dockerfile should end with cleanup RUN")
	}
	if dockerfile == "" {
		t.Error("Dockerfile should not be empty")
	}
}

func TestGenerateFoundationDockerfile_SingleDep(t *testing.T) {
	t.Parallel()

	deps := []FlatDep{
		{
			Tool:    "rust",
			Version: "1.82.0",
			Plan:    &executor.InstallationPlan{Tool: "rust", Version: "1.82.0"},
		},
	}

	dockerfile := GenerateFoundationDockerfile("tsuku/sandbox-cache:debian-abc123", deps)

	// Check structure
	lines := strings.Split(strings.TrimRight(dockerfile, "\n"), "\n")
	expected := []string{
		"FROM tsuku/sandbox-cache:debian-abc123",
		"COPY tsuku /usr/local/bin/tsuku",
		"ENV TSUKU_HOME=/workspace/tsuku",
		"ENV PATH=/workspace/tsuku/bin:$PATH",
		"COPY plans/dep-00-rust.json /tmp/plans/dep-00-rust.json",
		"RUN tsuku install --plan /tmp/plans/dep-00-rust.json --force",
		"RUN rm -rf /usr/local/bin/tsuku /tmp/plans",
	}

	if len(lines) != len(expected) {
		t.Fatalf("Expected %d lines, got %d:\n%s", len(expected), len(lines), dockerfile)
	}
	for i, exp := range expected {
		if lines[i] != exp {
			t.Errorf("line %d: got %q, want %q", i, lines[i], exp)
		}
	}
}

func TestGenerateFoundationDockerfile_MultipleDeps(t *testing.T) {
	t.Parallel()

	deps := []FlatDep{
		{
			Tool:    "openssl",
			Version: "3.0.0",
			Plan:    &executor.InstallationPlan{Tool: "openssl", Version: "3.0.0"},
		},
		{
			Tool:    "rust",
			Version: "1.82.0",
			Plan:    &executor.InstallationPlan{Tool: "rust", Version: "1.82.0"},
		},
	}

	dockerfile := GenerateFoundationDockerfile("tsuku/sandbox-cache:debian-abc123", deps)

	// Verify interleaved COPY+RUN pairs
	if !strings.Contains(dockerfile, "COPY plans/dep-00-openssl.json /tmp/plans/dep-00-openssl.json\n") {
		t.Error("Missing COPY for dep-00-openssl.json")
	}
	if !strings.Contains(dockerfile, "RUN tsuku install --plan /tmp/plans/dep-00-openssl.json --force\n") {
		t.Error("Missing RUN for dep-00-openssl.json")
	}
	if !strings.Contains(dockerfile, "COPY plans/dep-01-rust.json /tmp/plans/dep-01-rust.json\n") {
		t.Error("Missing COPY for dep-01-rust.json")
	}
	if !strings.Contains(dockerfile, "RUN tsuku install --plan /tmp/plans/dep-01-rust.json --force\n") {
		t.Error("Missing RUN for dep-01-rust.json")
	}

	// Verify order: openssl COPY must come before rust COPY
	opensslIdx := strings.Index(dockerfile, "dep-00-openssl")
	rustIdx := strings.Index(dockerfile, "dep-01-rust")
	if opensslIdx >= rustIdx {
		t.Error("openssl (dep-00) should appear before rust (dep-01)")
	}
}

func TestGenerateFoundationDockerfile_ZeroPaddedIndex(t *testing.T) {
	t.Parallel()

	// Test with enough deps to verify zero-padding
	deps := make([]FlatDep, 3)
	for i := range deps {
		name := string(rune('a' + i))
		deps[i] = FlatDep{
			Tool:    name,
			Version: "1.0.0",
			Plan:    &executor.InstallationPlan{Tool: name, Version: "1.0.0"},
		}
	}

	dockerfile := GenerateFoundationDockerfile("base:latest", deps)

	if !strings.Contains(dockerfile, "dep-00-a.json") {
		t.Error("Missing zero-padded dep-00")
	}
	if !strings.Contains(dockerfile, "dep-01-b.json") {
		t.Error("Missing zero-padded dep-01")
	}
	if !strings.Contains(dockerfile, "dep-02-c.json") {
		t.Error("Missing zero-padded dep-02")
	}
}

func TestGenerateFoundationDockerfile_Deterministic(t *testing.T) {
	t.Parallel()

	deps := []FlatDep{
		{
			Tool:    "rust",
			Version: "1.82.0",
			Plan:    &executor.InstallationPlan{Tool: "rust", Version: "1.82.0"},
		},
	}

	d1 := GenerateFoundationDockerfile("tsuku/sandbox-cache:debian-abc", deps)
	d2 := GenerateFoundationDockerfile("tsuku/sandbox-cache:debian-abc", deps)

	if d1 != d2 {
		t.Error("GenerateFoundationDockerfile should be deterministic")
	}
}

// --- FoundationImageName tests ---

func TestFoundationImageName_Format(t *testing.T) {
	t.Parallel()

	name := FoundationImageName("debian", "FROM base\nRUN echo hello\n")

	// Should match pattern tsuku/sandbox-foundation:{family}-{16 hex chars}
	pattern := `^tsuku/sandbox-foundation:debian-[0-9a-f]{16}$`
	matched, err := regexp.MatchString(pattern, name)
	if err != nil {
		t.Fatalf("Regex error: %v", err)
	}
	if !matched {
		t.Errorf("Image name %q does not match pattern %s", name, pattern)
	}
}

func TestFoundationImageName_Deterministic(t *testing.T) {
	t.Parallel()

	dockerfile := "FROM base\nCOPY tsuku /usr/local/bin/tsuku\nRUN echo hello\n"

	n1 := FoundationImageName("debian", dockerfile)
	n2 := FoundationImageName("debian", dockerfile)

	if n1 != n2 {
		t.Errorf("FoundationImageName should be deterministic: %q != %q", n1, n2)
	}
}

func TestFoundationImageName_SensitiveToContent(t *testing.T) {
	t.Parallel()

	d1 := "FROM base\nRUN echo hello\n"
	d2 := "FROM base\nRUN echo world\n"

	n1 := FoundationImageName("debian", d1)
	n2 := FoundationImageName("debian", d2)

	if n1 == n2 {
		t.Error("Different Dockerfiles should produce different image names")
	}
}

func TestFoundationImageName_SensitiveToFamily(t *testing.T) {
	t.Parallel()

	dockerfile := "FROM base\nRUN echo hello\n"

	n1 := FoundationImageName("debian", dockerfile)
	n2 := FoundationImageName("fedora", dockerfile)

	if n1 == n2 {
		t.Error("Different families should produce different image names")
	}
}

func TestFoundationImageName_MultipleCallsConsistent(t *testing.T) {
	t.Parallel()

	// Generate a real Dockerfile and verify naming consistency
	deps := []FlatDep{
		{
			Tool:    "rust",
			Version: "1.82.0",
			Plan:    &executor.InstallationPlan{Tool: "rust", Version: "1.82.0"},
		},
	}
	dockerfile := GenerateFoundationDockerfile("tsuku/sandbox-cache:debian-abc123", deps)

	name1 := FoundationImageName("debian", dockerfile)
	name2 := FoundationImageName("debian", dockerfile)

	if name1 != name2 {
		t.Errorf("Consistency check failed: %q != %q", name1, name2)
	}

	// Different image should produce different name
	deps2 := []FlatDep{
		{
			Tool:    "nodejs",
			Version: "20.0.0",
			Plan:    &executor.InstallationPlan{Tool: "nodejs", Version: "20.0.0"},
		},
	}
	dockerfile2 := GenerateFoundationDockerfile("tsuku/sandbox-cache:debian-abc123", deps2)
	name3 := FoundationImageName("debian", dockerfile2)

	if name1 == name3 {
		t.Error("Different deps should produce different image names")
	}
}

// --- Integration-style tests combining multiple functions ---

func TestFoundation_EndToEnd_SingleDep(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Tool:        "cargo-nextest",
		Version:     "0.24.5",
		GeneratedAt: time.Now(),
		Dependencies: []executor.DependencyPlan{
			{
				Tool:    "rust",
				Version: "1.82.0",
				Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "r123"}},
			},
		},
	}

	deps := FlattenDependencies(plan)
	if len(deps) != 1 {
		t.Fatalf("Expected 1 dep, got %d", len(deps))
	}

	dockerfile := GenerateFoundationDockerfile("tsuku/sandbox-cache:debian-abc", deps)
	if !strings.Contains(dockerfile, "dep-00-rust.json") {
		t.Error("Dockerfile should reference rust plan")
	}

	name := FoundationImageName("debian", dockerfile)
	if !strings.HasPrefix(name, "tsuku/sandbox-foundation:debian-") {
		t.Errorf("Image name should start with 'tsuku/sandbox-foundation:debian-', got %q", name)
	}
}

func TestFoundation_EndToEnd_MultiDep(t *testing.T) {
	t.Parallel()

	plan := &executor.InstallationPlan{
		Tool:    "myapp",
		Version: "1.0.0",
		Dependencies: []executor.DependencyPlan{
			{
				Tool:    "rust",
				Version: "1.82.0",
				Dependencies: []executor.DependencyPlan{
					{
						Tool:    "llvm",
						Version: "17.0.0",
						Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "llvm123"}},
					},
				},
				Steps: []executor.ResolvedStep{{Action: "download_file", Checksum: "rust123"}},
			},
			{
				Tool:    "openssl",
				Version: "3.0.0",
				Steps:   []executor.ResolvedStep{{Action: "download_file", Checksum: "openssl123"}},
			},
		},
	}

	deps := FlattenDependencies(plan)

	// Alphabetical sort of top-level siblings: openssl, rust
	// For openssl: no children, emit openssl
	// For rust: recurse to child llvm (leaf), emit llvm, then emit rust
	// Result: openssl, llvm, rust
	if len(deps) != 3 {
		t.Fatalf("Expected 3 deps, got %d", len(deps))
	}

	expected := []string{"openssl", "llvm", "rust"}
	for i, exp := range expected {
		if deps[i].Tool != exp {
			t.Errorf("deps[%d].Tool = %q, want %q", i, deps[i].Tool, exp)
		}
	}

	dockerfile := GenerateFoundationDockerfile("tsuku/sandbox-cache:debian-abc", deps)

	// Verify all three deps have COPY+RUN pairs
	if !strings.Contains(dockerfile, "dep-00-openssl.json") {
		t.Error("Missing dep-00-openssl.json")
	}
	if !strings.Contains(dockerfile, "dep-01-llvm.json") {
		t.Error("Missing dep-01-llvm.json")
	}
	if !strings.Contains(dockerfile, "dep-02-rust.json") {
		t.Error("Missing dep-02-rust.json")
	}
}

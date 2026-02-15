package batch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- helpers ---

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readQueue(t *testing.T, path string) *UnifiedQueue {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var q UnifiedQueue
	if err := json.Unmarshal(data, &q); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return &q
}

func entryByName(entries []QueueEntry, name string) *QueueEntry {
	for i := range entries {
		if entries[i].Name == name {
			return &entries[i]
		}
	}
	return nil
}

// --- sourceFromStep tests ---

func TestSourceFromStep_AllActions(t *testing.T) {
	tests := []struct {
		name string
		step recipeStepMinimal
		want string
	}{
		{
			name: "homebrew",
			step: recipeStepMinimal{Action: "homebrew", Formula: "gh"},
			want: "homebrew:gh",
		},
		{
			name: "github_archive",
			step: recipeStepMinimal{Action: "github_archive", Repo: "sharkdp/bat"},
			want: "github:sharkdp/bat",
		},
		{
			name: "github_file",
			step: recipeStepMinimal{Action: "github_file", Repo: "junegunn/fzf"},
			want: "github:junegunn/fzf",
		},
		{
			name: "cargo_install",
			step: recipeStepMinimal{Action: "cargo_install", Crate: "ripgrep"},
			want: "cargo:ripgrep",
		},
		{
			name: "npm_install",
			step: recipeStepMinimal{Action: "npm_install", Package: "netlify-cli"},
			want: "npm:netlify-cli",
		},
		{
			name: "pipx_install",
			step: recipeStepMinimal{Action: "pipx_install", Package: "black"},
			want: "pypi:black",
		},
		{
			name: "gem_install",
			step: recipeStepMinimal{Action: "gem_install", Gem: "bundler"},
			want: "rubygems:bundler",
		},
		{
			name: "go_install",
			step: recipeStepMinimal{Action: "go_install", Module: "go.uber.org/mock/mockgen"},
			want: "go:go.uber.org/mock/mockgen",
		},
		{
			name: "nix_install",
			step: recipeStepMinimal{Action: "nix_install", Package: "hello"},
			want: "nix:hello",
		},
		{
			name: "unknown action returns empty",
			step: recipeStepMinimal{Action: "install_binaries"},
			want: "",
		},
		{
			name: "homebrew without formula returns empty",
			step: recipeStepMinimal{Action: "homebrew"},
			want: "",
		},
		{
			name: "download action returns empty",
			step: recipeStepMinimal{Action: "download"},
			want: "",
		},
		{
			name: "cpan_install",
			step: recipeStepMinimal{Action: "cpan_install", Distribution: "ack"},
			want: "cpan:ack",
		},
		{
			name: "download_archive with repo",
			step: recipeStepMinimal{Action: "download_archive", Repo: "golang/go"},
			want: "github:golang/go",
		},
		{
			name: "download_archive without repo returns empty",
			step: recipeStepMinimal{Action: "download_archive"},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sourceFromStep(tt.step)
			if got != tt.want {
				t.Errorf("sourceFromStep() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- recipeToEntry tests ---

func TestRecipeToEntry_HomebrewRecipe(t *testing.T) {
	dir := t.TempDir()
	recipe := filepath.Join(dir, "gh.toml")
	writeFile(t, recipe, `
[metadata]
name = "gh"

[[steps]]
action = "homebrew"
formula = "gh"

[[steps]]
action = "install_binaries"
binaries = ["bin/gh"]

[verify]
command = "gh --version"
`)

	entry, err := recipeToEntry(recipe)
	if err != nil {
		t.Fatalf("recipeToEntry: %v", err)
	}
	if entry.Name != "gh" {
		t.Errorf("Name = %q, want %q", entry.Name, "gh")
	}
	if entry.Source != "homebrew:gh" {
		t.Errorf("Source = %q, want %q", entry.Source, "homebrew:gh")
	}
	if entry.Status != StatusSuccess {
		t.Errorf("Status = %q, want %q", entry.Status, StatusSuccess)
	}
	if entry.Confidence != ConfidenceCurated {
		t.Errorf("Confidence = %q, want %q", entry.Confidence, ConfidenceCurated)
	}
}

func TestRecipeToEntry_GithubArchiveRecipe(t *testing.T) {
	dir := t.TempDir()
	recipe := filepath.Join(dir, "age.toml")
	writeFile(t, recipe, `
[metadata]
name = "age"

[[steps]]
action = "github_archive"
repo = "FiloSottile/age"
asset_pattern = "age-v{version}-{os}-{arch}.tar.gz"

[verify]
command = "age --version"
`)

	entry, err := recipeToEntry(recipe)
	if err != nil {
		t.Fatalf("recipeToEntry: %v", err)
	}
	if entry.Source != "github:FiloSottile/age" {
		t.Errorf("Source = %q, want %q", entry.Source, "github:FiloSottile/age")
	}
}

func TestRecipeToEntry_CargoRecipe(t *testing.T) {
	dir := t.TempDir()
	recipe := filepath.Join(dir, "cargo-audit.toml")
	writeFile(t, recipe, `
[metadata]
name = "cargo-audit"

[[steps]]
action = "cargo_install"
crate = "cargo-audit"

[verify]
command = "cargo-audit --version"
`)

	entry, err := recipeToEntry(recipe)
	if err != nil {
		t.Fatalf("recipeToEntry: %v", err)
	}
	if entry.Source != "cargo:cargo-audit" {
		t.Errorf("Source = %q, want %q", entry.Source, "cargo:cargo-audit")
	}
}

func TestRecipeToEntry_SkipsNonSourceSteps(t *testing.T) {
	// install_binaries and chmod come before the actual source step.
	// The function should skip those and find the github_file step.
	dir := t.TempDir()
	recipe := filepath.Join(dir, "tool.toml")
	writeFile(t, recipe, `
[metadata]
name = "tool"

[[steps]]
action = "download"
url = "https://example.com/tool.tar.gz"

[[steps]]
action = "extract"
archive = "tool.tar.gz"

[[steps]]
action = "chmod"
path = "tool"

[[steps]]
action = "github_file"
repo = "owner/tool"
asset_pattern = "tool-{os}-{arch}"

[[steps]]
action = "install_binaries"
binaries = ["tool"]

[verify]
command = "tool --version"
`)

	entry, err := recipeToEntry(recipe)
	if err != nil {
		t.Fatalf("recipeToEntry: %v", err)
	}
	if entry.Source != "github:owner/tool" {
		t.Errorf("Source = %q, want %q", entry.Source, "github:owner/tool")
	}
}

func TestRecipeToEntry_FallbackToVersionGitHubRepo(t *testing.T) {
	// Recipes using generic download/extract but with version.github_repo
	// should use the github_repo as the source.
	dir := t.TempDir()
	recipe := filepath.Join(dir, "terraform.toml")
	writeFile(t, recipe, `
[metadata]
name = "terraform"

[version]
github_repo = "hashicorp/terraform"

[[steps]]
action = "download"
url = "https://releases.hashicorp.com/terraform/{version}/terraform_{version}_{os}_{arch}.zip"

[[steps]]
action = "extract"
archive = "terraform.zip"

[[steps]]
action = "install_binaries"
outputs = ["terraform"]

[verify]
command = "terraform version"
`)

	entry, err := recipeToEntry(recipe)
	if err != nil {
		t.Fatalf("recipeToEntry: %v", err)
	}
	if entry.Source != "github:hashicorp/terraform" {
		t.Errorf("Source = %q, want %q", entry.Source, "github:hashicorp/terraform")
	}
}

func TestRecipeToEntry_CpanRecipe(t *testing.T) {
	dir := t.TempDir()
	recipe := filepath.Join(dir, "ack.toml")
	writeFile(t, recipe, `
[metadata]
name = "ack"

[[steps]]
action = "cpan_install"
distribution = "ack"

[verify]
command = "ack --version"
`)

	entry, err := recipeToEntry(recipe)
	if err != nil {
		t.Fatalf("recipeToEntry: %v", err)
	}
	if entry.Source != "cpan:ack" {
		t.Errorf("Source = %q, want %q", entry.Source, "cpan:ack")
	}
}

func TestRecipeToEntry_NoSourceReturnsError(t *testing.T) {
	dir := t.TempDir()
	recipe := filepath.Join(dir, "tool.toml")
	writeFile(t, recipe, `
[metadata]
name = "tool"

[[steps]]
action = "download"
url = "https://example.com/tool.tar.gz"

[[steps]]
action = "install_binaries"
binaries = ["tool"]

[verify]
command = "tool --version"
`)

	_, err := recipeToEntry(recipe)
	if err == nil {
		t.Fatal("expected error for recipe with no source step")
	}
}

func TestRecipeToEntry_NoNameReturnsError(t *testing.T) {
	dir := t.TempDir()
	recipe := filepath.Join(dir, "bad.toml")
	writeFile(t, recipe, `
[metadata]

[[steps]]
action = "homebrew"
formula = "bad"

[verify]
command = "bad --version"
`)

	_, err := recipeToEntry(recipe)
	if err == nil {
		t.Fatal("expected error for recipe without name")
	}
}

// --- scanRecipes tests ---

func TestScanRecipes_MultipleRecipes(t *testing.T) {
	dir := t.TempDir()

	// Create recipes in subdirectories (like recipes/g/gh.toml)
	writeFile(t, filepath.Join(dir, "g", "gh.toml"), `
[metadata]
name = "gh"
[[steps]]
action = "homebrew"
formula = "gh"
[verify]
command = "gh --version"
`)

	writeFile(t, filepath.Join(dir, "a", "age.toml"), `
[metadata]
name = "age"
[[steps]]
action = "github_archive"
repo = "FiloSottile/age"
[verify]
command = "age --version"
`)

	entries, err := scanRecipes(dir)
	if err != nil {
		t.Fatalf("scanRecipes: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}

	// All recipe entries should have status: success, confidence: curated
	for _, e := range entries {
		if e.Status != StatusSuccess {
			t.Errorf("%s: Status = %q, want %q", e.Name, e.Status, StatusSuccess)
		}
		if e.Confidence != ConfidenceCurated {
			t.Errorf("%s: Confidence = %q, want %q", e.Name, e.Confidence, ConfidenceCurated)
		}
	}
}

func TestScanRecipes_SkipsNonTomlFiles(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "README.md"), "# Recipes")
	writeFile(t, filepath.Join(dir, "CLAUDE.local.md"), "# Context")
	writeFile(t, filepath.Join(dir, "g", "gh.toml"), `
[metadata]
name = "gh"
[[steps]]
action = "homebrew"
formula = "gh"
[verify]
command = "gh --version"
`)

	entries, err := scanRecipes(dir)
	if err != nil {
		t.Fatalf("scanRecipes: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (should skip non-TOML files)", len(entries))
	}
}

func TestScanRecipes_SkipsBadRecipes(t *testing.T) {
	dir := t.TempDir()

	// Valid recipe
	writeFile(t, filepath.Join(dir, "good.toml"), `
[metadata]
name = "good"
[[steps]]
action = "homebrew"
formula = "good"
[verify]
command = "good --version"
`)

	// Invalid TOML
	writeFile(t, filepath.Join(dir, "bad.toml"), `not valid toml {{{{`)

	entries, err := scanRecipes(dir)
	if err != nil {
		t.Fatalf("scanRecipes: %v", err)
	}
	// Should still get the valid recipe; bad one is skipped with a warning
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (bad recipe should be skipped)", len(entries))
	}
	if entries[0].Name != "good" {
		t.Errorf("Name = %q, want %q", entries[0].Name, "good")
	}
}

// --- parseCurated tests ---

func TestParseCurated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "curated.jsonl")
	writeFile(t, path, `{
		"schema_version": 1,
		"ecosystem": "curated",
		"environment": "manual-selection",
		"updated_at": "2026-02-14T14:17:07Z",
		"disambiguations": [
			{"tool": "bat", "selected": "github:sharkdp/bat", "alternatives": [], "selection_reason": "curated", "high_risk": false},
			{"tool": "jq", "selected": "github:jqlang/jq", "alternatives": [], "selection_reason": "curated", "high_risk": false}
		]
	}`)

	entries, err := parseCurated(path)
	if err != nil {
		t.Fatalf("parseCurated: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}

	bat := entryByName(entries, "bat")
	if bat == nil {
		t.Fatal("missing entry for bat")
	}
	if bat.Source != "github:sharkdp/bat" {
		t.Errorf("bat Source = %q, want %q", bat.Source, "github:sharkdp/bat")
	}
	if bat.Status != StatusPending {
		t.Errorf("bat Status = %q, want %q", bat.Status, StatusPending)
	}
	if bat.Confidence != ConfidenceCurated {
		t.Errorf("bat Confidence = %q, want %q", bat.Confidence, ConfidenceCurated)
	}
	if bat.Priority != 1 {
		t.Errorf("bat Priority = %d, want 1", bat.Priority)
	}
}

func TestParseCurated_SkipsEmptyFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "curated.jsonl")
	writeFile(t, path, `{
		"schema_version": 1,
		"disambiguations": [
			{"tool": "bat", "selected": "github:sharkdp/bat"},
			{"tool": "", "selected": "github:empty/name"},
			{"tool": "noselected", "selected": ""}
		]
	}`)

	entries, err := parseCurated(path)
	if err != nil {
		t.Fatalf("parseCurated: %v", err)
	}
	// Only the first one should be included; the others have empty tool/selected
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Name != "bat" {
		t.Errorf("Name = %q, want %q", entries[0].Name, "bat")
	}
}

// --- parseHomebrew tests ---

func TestParseHomebrew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.json")
	writeFile(t, path, `{
		"schema_version": 1,
		"updated_at": "2026-02-15T04:57:24Z",
		"packages": [
			{"id": "homebrew:awscli", "source": "homebrew", "name": "awscli", "tier": 2, "status": "failed", "added_at": "2026-02-07T02:32:02Z"},
			{"id": "homebrew:gh", "source": "homebrew", "name": "gh", "tier": 1, "status": "success", "added_at": "2026-02-07T02:32:02Z"},
			{"id": "homebrew:node", "source": "homebrew", "name": "node", "tier": 1, "status": "blocked", "added_at": "2026-02-07T02:32:02Z"}
		]
	}`)

	entries, err := parseHomebrew(path)
	if err != nil {
		t.Fatalf("parseHomebrew: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}

	// All homebrew entries should get status: pending, confidence: auto
	for _, e := range entries {
		if e.Status != StatusPending {
			t.Errorf("%s: Status = %q, want %q", e.Name, e.Status, StatusPending)
		}
		if e.Confidence != ConfidenceAuto {
			t.Errorf("%s: Confidence = %q, want %q", e.Name, e.Confidence, ConfidenceAuto)
		}
		if !containsStr(e.Source, "homebrew:") {
			t.Errorf("%s: Source = %q, should start with homebrew:", e.Name, e.Source)
		}
	}

	// Check priority preserved from tier
	gh := entryByName(entries, "gh")
	if gh == nil {
		t.Fatal("missing entry for gh")
	}
	if gh.Priority != 1 {
		t.Errorf("gh Priority = %d, want 1", gh.Priority)
	}

	awscli := entryByName(entries, "awscli")
	if awscli == nil {
		t.Fatal("missing entry for awscli")
	}
	if awscli.Priority != 2 {
		t.Errorf("awscli Priority = %d, want 2", awscli.Priority)
	}
}

func TestParseHomebrew_ClampsBadTier(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.json")
	writeFile(t, path, `{
		"schema_version": 1,
		"packages": [
			{"id": "homebrew:bad", "source": "homebrew", "name": "bad", "tier": 0, "status": "pending", "added_at": "2026-02-07T02:32:02Z"},
			{"id": "homebrew:worse", "source": "homebrew", "name": "worse", "tier": 5, "status": "pending", "added_at": "2026-02-07T02:32:02Z"}
		]
	}`)

	entries, err := parseHomebrew(path)
	if err != nil {
		t.Fatalf("parseHomebrew: %v", err)
	}
	for _, e := range entries {
		if e.Priority != 3 {
			t.Errorf("%s: Priority = %d, want 3 (clamped)", e.Name, e.Priority)
		}
	}
}

// --- Bootstrap integration tests ---

func TestBootstrap_FullMigration(t *testing.T) {
	dir := t.TempDir()
	recipesDir := filepath.Join(dir, "recipes")
	curatedPath := filepath.Join(dir, "curated.jsonl")
	homebrewPath := filepath.Join(dir, "homebrew.json")
	outputPath := filepath.Join(dir, "output", "priority-queue.json")

	// Recipes: gh (homebrew), age (github)
	writeFile(t, filepath.Join(recipesDir, "g", "gh.toml"), `
[metadata]
name = "gh"
[[steps]]
action = "homebrew"
formula = "gh"
[verify]
command = "gh --version"
`)
	writeFile(t, filepath.Join(recipesDir, "a", "age.toml"), `
[metadata]
name = "age"
[[steps]]
action = "github_archive"
repo = "FiloSottile/age"
[verify]
command = "age --version"
`)

	// Curated: bat (new), gh (duplicate with recipe)
	writeFile(t, curatedPath, `{
		"schema_version": 1,
		"disambiguations": [
			{"tool": "bat", "selected": "github:sharkdp/bat", "selection_reason": "curated"},
			{"tool": "gh", "selected": "github:cli/cli", "selection_reason": "curated"}
		]
	}`)

	// Homebrew: gh (duplicate), bat (duplicate), node (new), awscli (new)
	writeFile(t, homebrewPath, `{
		"schema_version": 1,
		"packages": [
			{"id": "homebrew:gh", "source": "homebrew", "name": "gh", "tier": 1, "status": "success", "added_at": "2026-02-07T02:32:02Z"},
			{"id": "homebrew:bat", "source": "homebrew", "name": "bat", "tier": 2, "status": "failed", "added_at": "2026-02-07T02:32:02Z"},
			{"id": "homebrew:node", "source": "homebrew", "name": "node", "tier": 1, "status": "blocked", "added_at": "2026-02-07T02:32:02Z"},
			{"id": "homebrew:awscli", "source": "homebrew", "name": "awscli", "tier": 2, "status": "pending", "added_at": "2026-02-07T02:32:02Z"}
		]
	}`)

	cfg := BootstrapConfig{
		RecipesDir:   recipesDir,
		CuratedPath:  curatedPath,
		HomebrewPath: homebrewPath,
		OutputPath:   outputPath,
	}

	result, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	// 2 recipes, 1 curated (bat; gh is a duplicate), 2 homebrew (node, awscli)
	if result.RecipeEntries != 2 {
		t.Errorf("RecipeEntries = %d, want 2", result.RecipeEntries)
	}
	if result.CuratedEntries != 1 {
		t.Errorf("CuratedEntries = %d, want 1", result.CuratedEntries)
	}
	if result.HomebrewEntries != 2 {
		t.Errorf("HomebrewEntries = %d, want 2", result.HomebrewEntries)
	}
	if result.TotalEntries != 5 {
		t.Errorf("TotalEntries = %d, want 5", result.TotalEntries)
	}

	// Read and verify the output file
	q := readQueue(t, outputPath)
	if q.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", q.SchemaVersion)
	}
	if len(q.Entries) != 5 {
		t.Fatalf("len(Entries) = %d, want 5", len(q.Entries))
	}

	// Verify precedence: gh came from recipe (homebrew:gh), not curated (github:cli/cli)
	gh := entryByName(q.Entries, "gh")
	if gh == nil {
		t.Fatal("missing entry for gh")
	}
	if gh.Source != "homebrew:gh" {
		t.Errorf("gh Source = %q, want %q (from recipe, not curated)", gh.Source, "homebrew:gh")
	}
	if gh.Status != StatusSuccess {
		t.Errorf("gh Status = %q, want %q", gh.Status, StatusSuccess)
	}

	// Verify bat came from curated (github:sharkdp/bat), not homebrew
	bat := entryByName(q.Entries, "bat")
	if bat == nil {
		t.Fatal("missing entry for bat")
	}
	if bat.Source != "github:sharkdp/bat" {
		t.Errorf("bat Source = %q, want %q (from curated, not homebrew)", bat.Source, "github:sharkdp/bat")
	}
	if bat.Status != StatusPending {
		t.Errorf("bat Status = %q, want %q", bat.Status, StatusPending)
	}
	if bat.Confidence != ConfidenceCurated {
		t.Errorf("bat Confidence = %q, want %q", bat.Confidence, ConfidenceCurated)
	}

	// Verify node came from homebrew (not in recipes or curated)
	node := entryByName(q.Entries, "node")
	if node == nil {
		t.Fatal("missing entry for node")
	}
	if node.Source != "homebrew:node" {
		t.Errorf("node Source = %q, want %q", node.Source, "homebrew:node")
	}
	if node.Confidence != ConfidenceAuto {
		t.Errorf("node Confidence = %q, want %q", node.Confidence, ConfidenceAuto)
	}
}

func TestBootstrap_OutputSortedByPriorityThenName(t *testing.T) {
	dir := t.TempDir()
	recipesDir := filepath.Join(dir, "recipes")
	curatedPath := filepath.Join(dir, "curated.jsonl")
	homebrewPath := filepath.Join(dir, "homebrew.json")
	outputPath := filepath.Join(dir, "output", "priority-queue.json")

	// Empty recipes and curated
	if err := os.MkdirAll(recipesDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, curatedPath, `{"schema_version": 1, "disambiguations": []}`)

	// Homebrew with various tiers
	writeFile(t, homebrewPath, `{
		"schema_version": 1,
		"packages": [
			{"id": "homebrew:zsh", "source": "homebrew", "name": "zsh", "tier": 3, "status": "pending", "added_at": "2026-02-07T02:32:02Z"},
			{"id": "homebrew:gh", "source": "homebrew", "name": "gh", "tier": 1, "status": "pending", "added_at": "2026-02-07T02:32:02Z"},
			{"id": "homebrew:bat", "source": "homebrew", "name": "bat", "tier": 2, "status": "pending", "added_at": "2026-02-07T02:32:02Z"},
			{"id": "homebrew:act", "source": "homebrew", "name": "act", "tier": 1, "status": "pending", "added_at": "2026-02-07T02:32:02Z"},
			{"id": "homebrew:age", "source": "homebrew", "name": "age", "tier": 2, "status": "pending", "added_at": "2026-02-07T02:32:02Z"}
		]
	}`)

	cfg := BootstrapConfig{
		RecipesDir:   recipesDir,
		CuratedPath:  curatedPath,
		HomebrewPath: homebrewPath,
		OutputPath:   outputPath,
	}

	_, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	q := readQueue(t, outputPath)
	if len(q.Entries) != 5 {
		t.Fatalf("len(Entries) = %d, want 5", len(q.Entries))
	}

	// Expected order: priority 1 (act, gh), priority 2 (age, bat), priority 3 (zsh)
	expected := []struct {
		name     string
		priority int
	}{
		{"act", 1}, {"gh", 1}, {"age", 2}, {"bat", 2}, {"zsh", 3},
	}
	for i, exp := range expected {
		if q.Entries[i].Name != exp.name {
			t.Errorf("Entries[%d].Name = %q, want %q", i, q.Entries[i].Name, exp.name)
		}
		if q.Entries[i].Priority != exp.priority {
			t.Errorf("Entries[%d].Priority = %d, want %d", i, q.Entries[i].Priority, exp.priority)
		}
	}
}

func TestBootstrap_EmptyInputs(t *testing.T) {
	dir := t.TempDir()
	recipesDir := filepath.Join(dir, "recipes")
	curatedPath := filepath.Join(dir, "curated.jsonl")
	homebrewPath := filepath.Join(dir, "homebrew.json")
	outputPath := filepath.Join(dir, "output", "priority-queue.json")

	if err := os.MkdirAll(recipesDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, curatedPath, `{"schema_version": 1, "disambiguations": []}`)
	writeFile(t, homebrewPath, `{"schema_version": 1, "packages": []}`)

	cfg := BootstrapConfig{
		RecipesDir:   recipesDir,
		CuratedPath:  curatedPath,
		HomebrewPath: homebrewPath,
		OutputPath:   outputPath,
	}

	result, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if result.TotalEntries != 0 {
		t.Errorf("TotalEntries = %d, want 0", result.TotalEntries)
	}

	q := readQueue(t, outputPath)
	if len(q.Entries) != 0 {
		t.Errorf("len(Entries) = %d, want 0", len(q.Entries))
	}
}

func TestBootstrap_MissingCuratedFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	recipesDir := filepath.Join(dir, "recipes")
	if err := os.MkdirAll(recipesDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := BootstrapConfig{
		RecipesDir:   recipesDir,
		CuratedPath:  filepath.Join(dir, "nonexistent.jsonl"),
		HomebrewPath: filepath.Join(dir, "homebrew.json"),
		OutputPath:   filepath.Join(dir, "output.json"),
	}

	_, err := Bootstrap(cfg)
	if err == nil {
		t.Fatal("expected error for missing curated file")
	}
}

func TestBootstrap_MissingHomebrewFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	recipesDir := filepath.Join(dir, "recipes")
	curatedPath := filepath.Join(dir, "curated.jsonl")
	if err := os.MkdirAll(recipesDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, curatedPath, `{"schema_version": 1, "disambiguations": []}`)

	cfg := BootstrapConfig{
		RecipesDir:   recipesDir,
		CuratedPath:  curatedPath,
		HomebrewPath: filepath.Join(dir, "nonexistent.json"),
		OutputPath:   filepath.Join(dir, "output.json"),
	}

	_, err := Bootstrap(cfg)
	if err == nil {
		t.Fatal("expected error for missing homebrew file")
	}
}

func TestBootstrap_OutputIsValidJSON(t *testing.T) {
	dir := t.TempDir()
	recipesDir := filepath.Join(dir, "recipes")
	curatedPath := filepath.Join(dir, "curated.jsonl")
	homebrewPath := filepath.Join(dir, "homebrew.json")
	outputPath := filepath.Join(dir, "output", "priority-queue.json")

	writeFile(t, filepath.Join(recipesDir, "t", "tool.toml"), `
[metadata]
name = "tool"
[[steps]]
action = "homebrew"
formula = "tool"
[verify]
command = "tool --version"
`)
	writeFile(t, curatedPath, `{"schema_version": 1, "disambiguations": []}`)
	writeFile(t, homebrewPath, `{"schema_version": 1, "packages": []}`)

	cfg := BootstrapConfig{
		RecipesDir:   recipesDir,
		CuratedPath:  curatedPath,
		HomebrewPath: homebrewPath,
		OutputPath:   outputPath,
	}

	_, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	// Read raw JSON and verify it round-trips cleanly
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Verify expected top-level fields
	for _, field := range []string{"schema_version", "updated_at", "entries"} {
		if _, ok := raw[field]; !ok {
			t.Errorf("missing top-level field %q", field)
		}
	}
}

func TestBootstrap_AllEntriesValidate(t *testing.T) {
	dir := t.TempDir()
	recipesDir := filepath.Join(dir, "recipes")
	curatedPath := filepath.Join(dir, "curated.jsonl")
	homebrewPath := filepath.Join(dir, "homebrew.json")
	outputPath := filepath.Join(dir, "output", "priority-queue.json")

	writeFile(t, filepath.Join(recipesDir, "g", "gh.toml"), `
[metadata]
name = "gh"
[[steps]]
action = "homebrew"
formula = "gh"
[verify]
command = "gh --version"
`)
	writeFile(t, curatedPath, `{
		"schema_version": 1,
		"disambiguations": [
			{"tool": "bat", "selected": "github:sharkdp/bat", "selection_reason": "curated"}
		]
	}`)
	writeFile(t, homebrewPath, `{
		"schema_version": 1,
		"packages": [
			{"id": "homebrew:node", "source": "homebrew", "name": "node", "tier": 1, "status": "blocked", "added_at": "2026-02-07T02:32:02Z"}
		]
	}`)

	cfg := BootstrapConfig{
		RecipesDir:   recipesDir,
		CuratedPath:  curatedPath,
		HomebrewPath: homebrewPath,
		OutputPath:   outputPath,
	}

	_, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	q := readQueue(t, outputPath)
	for _, entry := range q.Entries {
		if err := entry.Validate(); err != nil {
			t.Errorf("entry %q fails validation: %v", entry.Name, err)
		}
	}
}

func TestBootstrap_RecipePriorityOverriddenByHomebrew(t *testing.T) {
	// When a recipe entry also exists in the homebrew queue, the recipe
	// entry wins (from phase 1). We set default priority 3 for recipes.
	// This verifies that recipe entries get the default priority.
	dir := t.TempDir()
	recipesDir := filepath.Join(dir, "recipes")
	curatedPath := filepath.Join(dir, "curated.jsonl")
	homebrewPath := filepath.Join(dir, "homebrew.json")
	outputPath := filepath.Join(dir, "output", "priority-queue.json")

	writeFile(t, filepath.Join(recipesDir, "g", "gh.toml"), `
[metadata]
name = "gh"
[[steps]]
action = "homebrew"
formula = "gh"
[verify]
command = "gh --version"
`)
	writeFile(t, curatedPath, `{"schema_version": 1, "disambiguations": []}`)
	writeFile(t, homebrewPath, `{
		"schema_version": 1,
		"packages": [
			{"id": "homebrew:gh", "source": "homebrew", "name": "gh", "tier": 1, "status": "success", "added_at": "2026-02-07T02:32:02Z"}
		]
	}`)

	cfg := BootstrapConfig{
		RecipesDir:   recipesDir,
		CuratedPath:  curatedPath,
		HomebrewPath: homebrewPath,
		OutputPath:   outputPath,
	}

	result, err := Bootstrap(cfg)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	// Recipe takes precedence; homebrew duplicate is skipped
	if result.RecipeEntries != 1 {
		t.Errorf("RecipeEntries = %d, want 1", result.RecipeEntries)
	}
	if result.HomebrewEntries != 0 {
		t.Errorf("HomebrewEntries = %d, want 0 (gh is a duplicate)", result.HomebrewEntries)
	}
	if result.TotalEntries != 1 {
		t.Errorf("TotalEntries = %d, want 1", result.TotalEntries)
	}

	q := readQueue(t, outputPath)
	gh := entryByName(q.Entries, "gh")
	if gh == nil {
		t.Fatal("missing entry for gh")
	}
	// Recipe entry has default priority 3
	if gh.Priority != 3 {
		t.Errorf("gh Priority = %d, want 3 (recipe default)", gh.Priority)
	}
}

// containsStr checks if s contains substr.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

package batch

import (
	"os"
	"path/filepath"
	"testing"
)

// writeRecipe writes a recipe file at dir/<first letter>/<name>.toml.
func writeRecipe(t *testing.T, dir, name, body string) {
	t.Helper()
	sub := filepath.Join(dir, name[:1])
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, name+".toml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func reconcileFixture(t *testing.T) []RecipeIdentity {
	t.Helper()
	dir := t.TempDir()
	// Same formula as the queue entry "task", different name.
	writeRecipe(t, dir, "go-task", `
[metadata]
name = "go-task"
[[steps]]
action = "homebrew"
formula = "go-task"
`)
	// Declares an alias.
	writeRecipe(t, dir, "ripgrep", `
[metadata]
name = "ripgrep"
[metadata.satisfies]
aliases = ["rg"]
[[steps]]
action = "github_archive"
repo = "BurntSushi/ripgrep"
`)
	// Declares a Homebrew ecosystem key.
	writeRecipe(t, dir, "openssl", `
[metadata]
name = "openssl"
[metadata.satisfies]
homebrew = ["openssl@3"]
[[steps]]
action = "download_archive"
url = "https://example.invalid/openssl.tar.gz"
`)
	// Installed with cargo_install: recorded as cargo:, queued as crates.io:.
	writeRecipe(t, dir, "bindgen", `
[metadata]
name = "bindgen"
[[steps]]
action = "cargo_install"
crate = "bindgen-cli"
`)
	// Installs a binary named "parallel"; binaries must not identify a tool.
	writeRecipe(t, dir, "moreutils", `
[metadata]
name = "moreutils"
[[steps]]
action = "homebrew"
formula = "moreutils"
[[steps]]
action = "install_binaries"
binaries = ["bin/parallel"]
`)
	// Same name as a queue entry but a different source.
	writeRecipe(t, dir, "jq", `
[metadata]
name = "jq"
[[steps]]
action = "github_archive"
repo = "jqlang/jq"
`)
	ids, warnings, err := ScanRecipeIdentities(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(ids) != 6 {
		t.Fatalf("scanned %d recipes, want 6", len(ids))
	}
	return ids
}

func TestReconcile_Rules(t *testing.T) {
	recipes := reconcileFixture(t)

	tests := []struct {
		name       string
		entry      QueueEntry
		wantStatus string
		wantRule   string // "" when the entry must be left alone
		wantRecipe string
	}{
		{"same source, other name", QueueEntry{Name: "task", Source: "homebrew:go-task", Status: StatusPending}, StatusSuccess, RuleSource, "go-task"},
		{"alias", QueueEntry{Name: "rg", Source: "github:someone/else", Status: StatusPending}, StatusSuccess, RuleAlias, "ripgrep"},
		{"satisfies ecosystem key", QueueEntry{Name: "openssl@3", Source: "homebrew:openssl@3", Status: StatusFailed}, StatusSuccess, RuleSatisfies, "openssl"},
		{"cargo and crates.io are one ecosystem", QueueEntry{Name: "bindgen-cli", Source: "crates.io:bindgen-cli", Status: StatusPending}, StatusSuccess, RuleSource, "bindgen"},
		{"cargo entry matches cargo recipe", QueueEntry{Name: "bindgen-cli", Source: "cargo:bindgen-cli", Status: StatusPending}, StatusSuccess, RuleSource, "bindgen"},
		{"blocked counts as open", QueueEntry{Name: "task2", Source: "homebrew:go-task", Status: StatusBlocked}, StatusSuccess, RuleSource, "go-task"},
		{"name match", QueueEntry{Name: "jq", Source: "homebrew:jq", Status: StatusPending}, StatusSuccess, RuleName, "jq"},
		{"binary name is not an identity", QueueEntry{Name: "parallel", Source: "homebrew:parallel", Status: StatusPending}, StatusPending, "", ""},
		{"satisfies key in another ecosystem", QueueEntry{Name: "openssl@3", Source: "npm:openssl@3", Status: StatusPending}, StatusPending, "", ""},
		{"excluded stays excluded", QueueEntry{Name: "task", Source: "homebrew:go-task", Status: StatusExcluded}, StatusExcluded, "", ""},
		{"uncovered stays pending", QueueEntry{Name: "hub", Source: "github:github/hub", Status: StatusPending}, StatusPending, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &UnifiedQueue{Entries: []QueueEntry{tt.entry}}
			res := Reconcile(q, recipes)
			if got := q.Entries[0].Status; got != tt.wantStatus {
				t.Errorf("status = %q, want %q", got, tt.wantStatus)
			}
			if tt.wantRule == "" {
				if len(res.Changes) != 0 {
					t.Errorf("changes = %+v, want none", res.Changes)
				}
				return
			}
			if len(res.Changes) != 1 {
				t.Fatalf("changes = %+v, want one", res.Changes)
			}
			c := res.Changes[0]
			if c.Rule != tt.wantRule || c.Recipe != tt.wantRecipe {
				t.Errorf("change = rule %q recipe %q, want rule %q recipe %q", c.Rule, c.Recipe, tt.wantRule, tt.wantRecipe)
			}
			if c.FromStatus != tt.entry.Status {
				t.Errorf("from_status = %q, want %q", c.FromStatus, tt.entry.Status)
			}
		})
	}
}

func TestReconcile_NameMatchWithOtherSourceBecomesCurated(t *testing.T) {
	recipes := reconcileFixture(t)
	q := &UnifiedQueue{Entries: []QueueEntry{
		{Name: "jq", Source: "homebrew:jq", Status: StatusPending, Confidence: ConfidenceAuto},
		{Name: "go-task", Source: "homebrew:go-task", Status: StatusPending, Confidence: ConfidenceAuto},
	}}
	Reconcile(q, recipes)
	if got := q.Entries[0].Confidence; got != ConfidenceCurated {
		t.Errorf("jq confidence = %q, want curated (recipe source differs)", got)
	}
	if got := q.Entries[1].Confidence; got != ConfidenceAuto {
		t.Errorf("go-task confidence = %q, want auto (recipe source matches)", got)
	}
}

// The result is what a reader of the job log sees, so it has to agree with
// what actually changed in the queue.
func TestReconcile_ReportMatchesQueue(t *testing.T) {
	recipes := reconcileFixture(t)
	q := &UnifiedQueue{Entries: []QueueEntry{
		{Name: "task", Source: "homebrew:go-task", Status: StatusPending},
		{Name: "rg", Source: "github:BurntSushi/ripgrep", Status: StatusFailed},
		{Name: "hub", Source: "github:github/hub", Status: StatusPending},
		{Name: "go-task", Source: "homebrew:go-task", Status: StatusSuccess},
		{Name: "conduit", Source: "rubygems:conduit", Status: StatusExcluded},
	}}
	before := make([]string, len(q.Entries))
	for i, e := range q.Entries {
		before[i] = e.Status
	}

	res := Reconcile(q, recipes)

	flipped := 0
	for i, e := range q.Entries {
		if before[i] != e.Status {
			flipped++
		}
	}
	if res.Recipes != len(recipes) {
		t.Errorf("Recipes = %d, want %d", res.Recipes, len(recipes))
	}
	if res.Open != 3 {
		t.Errorf("Open = %d, want 3 (task, rg, hub)", res.Open)
	}
	if res.Reconciled != 2 || len(res.Changes) != 2 || flipped != 2 {
		t.Errorf("Reconciled = %d, len(Changes) = %d, statuses flipped = %d; want 2, 2, 2", res.Reconciled, len(res.Changes), flipped)
	}
}

func TestScanRecipeIdentities_WarnsAndSkipsMissingDir(t *testing.T) {
	dir := t.TempDir()
	writeRecipe(t, dir, "good", "[metadata]\nname = \"good\"\n[[steps]]\naction = \"homebrew\"\nformula = \"good\"\n")
	writeRecipe(t, dir, "broken", "this is not toml [[[")
	writeRecipe(t, dir, "nameless", "[metadata]\n[[steps]]\naction = \"homebrew\"\nformula = \"x\"\n")

	ids, warnings, err := ScanRecipeIdentities(dir, filepath.Join(dir, "does-not-exist"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0].Name != "good" || ids[0].Source != "homebrew:good" {
		t.Errorf("ids = %+v, want only good (homebrew:good)", ids)
	}
	if len(warnings) != 2 {
		t.Errorf("warnings = %v, want 2 (broken, nameless)", warnings)
	}
}

func TestNormalizeSource(t *testing.T) {
	for in, want := range map[string]string{
		"cargo:ripgrep":     "crates.io:ripgrep",
		"crates.io:ripgrep": "crates.io:ripgrep",
		"homebrew:cargo":    "homebrew:cargo",
		"github:a/b":        "github:a/b",
	} {
		if got := NormalizeSource(in); got != want {
			t.Errorf("NormalizeSource(%q) = %q, want %q", in, got, want)
		}
	}
}

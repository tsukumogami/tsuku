# Version Resolution for Unified Release Tagging
Research Round 1: Lead - Version Resolution Investigation

## Summary
- Both tsuku-dltest (lines 9-10 of tsuku-dltest.toml) and tsuku-llm (no [version] section) currently lack explicit version configuration
- The system has a working GitHubProvider that can resolve versions from GitHub tags using `github_repo` + optional `tag_prefix` fields
- tsuku-dltest can immediately support unified versioning by adding `[version] github_repo = "tsukumogami/tsuku"` (already uses correct `tag_prefix = "v"`)

---

## Current State: Recipe Version Resolution

### tsuku-dltest Recipe
**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/t/tsuku-dltest.toml`

```toml
[version]
tag_prefix = "v"

[[steps]]
action = "github_file"
repo = "tsukumogami/tsuku"
asset_pattern = "tsuku-dltest-{os}-{arch}"
```

**Current Behavior:**
- Has `tag_prefix = "v"` configured
- Downloads from `tsukumogami/tsuku` repo
- **Missing:** No explicit `[version]` source or `github_repo` field
- Falls back to InferredGitHubStrategy (priority 10) which infers from the `github_file` action

### tsuku-llm Recipe
**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/recipes/t/tsuku-llm.toml`

```toml
[metadata]
name = "tsuku-llm"
# ... no [version] section

[[steps]]
action = "github_file"
when = { os = ["darwin"], arch = "arm64" }
repo = "tsukumogami/tsuku-llm"
asset_pattern = "tsuku-llm-v{version}-darwin-arm64"
```

**Current Behavior:**
- Has NO `[version]` section at all
- Downloads from separate `tsukumogami/tsuku-llm` repo
- Also falls back to InferredGitHubStrategy
- Uses `tsuku-llm-v{version}` prefix in asset_pattern (different from tsuku-dltest)

---

## Version Provider System Architecture

### Factory & Strategy Pattern
**File:** `internal/version/provider_factory.go`

The system uses a priority-ordered strategy pattern:

| Priority | Strategy | Condition |
|----------|----------|-----------|
| 100 (KnownRegistry) | PyPI, CratesIO, RubyGems, npm, Nixpkgs, etc. | `source = "registry_name"` |
| 90 (ExplicitHint) | **GitHubRepoStrategy** | `github_repo` field set |
| 80 (ExplicitSource) | CustomProvider | `source = "custom_name"` |
| 10 (Inferred) | InferredGitHubStrategy | `github_archive` or `github_file` action |

Key insight: `GitHubRepoStrategy` (priority 90) handles `github_repo` + `tag_prefix`, but only if explicitly set.

### GitHubProvider Implementation
**File:** `internal/version/provider_github.go` (lines 1-145)

```go
type GitHubProvider struct {
	resolver  *Resolver
	repo      string // owner/repo format
	tagPrefix string // optional prefix to filter tags
}

// NewGitHubProviderWithPrefix creates a provider that filters tags by prefix
// The prefix is stripped from version strings (e.g., "ruby-3.3.10" -> "3.3.10")
func NewGitHubProviderWithPrefix(resolver *Resolver, repo, tagPrefix string) *GitHubProvider
```

**Version Resolution Flow:**
1. `ListVersions()` → calls `resolver.ListGitHubVersions(ctx, repo)`
   - Fetches all tags from GitHub
   - If `tagPrefix` set, filters and strips: `"v1.2.3"` → `"1.2.3"`
2. `ResolveLatest()` → returns first stable version (skips preview/alpha/beta/rc)
3. `ResolveVersion(ctx, "1.2")` → fuzzy matches (exact, then prefix match)

### GitHubRepoStrategy
**File:** `internal/version/provider_factory.go` (lines 158-172)

```go
type GitHubRepoStrategy struct{}

func (s *GitHubRepoStrategy) CanHandle(r *recipe.Recipe) bool {
	return r.Version.GitHubRepo != ""
}

func (s *GitHubRepoStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
	if r.Version.TagPrefix != "" {
		return NewGitHubProviderWithPrefix(resolver, r.Version.GitHubRepo, r.Version.TagPrefix), nil
	}
	return NewGitHubProvider(resolver, r.Version.GitHubRepo), nil
}
```

**Activation:** Only runs when `Version.GitHubRepo` is non-empty.

### Inferred GitHub Strategy (Current Fallback)
**File:** `internal/version/provider_factory.go` (lines 174-199)

```go
type InferredGitHubStrategy struct{}

func (s *InferredGitHubStrategy) CanHandle(r *recipe.Recipe) bool {
	for _, step := range r.Steps {
		if step.Action == "github_archive" || step.Action == "github_file" {
			if _, ok := step.Params["repo"].(string); ok {
				return true
			}
		}
	}
	return false
}
```

**Current State:**
- Extracts `repo` from `github_file` action
- Does NOT respect `tag_prefix` from recipe's `[version]` section
- Has lower priority (10) than explicit configuration (90)

---

## Release Workflow Analysis

### Go CLI Release (release.yml)
- Triggered by tags matching `v*` (e.g., `v0.1.0`)
- Produces: `tsuku-linux-amd64_VERSION_linux_amd64`, `tsuku-darwin-arm64_VERSION_darwin_arm64`, etc.

### tsuku-dltest Release (release.yml, lines 60-168)
- Triggered by same `v*` tags as Go CLI
- Injects version into `cmd/tsuku-dltest/Cargo.toml` using `GITHUB_REF_NAME#v`
- Produces: `tsuku-dltest-linux-amd64`, `tsuku-dltest-darwin-arm64`, `tsuku-dltest-linux-amd64-musl`
- **All artifacts tagged with SAME `v*` version** as Go CLI

### tsuku-llm Release (llm-release.yml)
- Triggered by separate tags matching `tsuku-llm-v*` (e.g., `tsuku-llm-v0.3.0`)
- Produces: `tsuku-llm-v0.3.0-darwin-arm64`, `tsuku-llm-v0.3.0-linux-amd64-cuda`, etc.
- **Uses separate versioning** from main CLI

### VersionSection Structure
**File:** `internal/recipe/types.go` (lines 177-192)

```go
type VersionSection struct {
	Source     string `toml:"source"`      // "nodejs_dist", "github_releases", "npm_registry", "homebrew", "cask", "tap"
	GitHubRepo string `toml:"github_repo"` // "rust-lang/rust" - for version detection only
	TagPrefix  string `toml:"tag_prefix"`  // "ruby-" - filter tags by prefix and strip it from version
	Module     string `toml:"module"`      // Go module path for goproxy
	Formula    string `toml:"formula"`     // Homebrew formula name
	Cask       string `toml:"cask"`        // Homebrew Cask name
	Tap        string `toml:"tap"`         // Homebrew tap
	FossilRepo string `toml:"fossil_repo"`
	ProjectName string `toml:"project_name"`
	VersionSeparator string `toml:"version_separator"`
	TimelineTag string `toml:"timeline_tag"`
}
```

---

## How GitHub Tag Resolution Works

### ListGitHubVersions() Implementation
**File:** `internal/version/resolver.go`

```
1. Split repo as "owner/repo"
2. Call GitHub API: Repositories.ListTags(ctx, owner, repoName, opts)
3. Return list of tags in newest-first order
```

Returns raw tag names (e.g., `["v0.2.0", "v0.1.5", "v0.1.0"]`)

### GitHubProvider.ListVersions() with Prefix
**File:** `internal/version/provider_github.go` (lines 36-56)

```
Input: tagPrefix = "v"
1. Get raw tags: ["v0.2.0", "v0.1.5", "v0.1.0"]
2. Filter by prefix: keep tags starting with "v"
3. Strip prefix: ["0.2.0", "0.1.5", "0.1.0"]
4. Return stripped versions
```

Example with multi-part prefix (e.g., `tagPrefix = "release-"`):
- Raw: `["release-1.0.0", "release-0.9.0"]`
- After strip: `["1.0.0", "0.9.0"]`

---

## Unified Release Strategy: Technical Requirements

### Goal
Both tsuku-dltest and tsuku-llm should resolve versions from the same `v*` tags in `tsukumogami/tsuku`.

### Current Obstacles

**tsuku-dltest:**
- Recipe has `tag_prefix = "v"` (correct)
- Recipe has no `github_repo` field (missing explicit config)
- Falls back to InferredGitHubStrategy (does extract repo from action, but ignores tag_prefix)
- **Fix:** Add `github_repo = "tsukumogami/tsuku"` to `[version]` section

**tsuku-llm:**
- Recipe has NO `[version]` section
- Falls back to InferredGitHubStrategy (infers from `tsukumogami/tsuku-llm`)
- Uses `asset_pattern = "tsuku-llm-v{version}-..."` (expects embedded version string in asset)
- Released separately via `tsuku-llm-v*` tags, not `v*` tags
- **Fix:** Requires architectural change: either (a) move to shared `v*` tags and modify asset_pattern, or (b) add version source pointing to `tsukumogami/tsuku` with `tag_prefix = "tsuku-llm-v"`

### Solution for Unified Versioning

#### Option A: Move tsuku-llm to shared `v*` tags (recommended)

**Changes needed:**
1. Modify `llm-release.yml` to trigger on `v*` tags instead of `tsuku-llm-v*`
2. Update artifact naming in llm-release.yml to remove the version prefix (or keep it)
3. Update `recipes/t/tsuku-llm.toml`:
   ```toml
   [version]
   github_repo = "tsukumogami/tsuku"
   tag_prefix = "v"
   
   [[steps]]
   action = "github_file"
   when = { os = ["darwin"], arch = "arm64" }
   repo = "tsukumogami/tsuku"
   asset_pattern = "tsuku-llm-{os}-{arch}"  # Remove "v{version}-" prefix
   ```

**Consequence:** tsuku-llm versions will track main CLI versions (v0.1.0 → llm also v0.1.0).

#### Option B: Use tag_prefix filtering (advanced)

If tsuku-llm must have independent versioning but same repo:

```toml
[version]
github_repo = "tsukumogami/tsuku"
tag_prefix = "tsuku-llm-v"

[[steps]]
asset_pattern = "tsuku-llm-v{version}-linux-amd64-cuda"
```

The GitHubProvider will:
1. Fetch all tags from tsukumogami/tsuku
2. Filter only tags starting with "tsuku-llm-v"
3. Strip "tsuku-llm-v" prefix (e.g., "tsuku-llm-v0.3.0" → "0.3.0")
4. Return "0.3.0" as the resolved version
5. asset_pattern substitutes: `{version}` = "0.3.0"

This requires maintaining separate `tsuku-llm-v*` tags in the main repo.

---

## Version Provider Priority & Resolution Logic

### Provider Selection Flow for tsuku-dltest (Current)

1. **Priority 90 (GitHubRepoStrategy):** Check if `github_repo` set
   - Currently: NO → skip
   
2. **Priority 80 (ExplicitSourceStrategy):** Check if `source` set
   - Currently: NO → skip
   
3. **Priority 10 (InferredGitHubStrategy):** Check for `github_file` action
   - Currently: YES → Create GitHubProvider(resolver, "tsukumogami/tsuku")
   - **Problem:** This provider gets created WITHOUT tagPrefix, so doesn't filter by "v"

### Provider Selection Flow for tsuku-dltest (Proposed Fix)

After adding `github_repo = "tsukumogami/tsuku"` to [version]:

1. **Priority 90 (GitHubRepoStrategy):** Check if `github_repo` set
   - YES → Create GitHubProviderWithPrefix(resolver, "tsukumogami/tsuku", "v")
   - Uses tag_prefix from recipe
   - All subsequent strategies skipped

---

## Tag Format Analysis

### Current Tag Format in tsukumogami/tsuku
- Main CLI: `v0.1.0`, `v0.2.0`, etc.
- All release artifacts built from same tag

### artifact_pattern Behavior

**tsuku-dltest example:**
```toml
asset_pattern = "tsuku-dltest-{os}-{arch}"
# With resolved version "0.1.0", expects artifacts:
# - tsuku-dltest-linux-amd64
# - tsuku-dltest-darwin-arm64
```

**tsuku-llm example:**
```toml
asset_pattern = "tsuku-llm-v{version}-darwin-arm64"
# With resolved version "0.1.0", expects artifacts:
# - tsuku-llm-v0.1.0-darwin-arm64
```

Key difference: tsuku-llm embeds version in asset name; tsuku-dltest does not.

---

## Conclusion: Action Items

To enable unified release versioning where both tsuku-dltest and tsuku-llm resolve from `v*` tags:

**Minimal Change (tsuku-dltest only):**
- File: `recipes/t/tsuku-dltest.toml`
- Add: `github_repo = "tsukumogami/tsuku"` to [version] section
- This promotes it from Priority 10 (inferred) to Priority 90 (explicit)
- Now respects existing `tag_prefix = "v"`

**Complete Unification (tsuku-dltest + tsuku-llm):**
- Change llm-release.yml trigger from `tsuku-llm-v*` to `v*`
- Add [version] section to tsuku-llm recipe with `github_repo = "tsukumogami/tsuku"` and `tag_prefix = "v"`
- Either:
  - Keep asset_pattern with "v{version}" and accept "v" in resolved version, OR
  - Change asset_pattern to remove "v" and adjust llm-release.yml artifact naming
- This ensures both artifacts get built from same release tag, can be deployed together

**Version Constraint Enforcement:**
- Currently not implemented in recipe schema
- Would require: new [version].same_version_as field or external metadata
- Alternatively: document in CLAUDE.local.md that release process must tag all artifacts simultaneously

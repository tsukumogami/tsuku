---
status: Current
problem: |
  The seed-queue tool populates the priority queue without checking whether packages
  already have recipes in the registry or embedded recipe directories. This causes
  the batch pipeline to waste CI time attempting to generate recipes for tools like
  git, docker, cmake, and go that already have working installations. The wasted
  cycles also pollute failure logs, making it harder to identify genuine generation
  issues.
decision: |
  Add a RecipeFilter to the seed package that checks both recipe directories
  (recipes/{letter}/{name}.toml and internal/recipe/recipes/{name}.toml) before
  packages enter the queue. The filter runs as a pre-processing step in
  cmd/seed-queue/main.go between Fetch() and Merge(), keeping Merge()'s existing
  semantics unchanged. Two new CLI flags (-recipes-dir and -embedded-dir) let
  callers specify the recipe locations.
rationale: |
  Filtering before merge is the natural integration point because it prevents
  unwanted packages from ever entering the queue, rather than skipping them later
  during generation. This keeps Merge() as a pure dedup-by-ID operation and avoids
  coupling the queue data structure to recipe storage layout. The alternative of
  filtering during orchestrator generation was rejected because packages would still
  consume queue slots and appear in batch metrics.
---

# DESIGN: Seed Recipe Skip

## Status

Current

## Context and Problem Statement

The batch generation prototype (PR #1265) revealed that the seed-queue tool (`cmd/seed-queue`) adds packages to the priority queue without checking whether they already have recipes. Recipes exist in two locations:

1. **Registry recipes** at `recipes/{first-letter}/{name}.toml` -- tools installable from the registry
2. **Embedded recipes** at `internal/recipe/recipes/{name}.toml` -- core tools compiled into the binary

When these packages reach the batch pipeline, they either fail CI (because generating a duplicate recipe triggers the duplicate recipe check) or waste generation time producing a recipe identical to one that already exists. The prototype hit both cases: git/docker/mise had registry recipes, and cmake/go had embedded recipes.

The `Merge()` function in `internal/seed/queue.go` deduplicates by package ID only. It has no awareness of the recipe directory structure.

### Scope

**In scope:**
- Filtering packages with existing recipes before they enter the queue
- Checking both registry and embedded recipe directories
- Using the same naming convention as the batch orchestrator

**Out of scope:**
- Changing `Merge()` semantics or the queue data structure
- Filtering during batch generation (orchestrator already handles validation failures)
- Discovery registry cross-referencing (handled separately by graduation logic)

## Decision Drivers

- Must check both recipe locations to avoid false negatives
- Must use the same path convention as `recipeOutputPath()` in the orchestrator (`recipes/{first-letter}/{name}.toml`)
- Should not change `Merge()` behavior -- it's a pure dedup-by-ID operation and should stay that way
- Should report skipped packages for visibility into what was filtered

## Considered Options

### Decision 1: Where to filter

The core question is at which point in the pipeline to exclude packages that already have recipes. Filtering too early risks missing context; filtering too late wastes resources. The seed-queue tool fetches packages from ecosystem sources, merges them into the priority queue, and the batch orchestrator later processes queue entries one at a time.

#### Chosen: Filter before Merge() in cmd/seed-queue

Add a filtering step between `Fetch()` and `Merge()` in `cmd/seed-queue/main.go`. A new `FilterExistingRecipes()` function in `internal/seed/` takes a slice of packages and two directory paths (registry, embedded), returning only packages without existing recipes. The caller passes two new flags: `-recipes-dir` and `-embedded-dir`.

This keeps `Merge()` focused on ID-based deduplication and prevents unwanted packages from ever entering the queue. Skipped packages are logged to stderr for visibility.

#### Alternatives Considered

**Filter inside Merge()**: Make `Merge()` accept recipe directory paths and check for existing recipes during its dedup loop. Rejected because it couples the queue data structure to recipe storage layout. `Merge()` is a generic dedup operation used by the queue -- adding file system checks violates that abstraction.

**Filter in the orchestrator during generation**: Let packages enter the queue and skip them when the orchestrator picks them up for generation. Rejected because packages still consume queue slots, inflate batch metrics (total count), and create confusing "skipped" statuses that must be tracked separately from genuine failures.

### Decision 2: Recipe path resolution

The filter needs to construct file paths from package names. The registry uses `recipes/{first-letter}/{name}.toml` and the embedded directory uses `internal/recipe/recipes/{name}.toml` (flat, no letter subdirectory).

#### Chosen: Reuse existing path logic

The orchestrator already has `recipeOutputPath()` that computes `{dir}/{first-letter}/{name}.toml`. Extract this into a shared utility in `internal/seed/` (or reference the pattern directly). For embedded recipes, simply check `{dir}/{name}.toml` since that directory is flat.

#### Alternatives Considered

**Walk the directories and build a set**: Scan both recipe directories at startup to build a name set, then filter against it. Rejected because the directories can contain hundreds of files and walking them adds startup cost. Direct path construction and `os.Stat()` per package is simpler and fast enough for the expected ~100 packages per seed run.

## Decision Outcome

**Chosen: Pre-merge filter with direct path checks**

### Summary

The seed-queue tool gets a new filtering step between fetching packages and merging them into the queue. A `FilterExistingRecipes()` function in `internal/seed/filter.go` takes a slice of packages and two directory paths, then returns only packages that don't have existing recipe files. For each package, it checks:

1. `{recipes-dir}/{first-letter}/{name}.toml` (registry recipe)
2. `{embedded-dir}/{name}.toml` (embedded recipe)

If either file exists, the package is excluded. The function also returns a list of skipped package names for logging. Two new CLI flags in `cmd/seed-queue/main.go` control the paths: `-recipes-dir` (default: `recipes`) and `-embedded-dir` (default: `internal/recipe/recipes`).

The path construction for registry recipes uses the same convention as the batch orchestrator: lowercase first letter of the package name as the subdirectory. Embedded recipes use a flat directory with no letter prefix. Package names from the seed source are used as-is for file lookup -- the seed sources already produce names that match recipe file naming (e.g., "bat" not "bat-cat"). If a recipe directory doesn't exist (e.g., no `-recipes-dir` provided or path is empty), that check is silently skipped.

### Rationale

Pre-merge filtering keeps the pipeline's responsibilities clean. The seed tool decides *what* goes in the queue; `Merge()` handles *deduplication*; the orchestrator handles *generation and validation*. Each component has a single concern. Direct path checks via `os.Stat()` are simpler than building a directory index and fast enough -- a typical seed run processes ~100 packages, so ~200 stat calls complete in milliseconds.

### Trade-offs Accepted

By choosing pre-merge filtering, we accept:
- Packages added before this feature existed remain in the queue (no retroactive cleanup)
- If recipe naming conventions change, the filter must be updated separately from the orchestrator

These are acceptable because existing queue entries will naturally fail or succeed in the orchestrator (no behavioral change), and naming convention changes are extremely rare.

## Solution Architecture

### Overview

A single new file `internal/seed/filter.go` contains the filtering logic. The `cmd/seed-queue/main.go` gains two flags and one new function call.

### Components

```
cmd/seed-queue/main.go
  ├── Fetch() → []Package
  ├── FilterExistingRecipes(packages, recipesDir, embeddedDir) → ([]Package, []string)  [NEW]
  └── Merge(filtered) → int

internal/seed/filter.go  [NEW]
  ├── FilterExistingRecipes(packages []Package, recipesDir, embeddedDir string) ([]Package, []string)
  └── recipeExists(name, recipesDir, embeddedDir string) bool

internal/seed/filter_test.go  [NEW]
  └── Tests for FilterExistingRecipes
```

### Key Interfaces

```go
// FilterExistingRecipes removes packages that already have recipes.
// Returns the filtered slice and a list of skipped package names.
func FilterExistingRecipes(packages []Package, recipesDir, embeddedDir string) ([]Package, []string)
```

### Data Flow

1. `Fetch()` returns raw packages from the ecosystem source
2. `FilterExistingRecipes()` checks each package name against both recipe directories
3. Filtered packages pass to `Merge()` for ID-based deduplication
4. Queue is saved to disk

## Implementation Approach

### Phase 1: Add filter function

Create `internal/seed/filter.go` with `FilterExistingRecipes()` and `recipeExists()`. Add tests in `filter_test.go` that create temp directories with recipe files and verify filtering.

### Phase 2: Wire into cmd/seed-queue

Add `-recipes-dir` and `-embedded-dir` flags to `cmd/seed-queue/main.go`. Call `FilterExistingRecipes()` between `Fetch()` and `Merge()`. Log skipped packages to stderr.

### Phase 3: Update CI workflow

Update the `batch-generate.yml` workflow to pass `-recipes-dir` and `-embedded-dir` flags when invoking `seed-queue`.

## Security Considerations

### Download Verification

Not applicable -- this feature does not download any artifacts. It only checks for the existence of local files using `os.Stat()`. The filter assumes existing recipes are valid; recipe integrity is maintained through code review on PRs that add or modify recipes. If a recipe were compromised, the filter would correctly skip re-generation for that tool, but this matches the existing trust model where checked-in recipes are treated as authoritative.

### Execution Isolation

The filter performs read-only file system access (stat calls) against two known directories. No files are created, modified, or executed. No elevated permissions required.

### Supply Chain Risks

Not applicable -- the filter reads local recipe file paths to determine existence. It does not fetch external content or trust external sources. The recipe directories are part of the checked-in repository. Package names are used to construct file paths via `filepath.Join()`, which normalizes path separators and prevents traversal attacks (e.g., a name containing `../` would resolve to a path outside the recipe directory, where no `.toml` file would exist, so the check would correctly return false).

### User Data Exposure

No user data is accessed or transmitted. The filter operates on package names (from the ecosystem source) and recipe file paths (from the repository).

## Consequences

### Positive

- Eliminates wasted CI time generating recipes for tools that already have them
- Cleaner failure logs -- only genuine generation failures appear
- Batch metrics more accurately reflect actual generation work
- Queue stays lean, containing only actionable entries

### Negative

- Two new flags add minor complexity to the seed-queue CLI
- Existing queue entries with recipes won't be retroactively removed

### Mitigations

- Default flag values match the standard repository layout, so most invocations don't need explicit flags
- Existing queue entries will naturally reach the orchestrator and fail fast during validation, matching current behavior

---
status: Proposed
problem: |
  The GitHub version provider's `isStableVersion` filter uses substring matching
  against a hardcoded keyword list (`preview`, `alpha`, `beta`, `rc`, `dev`,
  `snapshot`, `nightly`). This admits exotic prerelease formats like `9.6.0-M1`
  (gradle), `2.0.0-M5` (sbt), and any ecosystem-specific qualifier the keyword
  list happens not to mention, causing `tsuku eval` to resolve to a milestone
  build that does not exist on the upstream's distribution server. The filter
  is also too aggressive in the other direction: a stricter SemVer-aware
  definition would falsely reject hyphen-suffixed stable qualifiers like
  `-RELEASE`, `-FINAL`, `-LTS`, and `-GA` used by some JVM-ecosystem projects.
decision: |
  Replace the substring keyword filter with a SemVer-aware definition: any
  version whose `splitPrerelease` yields a non-empty prerelease component is
  unstable, with one exception. Add an opt-in `[version] stable_qualifiers`
  recipe field that names hyphenated suffixes which the upstream uses as
  release qualifiers rather than prereleases. The default is empty
  (= strict SemVer); recipes whose upstream uses RELEASE/FINAL-style
  conventions opt in by listing those qualifiers explicitly. The change is
  scoped to the GitHub provider's filter and to the parallel filter in the
  Fossil provider; the comparison logic in `version_utils.go` is unchanged.
rationale: |
  The filter and the comparison logic disagree today about what counts as a
  prerelease. The comparison logic uses `splitPrerelease`, which is the
  SemVer-correct definition. The filter uses substring matching, which is a
  weaker heuristic that misses milestone tags. Aligning the filter with the
  comparison logic eliminates the underlying mismatch without inventing a new
  abstraction. Per-recipe opt-in for stable qualifiers reflects the empirical
  reality that tagging conventions vary across upstreams but are stable within
  a single repository, and lets the recipe author encode their upstream's
  convention in the same place they encode the version source. A small global
  default allowlist was considered and rejected as needlessly impositional —
  the few projects that use these qualifiers can declare them once.
---

# DESIGN: Prerelease Detection in Version Providers

## Status

Proposed

## Context and Problem Statement

The GitHub version provider's job is to take a `[version] github_repo = "..."`
declaration and return the latest stable release. With a `tag_prefix`, it
filters tags by prefix, sorts them, and returns the first one that
`isStableVersion` admits.

`isStableVersion` lives in `internal/version/provider_github.go` and is
implemented as a case-insensitive substring search against a fixed keyword
list:

```go
func isStableVersion(version string) bool {
    lower := strings.ToLower(version)
    unstablePatterns := []string{"preview", "alpha", "beta", "rc", "dev", "snapshot", "nightly"}
    for _, pattern := range unstablePatterns {
        if strings.Contains(lower, pattern) {
            return false
        }
    }
    return true
}
```

The keyword list catches the most common prerelease markers, and Maven-style
tags (`maven-4.0.0-rc-5`, `maven-4.0.0-beta-3`, `maven-4.0.0-alpha-13`) flow
through it correctly.

It does not catch milestone tags. Two real-world examples surfaced during
the curated-recipe milestone:

- `gradle/gradle` publishes milestone pre-releases tagged `v9.6.0-M1`,
  `v9.5.0-M7`, `v9.5.0-RC4`. The RC tags are filtered. The M tags are not:
  `9.6.0-M1` lowercased contains `m1`, which matches none of the keywords.
- `sbt/sbt` publishes the same shape: `v2.0.0-M5`, `v2.0.0-RC12`, etc. The M
  tags pass the filter; the RC tags do not.

The version comparison logic in `internal/version/version_utils.go` ranks
`9.6.0-M1` *above* `9.4.1` (the genuine latest stable) because
`compareCoreParts` compares major.minor.patch first, and only falls back to
`comparePrereleases` when the core matches. So the provider returns the
milestone tag, the recipe derives a download URL like
`gradle-9.6.0-M1-bin.zip`, and `tsuku eval` fails with a 404 because that
URL does not exist on the gradle distribution server.

This is the immediate trigger for tightening the filter. But naively
tightening it — for example, by treating any hyphenated suffix as a
prerelease — would break a different class of upstream that uses hyphenated
suffixes to *signal stability*:

- **Spring** (historically): tags like `5.3.39-RELEASE` predate the project's
  move to plain semver in 6.x.
- **Hibernate** and other JBoss-era projects: `5.6.15-Final` (the Final
  qualifier signals "this is a stable release", not "this is a prerelease").
- **General availability releases**: occasional tags ending in `-GA` from
  Apache and similar projects.
- **Long-term support markers**: sometimes `-LTS` appears in distro and
  framework versioning.
- **Pure stability markers**: occasional tags ending in `-stable`.

By strict SemVer, every one of these is a prerelease. By the project's own
intent, none of them is. The filter cannot get this right without knowing
the upstream's convention.

## Decision Drivers

The chosen approach must satisfy these requirements, in priority order:

1. **Stop returning milestone (or other exotic) prereleases as the latest
   stable** when the upstream also publishes plain semver stables. This is
   the bug that triggered the work.
2. **Continue to admit upstreams whose hyphenated suffix signals stability**
   (`-RELEASE`, `-FINAL`, `-LTS`, `-GA`, `-stable`). The fix must not
   regress the JVM ecosystem.
3. **Keep existing curated and handcrafted recipes resolving to the same
   versions they do today**, except in cases where they were silently
   resolving to a wrong (milestone) version that happened not to break.
4. **Reuse the SemVer-aware `splitPrerelease` already in `version_utils.go`**.
   The filter and the comparison logic disagree today about what a
   prerelease is; aligning them is the right shape.
5. **Avoid inventing new abstractions** unless the simpler fix cannot meet
   the first four drivers.

Out of scope (intentionally not driving the design):
- Adding a general-purpose tag-filter regex to `[version]`. Captured as a
  rejected option below.
- Distinguishing "alpha < beta < rc" within prereleases. That ordering
  already lives in `comparePrereleaseIdentifiers` and is unchanged.
- Filtering pre-releases out of `ListVersions`. The lister must continue
  to return every tag so explicit pins (e.g.,
  `tsuku install gradle@9.6.0-M1`) keep working.

## Considered Options

### Option A: Strict SemVer with per-recipe `stable_qualifiers` field (chosen)

Replace the substring filter with `_, pre := splitPrerelease(v); pre == ""`,
and add an opt-in `[version] stable_qualifiers` recipe field listing
hyphenated suffixes the upstream uses as release qualifiers.

- Pro: Aligns the filter with the comparison logic. Catches every exotic
  prerelease format by construction. Recipe authors document upstream
  convention where it belongs (in the recipe).
- Pro: Per-recipe scope reflects empirical reality — tagging conventions
  vary across upstreams but are stable within a single repository.
- Con: Requires an audit of existing recipes whose upstream tags use
  hyphen-suffixed stables before flipping the default to strict.
- Con: New recipe authors must check upstream convention. Small ask
  (`git ls-remote --tags` plus a glance), but one extra step.

### Option B: Extend the keyword blocklist

Add `milestone` to the substring list and a regex matching `^m\d+$`
against the prerelease component. Keep everything else unchanged.

- Pro: Smallest possible change. Zero risk to existing recipes.
- Con: Whack-a-mole. The next exotic prerelease format from another
  ecosystem brings us back to this issue. The substring approach is
  the underlying flaw.

### Option C: Strict SemVer with a global stable-qualifier allowlist

Hardcode `["release", "final", "lts", "ga", "stable"]` into the provider
as universally-admitted prerelease components.

- Pro: No per-recipe configuration; existing recipes that depend on
  RELEASE-suffixed tags keep working without changes.
- Con: A global list risks both false positives (admitting a prerelease
  the upstream actually intends as unstable) and false negatives (missing
  a project-specific qualifier we did not anticipate). Tagging
  conventions vary across upstreams; a global list cannot get every
  upstream right.

### Option D: Per-recipe regex tag filter

Add `[version] tag_filter = "regex"` that admits only matching tags.

- Pro: Maximally expressive. Handles cases the allowlist cannot.
- Con: More general than the problem requires. Recipe authors must design
  and debug a regex per recipe, and the regex must handle prefix-stripped
  vs raw tags, escape semantics, etc. Higher cost per recipe and per
  reviewer.

### Option E: Use the GitHub releases API's `prerelease` flag

The `releases` API exposes a `prerelease: bool` annotation that respects
the upstream's intent.

- Pro: Authoritative when available — the upstream tells us directly.
- Con: Only works when the upstream uses the GitHub releases UI
  consistently. Many repos use git tags without GitHub releases (the
  existing `Resolver.resolveFromTags` fallback exists for exactly this
  case), and in that path no `prerelease` flag is available. Cannot be
  the primary mechanism without losing coverage.

## Decision Outcome

Chose **Option A: Strict SemVer with per-recipe `stable_qualifiers` field**.

Option A satisfies all five decision drivers. The strict SemVer baseline
catches the immediate gradle/sbt bug and every future exotic prerelease
format by construction (driver 1). The per-recipe override admits the JVM
RELEASE/FINAL-style conventions without globally weakening the filter
(driver 2). The audit step protects existing recipes from regression
(driver 3). Reusing `splitPrerelease` reuses an existing primitive
(driver 4). A single new recipe field is minimal new abstraction (driver 5).

Option B was rejected because it does not address driver 1 generally —
the next exotic prerelease format requires another keyword addition.
Option C was rejected because the user observed that tagging conventions
vary across upstreams but are stable within a repository, which makes
per-recipe configuration the correct abstraction. Option D was rejected
as more general than needed. Option E was rejected because it does not
work uniformly across the tags and releases code paths.

## Solution Architecture

The solution lives entirely in the version-resolution layer of tsuku. No
runtime, executor, or recipe-action changes are involved.

### New recipe field

`recipe.VersionSection` gains an optional field:

```go
StableQualifiers []string `toml:"stable_qualifiers"`
```

Recipes whose upstream uses hyphen-suffixed stable qualifiers list them
explicitly:

```toml
[version]
github_repo = "spring-projects/spring-framework"
tag_prefix = "v"
stable_qualifiers = ["release"]
```

```toml
[version]
github_repo = "hibernate/hibernate-orm"
stable_qualifiers = ["final", "ga"]
```

The default is empty (`StableQualifiers = nil` or `[]`), meaning strict
SemVer: any hyphenated suffix is a prerelease.

### Updated filter predicate

`isStableVersion` in `internal/version/provider_github.go` becomes:

```go
func isStableVersion(version string, stableQualifiers map[string]bool) bool {
    _, prerelease := splitPrerelease(version)
    if prerelease == "" {
        return true
    }
    return stableQualifiers[strings.ToLower(prerelease)]
}
```

The `stableQualifiers` map is constructed once at provider construction
time from the recipe's `StableQualifiers` slice (lowercased keys).

### Threading the field through the providers

`NewGitHubProvider` and `NewGitHubProviderWithPrefix` gain a
`stableQualifiers []string` argument. The provider stores the lowercased
set as a `map[string]bool` and passes it into `isStableVersion`.

`internal/version/fossil_provider.go` calls `isStableVersion(v)` in
`ResolveLatest` (the only other caller in the codebase). Update the
signature consistently and thread the recipe's
`StableQualifiers` through the Fossil provider as well.

`internal/version/provider_factory.go` reads
`recipe.VersionSection.StableQualifiers` and passes it into the provider
constructors.

### Composition with the comparison logic

The comparison logic (`comparePrereleases` in `version_utils.go`) is
unchanged. It already ranks plain semver above any hyphenated version. So:

- When upstream tags both `1.0.0` and `1.0.0-RELEASE`, plain wins
  (correct: `comparePrereleases` ranks empty prerelease above any
  non-empty prerelease).
- When upstream tags only `1.0.0-RELEASE`, `2.0.0-RELEASE`,
  `3.0.0-RELEASE`, the qualifier-admitted family sorts among itself
  correctly (`compareCoreParts` ranks `3.0.0` highest, so
  `3.0.0-RELEASE` wins).

### What is explicitly out of scope

- **Compound prerelease suffixes** like `1.0.0-final.1` or
  `1.0.0-RELEASE-hotfix`. The qualifier match is exact; compound forms
  are rejected. If a real upstream needs this, the recipe author can pin
  manually until we add a richer mechanism.
- **Case-and-space variations** like `Final-1` or `release_1`. Same
  answer.

## Implementation Approach

The implementation lands in #2325 in three contained slices:

1. **Schema and provider plumbing.**
   - Add `StableQualifiers []string` to `recipe.VersionSection` in
     `internal/recipe/types.go`. No validator changes required — the
     strict-validate path already enforces typed recipe fields, and an
     optional list with no constraint passes through.
   - Update `NewGitHubProvider`, `NewGitHubProviderWithPrefix`, and the
     Fossil provider constructors to accept `stableQualifiers []string`,
     storing it as a `map[string]bool` keyed by the lowercased
     qualifier.
   - Update `internal/version/provider_factory.go` to read the field
     from the recipe and pass it through.
2. **Filter logic.**
   - Replace the body of `isStableVersion` with the SemVer-aware
     predicate above.
   - Add a new `internal/version/provider_github_test.go` (existing
     tests for this file are sparse). Table-driven cases covering:
     plain semver, alpha/beta/rc, milestone (`-M1`, `-M2`), each stable
     qualifier (`release`, `final`, `lts`, `ga`, `stable`), build
     metadata after `+`, mixed qualifier and prerelease in different
     case orderings, and the existing keywords (which should still be
     rejected through the prerelease check, since the keyword test is
     no longer needed but the prerelease test catches them).
3. **Audit and recipe updates.**
   - Run `git grep -l 'github_repo' recipes/ internal/recipe/recipes/`.
   - For each recipe, inspect upstream tags via
     `git ls-remote --tags <repo> | tail -50` (or the GitHub API) to
     check for hyphen-suffixed stable releases.
   - Recipes whose upstream uses hyphen-suffixed stables receive a
     `stable_qualifiers = [...]` line in the same PR as the filter
     change.
   - The PR description explicitly lists the audit results so reviewers
     can flag any miss.

The three slices land in a single PR because the filter change without
the audit risks regression and the recipe updates without the schema
change do not parse.

## Security Considerations

This change is confined to the version-resolution layer and operates on
data that already flows through tsuku at install time:

- **No new external attack surface.** The version provider already fetches
  tags and releases from GitHub via authenticated or anonymous API calls
  with established error handling. The change reads the same data and
  applies a different in-memory predicate; no new endpoints, no new
  credential paths.
- **No new code-execution paths.** `isStableVersion` is a pure function on
  string input. Adding the `stableQualifiers` map does not introduce
  reflection, dynamic loading, or shell-out behavior.
- **Recipe field is data, not code.** `stable_qualifiers` is a list of
  lowercase strings used only as map keys. There is no regex or template
  evaluation; an attacker who controls a recipe cannot use the field to
  affect any other code path. The field is also checked by the
  strict-validate path before any plan is generated.
- **No version-resolution regression that could install untrusted
  binaries.** The change tightens the filter (admits fewer versions by
  default) rather than loosening it. The qualifier opt-in only re-admits
  versions the upstream itself published with a stable-signaling
  hyphen-suffix, and the recipe author who wrote the field has reviewed
  the upstream's tagging convention. Pinning behavior
  (`tsuku install <tool>@<version>`) is unaffected because the lister
  continues to return every tag.

## Consequences

### Positive

- Gradle and sbt recipes resolve correctly with the same `[version]
  github_repo` shape every other recipe uses, with no special-casing in
  the recipe.
- The filter and the comparison logic finally agree on what a prerelease
  is. Future exotic tag formats (which will appear) are caught by
  construction without code changes.
- Recipes that need stable-qualifier handling document their upstream's
  convention in the recipe itself, where the next person reading it can
  see why the recipe is structured the way it is.
- Tests gain a clear pinning surface: each behavior is exercised by a
  small input, not by a substring trick.

### Negative

- **Behavior change for existing recipes whose upstream tags use
  hyphen-suffixed stables.** Mitigation: the audit step in the
  Implementation Approach catches these, and the same PR adds the
  `stable_qualifiers` field to each affected recipe.
- **Recipe authors of new tools must know the upstream's tagging
  convention.** This is a small ask but it is one extra step in the
  recipe-authoring flow. We should document it in the recipe-author
  skill.
- The change touches a stable, well-exercised code path. We must add
  the table-driven tests described in the Implementation Approach to
  pin behavior before flipping the default.

### Affected Components

- `internal/version/provider_github.go` (filter and providers)
- `internal/version/fossil_provider.go` (parallel filter)
- `internal/recipe/types.go` (`VersionSection.StableQualifiers`)
- `internal/version/provider_factory.go` (threading the field)
- Recipes whose upstream uses hyphen-suffixed stable qualifiers (audit)
- Recipe-author skill documentation (one-line note about the new field)

### Implementation Issue

This design is implemented by [#2325](https://github.com/tsukumogami/tsuku/issues/2325).

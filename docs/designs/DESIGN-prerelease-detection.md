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

## Goals

1. Stop returning milestone (or other exotic) prereleases as the latest
   stable when the upstream also publishes plain semver stables.
2. Continue to admit upstreams whose hyphenated suffix signals stability
   (`-RELEASE`, `-FINAL`, `-LTS`, `-GA`, `-stable`).
3. Keep existing curated and handcrafted recipes resolving to the same
   versions they do today, except in cases where they were silently
   resolving to a wrong (milestone) version that happened not to break.
4. Avoid inventing new abstractions. Reuse the SemVer-aware
   `splitPrerelease` already in `version_utils.go`.

## Non-Goals

- Adding a general-purpose tag-filter regex to `[version]`. That is more
  expressive than the problem requires and shifts complexity onto recipe
  authors who would have to design and debug regular expressions per recipe.
  Captured as a future option in "Alternatives Considered" below.
- Distinguishing "alpha < beta < rc" within prereleases. That ordering
  already lives in `comparePrereleaseIdentifiers` and is unchanged.
- Filtering pre-releases out of `ListVersions`. The lister must continue
  to return every tag so that explicit version pins (e.g.,
  `tsuku install gradle@9.6.0-M1`) keep working.

## Decision

### 1. Replace the substring filter with a SemVer-aware predicate

Change `isStableVersion` to:

```go
func isStableVersion(version string, stableQualifiers map[string]bool) bool {
    _, prerelease := splitPrerelease(version)
    if prerelease == "" {
        return true
    }
    return stableQualifiers[strings.ToLower(prerelease)]
}
```

The function gains a `stableQualifiers` parameter sourced from the recipe.
Default (empty map) means "strict SemVer: any prerelease is unstable."

### 2. Add `[version] stable_qualifiers` recipe field

Extend `recipe.VersionSection` with:

```go
StableQualifiers []string `toml:"stable_qualifiers"`
```

The field is a list of lowercase prerelease identifiers that the upstream
uses as stable release qualifiers. Recipe example:

```toml
[version]
github_repo = "spring-projects/spring-framework"
tag_prefix = "v"
stable_qualifiers = ["release"]
```

The provider builds the `map[string]bool` from this slice once at
construction time and passes it into `isStableVersion`.

### 3. Apply the same change to the Fossil provider

`internal/version/fossil_provider.go` calls `isStableVersion(v)` in
`ResolveLatest` (the only other caller). Update the signature consistently
so the Fossil provider receives the same `stableQualifiers` map plumbed
through from the recipe.

### 4. Audit existing recipes before flipping the default

`git grep -l 'github_repo' recipes/ internal/recipe/recipes/` produces the
candidate list. For each, check whether the upstream's latest stable is
plain semver (no change needed) or hyphen-suffixed (add
`stable_qualifiers`). Realistically the second category is small and
likely does not include any currently-curated recipe.

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

### Negative / Risks

- **Behavior change for existing recipes whose upstream tags use
  hyphen-suffixed stables.** Mitigation: audit + add `stable_qualifiers`
  to those recipes in the same PR. The PR description should call out
  the audit explicitly so reviewers can flag any miss.
- **Recipe authors of new tools must know the upstream's tagging
  convention.** This is a small ask — `git ls-remote --tags` plus a
  glance is enough — but it is one extra step.
- The change touches a stable, well-exercised code path. We should add
  table-driven tests for `isStableVersion` covering: plain semver,
  alpha/beta/rc, milestone (`-M1`, `-M2`, with and without numeric
  suffix), each stable qualifier (`release`, `final`, `lts`, `ga`,
  `stable`), build metadata after `+`, and the existing keywords (since
  they are no longer special-cased — they should still be rejected
  through the prerelease check, but the test pins that behavior).

## Alternatives Considered

### A. Extend the keyword blocklist

Add `milestone` and a regex matching `^m\d+$` against the prerelease
component. Rejected: whack-a-mole. The next exotic prerelease format from
another ecosystem brings us back to this issue.

### B. Strict SemVer with a global allowlist

Hardcode `["release", "final", "lts", "ga", "stable"]` into the provider.
Rejected: the user pointed out that tagging conventions vary across
repositories but are stable within a repository. A global allowlist
risks both false positives (admitting a prerelease the upstream actually
intends as unstable) and false negatives (missing a project-specific
qualifier we did not anticipate). The cost of per-recipe configuration
is one extra line in the recipes that need it.

### C. Per-recipe regex tag filter

Add `[version] tag_filter = "regex"` that admits only matching tags.
Rejected for now: more general but more complex. The regex would need
to handle prefix-stripped vs raw tags, escape semantics, and recipe
authors would have to design and debug a regex per recipe. We can
revisit if the `stable_qualifiers` field proves insufficient.

### D. Use the GitHub releases API's `prerelease` flag

The `releases` API exposes a `prerelease: bool` annotation that respects
the upstream's intent. Rejected as a primary mechanism: it only works
when the upstream uses the GitHub releases UI consistently and tags
both releases and pre-releases through it. Many repos use git tags
without GitHub releases (the existing
`Resolver.resolveFromTags` fallback exists for exactly this case), and
in that path no `prerelease` flag is available. The proposed solution
works uniformly across the tags and releases code paths.

## Implementation Notes

- The signature change to `isStableVersion` is internal; no exported API
  changes.
- `provider_github.go:NewGitHubProvider` and `NewGitHubProviderWithPrefix`
  gain a `stableQualifiers []string` argument, populated from
  `recipe.VersionSection.StableQualifiers`.
- The `provider_factory.go` constructor for the GitHub strategy threads
  the field through.
- Tests live in a new `internal/version/provider_github_test.go` (the
  existing tests for this file are sparse) and exercise the
  `isStableVersion` predicate directly with the table cases listed above.
- Validation: `tsuku validate --strict` already enforces typed recipe
  fields; adding `StableQualifiers` extends the schema with no special
  validator work.

## Open Questions

None at this time. The audit in step 4 may surface recipes that need
the new field; if any of those recipes are currently curated, the audit
should be completed before this design moves to "Accepted" so the same
PR can land both the filter change and the recipe updates.

## Affected Components

- `internal/version/provider_github.go` (filter and providers)
- `internal/version/fossil_provider.go` (parallel filter)
- `internal/recipe/types.go` (`VersionSection.StableQualifiers`)
- `internal/version/provider_factory.go` (threading the field)
- Recipes whose upstream uses hyphen-suffixed stable qualifiers (audit)

## Implementation Issue

This design is implemented by [#2325](https://github.com/tsukumogami/tsuku/issues/2325).

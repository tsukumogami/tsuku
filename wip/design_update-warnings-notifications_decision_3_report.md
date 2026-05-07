<!-- decision:start id="static-pattern-version-fallback" status="assumed" -->
### Decision: Version Fallback for Static Asset Patterns

**Context**

`github_archive` has two execution paths depending on whether `asset_pattern` contains glob wildcards after variable expansion. Wildcard patterns (e.g., `cli_{version}_{os}_{arch}.tar.gz`) trigger `FetchReleaseAssets` inside `Decompose`, which hits the GitHub releases API to list assets and match the pattern. A missing asset is detected at plan time and can trigger a fallback retry before any download begins.

Static patterns (e.g., `deno-x86_64-unknown-linux-gnu.zip`, `eza_x86_64-unknown-linux-gnu.tar.gz`) construct the download URL directly from the version and platform without an API call. The URL is produced deterministically in `Decompose` and passed into the plan as a `download_file` step. The 404 surfaces only when the executor runs that step — well after plan generation completes. Retrofitting a fallback at that point requires either a re-plan loop in the executor or an intercepting retry at the download layer.

In practice, static pattern recipes use per-platform `when` clauses to select a fixed, fully-qualified asset name (the deno, eza, ripgrep, ollama recipes all follow this pattern). These names are stable across releases — upstream projects rarely rename binaries between minor versions. When a static pattern 404s, it typically indicates a broken release (assets not yet uploaded, release deleted) rather than a structural naming change.

**Assumptions**

- Static pattern recipes are a small minority of the registry (confirmed by inspection: ~5 recipes of ~100+ use static patterns; the rest use wildcard or template patterns).
- When a static pattern recipe 404s, the cause is usually a transient release problem (missing upload), not a structural misalignment that version fallback would fix. The asset name is not version-derived in these recipes, so falling back to X-1 would download the same-named file with the same URL shape.
- The constraint "zero call-site changes in executor/download pipeline" rules out Option C (retry on download 404) without additional architectural work.
- Wildcard fallback (decided by prior decisions) already handles the common case. Static patterns are a secondary concern.

**Chosen: Descope static patterns (v1 wildcard-only)**

Implement version fallback only for wildcard asset patterns in v1. Static pattern 404s continue to surface as download errors with the existing error message. Document the limitation as a known gap in the design doc.

**Rationale**

The proactive HEAD check (Option B) adds a network round-trip to every static-pattern install, even when the asset is present and the release is healthy. Given that static patterns are rare (~5% of recipes) and their 404s are typically transient release problems rather than fixable naming mismatches, this latency cost affects every user of those recipes on every install in exchange for handling an uncommon edge case. The download executor already handles transient HTTP failures with retry logic (403, 429, 5xx) — a broken release that gets assets uploaded after the initial attempt will succeed on retry without special fallback logic.

The retry-on-download-404 path (Option C) requires either coupling the executor to version fallback semantics (executor must know to trigger a re-plan) or adding a re-plan loop at the install manager level. Both approaches violate the zero call-site changes constraint and introduce non-trivial complexity across layers that have no other version-fallback awareness. The benefit doesn't justify the coupling.

Descoping (Option A) is consistent with the "version fallback belongs in `Decompose`" decision already recorded: `Decompose` is the only place with both `ctx.Resolver` and asset existence awareness. Static patterns don't call any asset API in `Decompose`, so there's no clean insertion point without adding a new API call — which is exactly what Option B proposes and what Option A defers. Deferring is correct because the pattern can be extended later once the wildcard path is proven and the cost-benefit of adding HEAD checks is clearer.

**Alternatives Considered**

- **Proactive HEAD check in Decompose**: For static patterns, `Decompose` makes an HTTP HEAD request to verify the asset URL before committing to the version. Adds a network round-trip to every static-pattern install (affects all users on every install, not just the rare fallback scenario). HEAD request does not guarantee the body will be present or complete — some CDNs respond 200 to HEAD but serve partial content. Also adds complexity to `Decompose` for a rare case with marginal benefit. Rejected in favor of deferring until the wildcard path proves the approach.

- **Retry on download 404**: The download executor detects a 404, signals the install manager to re-plan with version X-1, and retries. Violates the zero call-site changes constraint — requires the executor to propagate version fallback signals upward, and the install manager to accept mid-execution re-plan requests. Introduces state machine complexity (partial execution before failure, rollback or skip of already-completed steps). Rejected as out of scope for v1.

**Consequences**

Static pattern recipes that 404 will continue to fail with the current download error message. Users installing a tool during a broken release window must wait for the release to be fixed or manually install an older version. This is the existing behavior — v1 does not regress it. The limitation will be documented in the design doc so it can be extended in a follow-on once the wildcard fallback is operational and the real-world frequency of static-pattern failures is known.
<!-- decision:end -->

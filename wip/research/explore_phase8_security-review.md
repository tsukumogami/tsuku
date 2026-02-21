# Security Review: Ecosystem Name Resolution Design

**Reviewer:** pragmatic-reviewer
**Date:** 2026-02-21
**Document:** docs/designs/DESIGN-ecosystem-name-resolution.md

## Executive Summary

The security section is mostly correct for what this feature actually does. The "not applicable" justifications for download verification and execution isolation are valid -- this feature adds a TOML metadata field to recipe files that live in a reviewed git repository. The supply chain risk analysis is the only section that matters, and it identifies the right threat (malicious `satisfies` claim) with appropriate mitigations. One gap worth noting: the design doesn't specify behavior when a `satisfies` entry conflicts with an existing recipe's canonical name. Two items are over-engineered for the risk level.

## Detailed Findings

### 1. "Not Applicable" Justifications -- All Correct

**Download Verification (N/A):** Correct. The `satisfies` field changes which recipe is selected by name lookup. Once selected, the same recipe with the same download URLs, checksums, and verification runs. No new download paths are introduced.

**Execution Isolation (N/A):** Correct. The field is read-only metadata parsed from TOML at load time. It populates a `map[string][]string` on the `Metadata` struct. No file operations, no network calls, no privilege changes.

**User Data Exposure (N/A):** Correct. Pure name-to-name mapping within the recipe system.

### 2. Supply Chain Risk: Satisfies-to-Canonical Name Collision (Gap)

The design specifies collision detection when two recipes both claim the same `satisfies` entry (e.g., two recipes both claim `satisfies.homebrew = ["openssl@3"]`). Good.

**Missing case:** What if a recipe declares `satisfies.homebrew = ["sqlite"]` where `sqlite` is already the canonical name of another recipe? The satisfies fallback only triggers after exact-name lookup fails (design lines 108-111), so this wouldn't cause a resolution error -- the exact match would win. But it creates a semantic inconsistency: a recipe is claiming to satisfy a name it doesn't actually replace. This isn't a security vulnerability because the exact-match path takes priority, but the validation in Phase 4 should reject `satisfies` entries that collide with existing canonical recipe names to prevent confusion.

**Severity: Advisory.** No security impact due to the exact-match-first design. Worth a validation rule.

### 3. Supply Chain Risk: Registry Injection (Correctly Identified, Sufficient Mitigation)

The design's risk table notes "A compromised registry could inject entries" as residual risk. This is accurate. Looking at the actual registry code (`internal/registry/registry.go`), recipes are fetched from `raw.githubusercontent.com` over HTTPS. A compromised registry could include `satisfies` entries that redirect lookups.

However, this isn't a *new* risk introduced by this feature. A compromised registry can already serve malicious recipe content (different download URLs, different checksums). The `satisfies` field doesn't make registry compromise worse -- it just adds another field an attacker could manipulate, but the existing fields (download URLs, checksums) are already more dangerous.

**Severity: Not a finding.** The residual risk is pre-existing and correctly acknowledged.

### 4. Over-Engineering: Collision Detection Error Behavior

The design says (line 139): "What if two recipes both declare `satisfies.homebrew = ["openssl@3"]`? The loader should error rather than silently pick one."

For embedded recipes (compiled into the binary), collisions are caught at build time via PR review. For registry recipes, erroring on collision is reasonable. But this is listed as an uncertainty, not a security concern, and that's the right framing.

**However**, the collision detection doesn't need to be a hard error for all callers. A `tsuku install openssl@3` that errors because two registry recipes both claim the name is worse UX than picking the one that matches by canonical name first. The design already has exact-match priority, so collisions only matter in the fallback path.

**Severity: Advisory (over-engineering risk).** A warning + first-match-wins would be simpler than a hard error for end users. Hard errors are appropriate for recipe validation CI, not the runtime loader.

### 5. Over-Engineering: Ecosystem Key in Satisfies Map

The `satisfies` field is `map[string][]string` keyed by ecosystem (e.g., `satisfies.homebrew = ["openssl@3"]`). The design is scoped to Homebrew name resolution. No cross-ecosystem resolution is planned (explicitly out of scope, line 32).

The ecosystem key adds structure that no current or proposed consumer uses. The loader's `lookupSatisfies` (line 222) would need to search across all ecosystem keys regardless, since a dependency string like `"openssl@3"` doesn't carry ecosystem context. The flat alternative -- `satisfies = ["openssl@3"]` as a simple string array -- would be simpler and sufficient.

**Counter-argument:** The ecosystem key provides documentation value ("this is the Homebrew name") and future-proofs for potential per-ecosystem disambiguation. Since it's a TOML field shape that's hard to change later, the structure is defensible.

**Severity: Advisory.** The extra structure is small and bounded. Not blocking.

### 6. Attack Vector Not Considered: Local Recipe Shadowing + Satisfies

The loader's priority chain is: cache -> local -> embedded -> registry -> satisfies fallback. A local recipe at `$TSUKU_HOME/recipes/openssl.toml` already shadows the embedded `openssl` recipe (this is existing behavior, line 96-109 of loader.go).

With the satisfies feature: if a user has a local `openssl.toml` that does NOT declare `satisfies.homebrew = ["openssl@3"]`, but the embedded one does, then the satisfies index would still contain the embedded recipe's mapping (since the index is built from embedded + registry). A lookup for `openssl@3` would resolve to `openssl`, then the loader would return the local recipe (which wins by priority). This means the local recipe inherits a `satisfies` claim it never made.

This is actually fine -- the local recipe IS the `openssl` recipe (same canonical name), so it should be the one installed when something asks for `openssl@3`. The user chose to override `openssl` locally. No security issue.

**Severity: Not a finding.** The existing shadowing semantics compose correctly with satisfies. Noted for completeness.

### 7. Lazy Index Build: No DoS Concern

The lazy index build (scanning all embedded recipes on first fallback) is bounded by the number of embedded recipes ("tens of files", line 164). This is not a DoS vector. The registry manifest scan is a single file read. Both are fine.

**Severity: Not a finding.**

## Summary Table

| # | Finding | Severity | Recommendation |
|---|---------|----------|----------------|
| 1 | "N/A" justifications for download/execution/data | Correct | No action needed |
| 2 | Satisfies entry matching a canonical recipe name | Advisory | Add validation rule rejecting satisfies entries that match canonical names |
| 3 | Registry injection residual risk | Pre-existing | Already correctly acknowledged in design |
| 4 | Hard error on collision may be over-engineered for runtime | Advisory | Use warning + first-match for runtime, hard error for CI validation only |
| 5 | Ecosystem key in satisfies map | Advisory | Defensible, keep as-is |
| 6 | Local recipe shadowing + satisfies interaction | No issue | Composes correctly |
| 7 | Lazy index build performance | No issue | Bounded by design |

## Verdict

The security analysis in the design document is appropriate for the actual risk level. The "not applicable" dismissals are justified -- this feature is a TOML metadata field parsed from PR-reviewed recipe files. The supply chain risk section correctly identifies the meaningful threat (malicious satisfies claims) and provides adequate mitigations (PR review, collision detection, exact-match priority).

No blocking security issues. Two advisory items worth addressing during implementation (canonical-name collision validation, runtime vs. CI error behavior).

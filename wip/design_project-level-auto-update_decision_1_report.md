# Decision 1: How .tsuku.toml Modifies the Auto-Apply Decision

## Question

When a `.tsuku.toml` exists in the CWD ancestry, how should its version constraints
interact with `MaybeAutoApply`'s filtering of cached update entries?

## Analysis

### Option A: Project pin overrides global pin during auto-apply filtering

For each pending update, if the tool appears in `.tsuku.toml`, the project's Version
replaces the cached `Requested` field for filtering purposes. If the project pin is
`PinExact`, skip. If `PinMajor`/`PinMinor`, re-evaluate `LatestWithinPin` against
the project boundary.

**Strengths:**
- Fully honors PRD R17 across all pin levels.
- Consistent mental model: the project config is the effective pin, period.
- Prefix pins in `.tsuku.toml` genuinely constrain updates (e.g., project says "20"
  but cache has a 21.x candidate from a global "latest" pin -- that gets blocked).

**Weaknesses:**
- The cached `LatestWithinPin` was resolved against the *global* pin. If the project
  pin is narrower, the cached version might not satisfy it. We'd need to re-check
  `VersionMatchesPin(entry.LatestWithinPin, projectVersion)` at apply time. This is
  a string comparison (fast) but adds a code path that can silently suppress updates
  when the project pin doesn't match the cached candidate.
- If the project pin is *broader* than the global pin, we can't expand the candidate
  set without a new version check. The cached `LatestWithinPin` was computed under
  the narrower global pin, so it's still valid -- we just can't find a newer version
  that the broader project pin would allow. This is safe (conservative) but could
  surprise users expecting the broader pin to unlock more updates.

### Option B: Project exact pins suppress, prefix pins are advisory

Only exact pins (`PinExact`) in `.tsuku.toml` have enforcement power during
auto-apply. Prefix pins are used by `tsuku install` and `tsuku run` but don't
alter auto-apply behavior.

**Strengths:**
- Simplest implementation: just check `PinLevelFromRequested(projectVersion) == PinExact`
  and skip if true.
- No re-evaluation of cached candidates needed.

**Weaknesses:**
- Violates PRD R17. The PRD says `node = "20"` allows auto-update *within 20.x.y*,
  which implies it should also *block* updates outside 20.x.y. If the global pin is
  "latest" and the cache has node 22.0.0 as `LatestWithinPin`, Option B would apply
  it despite the project saying "20". This is the exact scenario R17 is designed to
  prevent.
- Inconsistent mental model: prefix pins mean different things in different contexts.

### Option C: Allowlist/blocklist classification

Classify each declared tool as blocked (exact pin) or allowed (anything else). The
global pin from `state.json` governs what version gets installed when allowed.

**Strengths:**
- Simple binary classification per tool.
- Slightly more nuanced than B (explicit allow vs implicit pass-through).

**Weaknesses:**
- Same R17 violation as Option B for prefix pins. A project declaring `node = "20"`
  would still allow a node 22.x update if the global pin is "latest".
- The "allowlist" framing adds conceptual overhead without adding correctness.
- Functionally equivalent to Option B with extra abstraction.

## CWD Edge Cases

All three options share these edge case behaviors:

1. **No .tsuku.toml in CWD ancestry**: Auto-apply proceeds with global pins only.
   No behavioral change from today.

2. **User runs tsuku from a different directory**: Project config doesn't apply.
   This is correct -- the user isn't "in" that project context.

3. **Nested .tsuku.toml files**: `LoadProjectConfig` already returns only the
   nearest one. No change needed. The nearest config wins, matching how tools
   like `.nvmrc` and `.tool-versions` work.

4. **Tool declared in .tsuku.toml but not installed**: No cached entry exists,
   so auto-apply has nothing to filter. No interaction.

5. **Tool installed but not in .tsuku.toml**: Global pin applies. The project
   config only speaks for tools it declares.

6. **Broader project pin than global pin** (e.g., project says "20", user
   installed with "20.16"): Under Option A, the project pin "20" is broader,
   so `VersionMatchesPin(entry.LatestWithinPin, "20")` will pass any 20.x.y
   candidate. The cached `LatestWithinPin` was computed under the narrower
   "20.16" global pin, so it's already within "20" -- the update proceeds.
   This is correct and conservative. A broader project pin never blocks an
   update that the global pin already allowed.

## Recommendation: Option A

**Confidence: High**

Option A is the only option that correctly implements PRD R17 across all pin levels.
The implementation cost is modest:

1. In `MaybeAutoApply`, after loading pending entries, call
   `LoadProjectConfig(cwd)` once (single filesystem walk, fast).
2. For each pending entry, check if the tool is declared in the project config.
3. If declared, derive the effective pin from the project's Version field.
4. If `PinExact`, skip the entry entirely.
5. If `PinMajor`/`PinMinor`, check `VersionMatchesPin(entry.LatestWithinPin,
   projectVersion)`. If the cached candidate doesn't satisfy the project pin,
   skip it -- the update check was done under a different constraint.
6. If the tool is not declared in `.tsuku.toml`, use the cached `Requested` as
   today.

The "broader project pin" case (edge case 6) is naturally correct: the cached
candidate already satisfies the narrower global pin, which is a subset of the
broader project pin.

The `LoadProjectConfig` call adds one directory walk (typically 2-5 `stat` calls)
per command invocation. This is well within the zero-latency budget from R19 since
the walk stops at `$HOME` and the common case is a shallow directory tree.

Options B and C both fail the `node = "20"` with global `"latest"` scenario, which
is the motivating use case in R17. The added complexity of Option A (one
`VersionMatchesPin` call per pending entry) is minimal compared to the correctness
gap.

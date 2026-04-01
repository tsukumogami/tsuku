<!-- decision:start id="self-update-cache-integration" status="assumed" -->
### Decision: Self-update cache integration

**Context**

Feature 2's `RunUpdateCheck()` iterates installed tools from `state.json`, resolves their latest versions via recipe-derived providers, and writes per-tool cache entries to `$TSUKU_HOME/cache/updates/<toolname>.json`. tsuku itself isn't a managed tool -- it has no recipe and no entry in `state.json` (PRD decision D5). Yet R8 requires that tsuku's own version appears in the periodic background check so that `tsuku outdated` and Feature 5 notifications can surface self-update availability.

The challenge is twofold: (1) injecting a self-check into the existing background check flow without a recipe or state entry, and (2) ensuring Feature 3's `MaybeAutoApply` skips tsuku's cache entry since self-update is a deliberate user action via `tsuku self-update`.

**Assumptions**

- The tool name "tsuku" won't collide with a managed recipe. This holds because D5 explicitly excludes tsuku from the managed tool system, and no recipe for tsuku itself will exist.
- `buildinfo.Version()` returns a semver-parseable string for release builds. Dev builds return "dev-..." which won't match any release, so the check correctly reports "no update" for development builds.

**Chosen: Append self-check to RunUpdateCheck with well-known constant**

Add a `checkSelf()` function called at the end of `RunUpdateCheck`, after the tool loop. This function:

1. Checks `userCfg.UpdatesSelfUpdate()` -- returns early if disabled.
2. Gets the current version from `buildinfo.Version()`.
3. Creates a `GitHubProvider` for `tsukumogami/tsuku` (no recipe needed).
4. Calls `ResolveLatest()` to get the newest release.
5. Writes a standard `UpdateCheckEntry` with `Tool: SelfToolName`, `ActiveVersion: buildinfo.Version()`, empty `Requested` and `LatestWithinPin`, and `LatestOverall` set to the resolved version.

A package-level constant `const SelfToolName = "tsuku"` identifies the self-update entry. `MaybeAutoApply` skips any entry where `entry.Tool == updates.SelfToolName`. Consumers like `tsuku outdated` and Feature 5's notification system check this constant to format the display differently (e.g., "tsuku self-update available: v0.2.0" instead of the regular tool update format).

The entry is written to the same cache directory as tool entries (`$TSUKU_HOME/cache/updates/tsuku.json`), so `ReadAllEntries` picks it up automatically without consumer changes beyond display formatting.

**Rationale**

This approach needs zero schema changes to `UpdateCheckEntry` -- the existing fields accommodate tsuku's case naturally. `Requested` is empty because tsuku tracks latest (no pin). `LatestWithinPin` is empty because there's no pin boundary. `LatestOverall` carries the latest release version. The well-known constant provides a single, greppable point of identification that both `MaybeAutoApply` and display consumers can use. Adding the check as a function call after the main loop keeps the self-check isolated while reusing the same cache infrastructure.

**Alternatives Considered**

- **Add IsSelfUpdate boolean field to UpdateCheckEntry**: A new `IsSelfUpdate bool` field on the struct, with `MaybeAutoApply` checking the flag instead of the tool name. Rejected because it adds a schema change for exactly one entry -- tsuku is the only self-updating tool, so a dedicated boolean is over-engineered. The well-known constant achieves the same disambiguation without modifying the shared data model.

- **Separate self-update cache file**: Write tsuku's check to `$TSUKU_HOME/cache/self-update.json` with its own struct, outside the tool cache directory. Rejected because it fragments every consumer. `ReadAllEntries` wouldn't see it, so `tsuku outdated`, Feature 5 notifications, and any future consumer would each need explicit separate-file handling. The main benefit (no confusion with managed tools) doesn't justify the maintenance cost when a constant already prevents confusion.

**Consequences**

- `RunUpdateCheck` gains a `checkSelf()` call (~20 lines) that uses the GitHub provider directly, bypassing recipe loading.
- `MaybeAutoApply` needs a one-line guard: `if entry.Tool == SelfToolName { continue }`.
- `tsuku outdated` and Feature 5 display code check `Tool == SelfToolName` for formatting.
- If tsukumogami/tsuku ever moves to a different GitHub org or hosting, the hardcoded repo string in `checkSelf()` would need updating. This is acceptable since the same string would appear in the self-update download logic anyway.
<!-- decision:end -->

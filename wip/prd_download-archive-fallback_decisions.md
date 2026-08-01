# Auto-mode decisions — PRD download-archive-fallback

`--auto` was in force for the whole run, so every decision below was taken
against the recommended default rather than put to the operator. Recorded here
so they are reviewable after the fact.

| # | Decision point | Chosen | Why |
|---|----------------|--------|-----|
| 1 | Recipe-facing shape for the source list | New `fallback_urls` array alongside the existing `url` | `url` keeps its type and its meaning as the first source, so every existing `GetString(params, "url")` call site, the `actionVersionRules` placeholder check, and `validateURLParam` keep working untouched. Widening `url` to string-or-array would force an `interface{}` branch at every one of those sites. |
| 2 | Does the generated plan record the full list or the winner? | The full ordered list | Plans are published to R2 and installed from weeks later. Recording the winner leaves the plan single-sourced exactly where the consumer can do nothing about it. Matches the architectural review on #2443. |
| 3 | Emit the list into every plan, or only when a recipe declares one? | Only when non-empty | Keeps every single-source golden byte-identical, so the diff shows the recipes that actually changed instead of a repo-wide regeneration that would launder unrelated version drift (`--pin-from` does not work for registry recipes). |
| 4 | Install-time fallback: identical to plan-time, or best-effort? | Identical | The published-plan journey is the one the user cannot repair. A plan with no `fallback_urls` (older tsuku, or a single-source recipe) behaves exactly as today. |
| 5 | Upstream-anchored verification, per the #2443 review comment | Scoped to "keeps working across fallback" plus a preflight warning; not made mandatory | The review comment asked for upstream anchoring in this issue's scope and stated minisign machinery already exists in `internal/actions/download.go`. It does not — the file carries PGP verification only (`grep -rn minisign internal/ --include=*.go` returns nothing). zig publishes `.minisig`, not `.sha256` (verified: `zig-x86_64-linux-0.14.1.tar.xz.sha256` returns 404, `.minisig` returns 200), so making anchoring mandatory would block the one recipe the acceptance criteria require converting. Departure is recorded in the design doc. |
| 6 | Naming: `fallback_urls` vs `sources` vs `mirrors` | `fallback_urls` | `mirrors` implies mirror-registry semantics the review comment explicitly warns against. `sources` reads as if `url` were one of them, which invites the question of ordering. `fallback_urls` states the ordering relationship to `url` in the name. |

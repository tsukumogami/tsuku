# Architecture Review: DESIGN-discovery-resolver.md

## 1. Is the architecture clear enough to break into implementation issues?

**Yes, with caveats.** The six phases map cleanly to issues. Phase 1 (interface + registry lookup) and Phase 2 (install/create convergence) are well-scoped. Phase 3 (ecosystem probe) is the largest and should probably split into two issues: one for the parallel probe infrastructure and one for disambiguation logic. Phase 5 (registry bootstrap) is data work, not code -- it needs a clear acceptance criterion for what "~500 entries" means and how they're validated.

## 2. Missing Components or Interfaces

### CanBuild() returns bool, but the probe needs metadata

The design acknowledges this gap (line 387: "requires extending CanBuild() or adding a Probe() method") but doesn't decide. This is a blocking decision for Phase 3. The current `CanBuild(ctx, BuildRequest) (bool, error)` signature can't return download counts or age. Options:

- **Add a `Probe(ctx, name) (*ProbeResult, error)` method to SessionBuilder**: Clean separation but requires touching all 8 builders.
- **Add a separate `EcosystemProber` interface**: Only builders that support metadata implement it. The probe falls back to bare `CanBuild()` for builders that don't. This is the better option -- it's backward-compatible and doesn't force metadata on builders that lack it (e.g., GitHub releases don't have "download count" in the same sense as npm).

**Recommendation**: Define `EcosystemProber` as an optional interface. Phase 3's design doc should decide this.

### BuildRequest requires Source, but discovery doesn't have one yet

`CanBuild()` takes a `BuildRequest` which includes a `Source` field. For ecosystem builders querying by name, the "source" is just the package name itself. But for the GitHub builder, `CanBuild()` expects `owner/repo` -- which is exactly what discovery is trying to find. This means the GitHub builder can't participate in ecosystem probing at all (it's not an ecosystem registry). The design implicitly assumes this but should state it explicitly: ecosystem probe only runs against ecosystem builders (cargo, gem, pypi, npm, go, cpan, homebrew), not the GitHub builder.

### No caching layer

If a user runs `tsuku install foo` and it fails during build, running it again re-probes all ecosystems. A short-lived cache (even just in-memory for the process lifetime) of discovery results would improve the retry experience. Not critical for v1 but worth noting.

## 3. Are the implementation phases correctly sequenced?

**Mostly yes, one reordering suggestion.**

Phase 5 (registry bootstrap) should move before Phase 2 (install/create convergence). Reason: the install convergence is only useful if the registry has data in it. Shipping the converged `tsuku install` with an empty registry means every tool falls through to ecosystem probe (Phase 3) or LLM (Phase 4), neither of which exist yet. The natural sequence:

1. Phase 1: Resolver interface + registry lookup (foundation)
2. Phase 5: Registry bootstrap (populate the data)
3. Phase 2: Install/create convergence (now the converged path actually works for ~500 tools)
4. Phase 3: Ecosystem probe
5. Phase 4: LLM discovery
6. Phase 6: Error UX polish

This way each phase delivers incremental user value.

## 4. Simpler Alternatives Overlooked?

### Alternative: Skip ecosystem probe entirely at launch

The registry covers ~500 tools. LLM discovery covers the long tail. The ecosystem probe is the most complex component (parallel queries, metadata retrieval, disambiguation, filtering) and covers a middle ground that may not be large enough to justify the complexity. Users who know their tool is on crates.io can still use `--from crates.io:toolname`.

**Counter-argument**: The ecosystem probe is where tsuku's builder infrastructure pays off. Without it, the eight builders are only reachable via explicit `--from` flags or LLM recommendation. The probe makes them discoverable. Still, the design could explicitly mark Phase 3 as optional for the v0.5.0 preview.

### Alternative: Discovery registry as TOML, not JSON

The recipe registry already uses TOML. Having a JSON discovery registry introduces a second format. Using TOML would be more consistent, though JSON is arguably better for a flat key-value map. Minor point.

## 5. Is the Resolver interface well-defined enough?

**Yes.** The `Resolver` interface is minimal and correct:

```go
type Resolver interface {
    Resolve(ctx context.Context, toolName string) (*DiscoveryResult, error)
}
```

One issue: **the error semantics need clarification**. Currently `ChainResolver` treats any error as "skip to next stage." But some errors should halt the chain:

- Context cancellation (already noted)
- Rate limit errors from ecosystem APIs (should these halt or skip?)
- LLM budget exhaustion

The design should define which errors are "soft" (skip to next) vs "hard" (stop the chain). A simple approach: only skip on `nil` result with `nil` error. Any non-nil error propagates unless it's a specific "not found" sentinel.

### DiscoveryResult.Confidence as string

Using a string for confidence levels ("registry", "ecosystem", "llm") is fine for display but makes programmatic comparison fragile. Consider a typed constant (`ConfidenceLevel` type with `iota`). Minor.

## 6. Ecosystem probe integration with CanBuild()

**Partially sound, with the gap noted above.** `CanBuild()` works for existence checks but returns no metadata. The design correctly identifies this gap but defers the decision.

Additional concern: `CanBuild()` currently takes a `BuildRequest` which is constructed from a `--from` flag (i.e., the source is already known). For discovery, we're going the other direction -- we have a tool name and want to check if an ecosystem claims it. Some builders may interpret the `BuildRequest.Name` vs `BuildRequest.Source` fields differently. The Phase 3 design needs to audit each builder's `CanBuild()` to ensure it works when only a name is provided.

## 7. Error/fallback UX gaps

### Missing scenarios

- **Multiple ecosystem matches, user is non-interactive** (piped input, CI): The design mentions `--yes` for LLM confirmation but doesn't address what happens when disambiguation requires a prompt in non-interactive mode. Should it pick the highest-popularity match? Error out?
- **Registry entry points to archived/deleted repo**: The error table doesn't cover this. The user would get a confusing build failure rather than a discovery-level message.
- **Partial ecosystem probe results**: If 5/7 ecosystems respond and 2 timeout, should the result include a warning that coverage was incomplete?

### The "also available" message

The disambiguation display (`"Found bat (sharkdp/bat via crates.io, 45K downloads/day). Also available: npm..."`) is good UX. But it should also mention `--from` syntax so the user knows how to override: `"Use tsuku install bat --from npm to install the npm version instead."`

Wait -- `tsuku install` with `--from` is `tsuku create`. The convergence means the user would need `tsuku create bat --from npm:bat` for the override. This command distinction undercuts the "single entry point" goal. Consider accepting `--from` on `tsuku install` as well (just forwarding to create internally).

## Summary of Key Findings

| # | Finding | Severity | Phase Affected |
|---|---------|----------|----------------|
| 1 | `CanBuild()` can't return metadata; need `EcosystemProber` interface decision | Blocking | Phase 3 |
| 2 | GitHub builder can't participate in ecosystem probe (needs explicit statement) | Clarification | Phase 3 |
| 3 | Phase 5 (bootstrap) should move before Phase 2 (convergence) for incremental value | Sequencing | All |
| 4 | Error semantics (soft vs hard) undefined for ChainResolver | Design gap | Phase 1 |
| 5 | Non-interactive disambiguation behavior undefined | UX gap | Phase 3 |
| 6 | `--from` should work on `tsuku install` too, not just `tsuku create` | UX gap | Phase 2 |
| 7 | Archived/deleted repo error path missing | UX gap | Phase 6 |
| 8 | Ecosystem probe could be optional for v0.5.0 preview | Simplification | Phase 3 |

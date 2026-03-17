---
role: pragmatic-reviewer
issue: 3
scope: feat(config): add registry configuration and GetFromSource
---

## Findings

### 1. Central case error-propagation path is untested (Advisory)

`internal/recipe/loader.go:128` -- The `SourceCentral` branch propagates non-not-found errors (network failures, parse errors) via `fmt.Errorf("central registry error: %w", err)`. No test exercises this path. A mock provider returning a non-not-found error would cover it.

**Fix:** Add `TestLoader_GetFromSource_Central_PropagatesRealError` with a mock that returns a non-`"not found"` error.

### 2. Local case swallows not-found errors differently than central (Advisory)

`internal/recipe/loader.go:146-158` -- The `SourceLocal` case returns raw errors from `p.Get()` without distinguishing not-found from real errors (line 150-151), while the central case (line 128) filters them. If a local provider returns a transient error, callers get a raw error instead of the wrapped form. This inconsistency is minor since LocalProvider currently only returns `os.IsNotExist` errors, but it's a latent divergence.

**Fix:** No action needed now, but worth noting if distributed providers are later added with richer error types.

### 3. TOML key format test gap (Advisory)

`internal/userconfig/userconfig_test.go:1530` -- `TestRegistryLoadFromTOMLFile` uses bare TOML keys (`[registries.acme_tools]`) with underscores, but real usage stores `"owner/repo"` keys that require TOML quoting (`[registries."acme/tools"]`). The round-trip test covers the actual format, but the hand-written TOML test doesn't match what users will see in their config files.

**Fix:** Either add a test case with quoted slash keys in raw TOML, or note that the round-trip test already covers this.

## No Blocking Findings

The implementation is straightforward. `RegistryEntry` and `StrictRegistries` are simple struct fields with proper `omitempty` tags. `GetFromSource` has clear dispatch logic, bypasses the cache correctly, and tests cover all documented acceptance criteria (cache bypass, cache non-write, provider selection, error on unknown source). No dead code, no speculative generality, no scope creep.

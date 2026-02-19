# Architecture Review: DESIGN-gpu-backend-selection.md

## Review Scope

Full architectural soundness review of the GPU backend selection design document before commit. Verified all claims against the current codebase at commit `57f5c1b8`.

---

## 1. Is the architecture clear enough to implement?

**Yes, with two gaps that need closing before implementation.**

The design correctly identifies the three-step change: manifest schema expansion, Go-side GPU detection, and variant-aware path construction. The component change list (`internal/llm/addon/`) maps directly to existing files, and the new files (`detect.go`, `detect_linux.go`, etc.) follow Go build-tag conventions for platform-specific code.

### Gap 1: AddonManager state management after detection

The design says `EnsureAddon()` will call `DetectBackend()` and then look up the variant in the manifest. But it doesn't specify how the `AddonManager` stores the selected backend internally. Currently, `AddonManager` has `homeDir` and `cachedPath` fields. After detection, the manager needs to remember the selected backend for:

- Constructing `BinaryPath()` (needs backend segment)
- `verifyBinary()` (needs to look up the right variant's SHA256)
- `.variant` file writes

The design should specify that `AddonManager` gains a `backend string` field set during `EnsureAddon()`, and that `BinaryPath()`, `AddonDir()`, and `verifyBinary()` use it. Without this, implementers will either thread the backend through every method (verbose) or invent their own storage pattern (divergence risk).

### Gap 2: DetectBackend signature and config loading

The design shows `DetectBackend(configOverride string) string` but also says the config override comes from `llm.backend` in `$TSUKU_HOME/config.toml`. Who loads the config? The addon package currently has no dependency on `userconfig`. Two options:

1. The caller (`local.go` or `factory.go`) loads the config and passes the override string to `DetectBackend()`.
2. `DetectBackend()` imports `userconfig` and loads it itself.

Option 1 is correct per the existing dependency direction (`llm` -> `addon`, `llm` -> `userconfig`). The design should make this explicit to prevent someone from adding a `userconfig` import into the `addon` package, which would be a dependency direction violation (lower-level package importing higher-level config).

---

## 2. Are there missing components or interfaces?

### Missing: LLMConfig interface update

The `llm.Factory` accepts an `LLMConfig` interface (defined in `factory.go:22`):

```go
type LLMConfig interface {
    LLMEnabled() bool
    LLMLocalEnabled() bool
    LLMIdleTimeout() time.Duration
    LLMProviders() []string
}
```

The `userconfig.Config` struct implements this interface. Adding `llm.backend` to userconfig means adding `LLMBackend() string` to the interface so the factory can pass it through. The design doesn't mention this interface, but the factory is the natural place to thread the config value from userconfig down to the addon manager.

### Missing: userconfig.Get/Set for llm.backend

The design says `llm.backend` is set via `tsuku config set llm.backend cpu`. The `Get()` and `Set()` methods in `userconfig.go` use a switch statement for known keys. A new `llm.backend` case needs to be added to both methods, plus the `LLMConfig` struct needs a `Backend` field, plus `AvailableKeys()` needs the entry. This is straightforward but the design should enumerate it in Phase 2 step 4 ("Add `llm.backend` config key to userconfig") to avoid an incomplete implementation.

### Not missing but worth confirming: No CLI surface changes needed

The design correctly avoids adding new CLI subcommands. `tsuku config set llm.backend cuda` works through the existing `config set` command once the userconfig key is registered. No CLI surface duplication.

---

## 3. Are the implementation phases correctly sequenced?

**Phase ordering is correct but Phase 1 scope is too large.**

Phase 1 combines manifest schema changes with legacy cleanup. These are both foundational but independent -- the manifest schema can change without removing `AddonPath()`, and `AddonPath()` can be refactored without changing the manifest schema. Combining them means Phase 1 touches both the data contract (manifest.json, Go types) and the caller contract (lifecycle.go, local.go). If either part has issues, the entire phase blocks.

**Recommendation**: Split Phase 1 into Phase 1a (legacy cleanup: remove `AddonPath()`, update `NewServerLifecycleWithManager()`) and Phase 1b (manifest schema + types). Phase 1a can ship and be verified independently. This reduces blast radius.

Phase 2 correctly depends on Phase 1 (needs the new manifest types to look up variants). Phase 3 (Rust error reporting) is correctly independent -- it can ship before or after Phase 2 since it's purely additive stderr output. Phase 4 (testing) should run throughout, not as a terminal phase, but that's a process concern not an architecture issue.

---

## 4. Is the variant tracking (.variant file) approach sound?

**Sound but slightly over-engineered for the initial scope. It works, but there's a simpler option the design should consider and reject explicitly.**

The `.variant` file solves this problem: `verifyBinary()` needs to know which variant's SHA256 to check against, and the selected backend might change between runs (user changes `llm.backend` config). Without tracking, the manager would re-run detection, get a different result, and fail verification against the wrong checksum.

**Simpler alternative**: Encode the backend in the directory path (`<version>/<backend>/tsuku-llm`), which the design already proposes. Then `verifyBinary()` can extract the backend from the path itself -- the parent directory name IS the variant. No `.variant` file needed.

The directory layout `$TSUKU_HOME/tools/tsuku-llm/0.2.0/vulkan/tsuku-llm` already contains the variant information. When `verifyBinary(path)` is called, it can do:

```go
backend := filepath.Base(filepath.Dir(path))  // "vulkan"
info, err := manifest.GetVariantInfo(PlatformKey(), backend)
```

This eliminates the `.variant` file entirely. The path is the source of truth.

The `.variant` file would only add value if the backend weren't already in the path, or if we needed metadata beyond the backend name. Since the design proposes both the directory layout AND the `.variant` file, one of them is redundant.

**Recommendation**: Drop the `.variant` file. Derive the variant from the directory structure. This removes a write-then-read coordination point and a file that could become stale.

---

## 5. Should AddonPath removal happen before or in parallel with manifest changes?

**Before. The design gets this right.**

`AddonPath()` at `manager.go:232` constructs a path without the backend segment. `NewServerLifecycleWithManager()` at `lifecycle.go:71` calls `AddonPath()` to set `addonPath`. If the manifest changes deploy first (new directory layout with backend segment), then `AddonPath()` would construct wrong paths and `NewServerLifecycleWithManager()` would break.

The design's proposed fix is also correct: `NewServerLifecycleWithManager()` should accept the binary path from `EnsureAddon()` rather than computing it independently. Looking at the call chain:

1. `local.go:39` - `NewLocalProviderWithTimeout()` creates an `AddonManager` and calls `NewServerLifecycleWithManager(socketPath, addonManager)`
2. `lifecycle.go:71` - `NewServerLifecycleWithManager()` calls `addon.AddonPath()` to get `addonPath`
3. `local.go:57` - `Complete()` calls `p.addonManager.EnsureAddon()` which downloads and returns the path
4. `local.go:62` - `Complete()` calls `p.lifecycle.EnsureRunning()` which uses the `addonPath` from step 2

There's a subtle timing issue here: `NewServerLifecycleWithManager()` sets `addonPath` at construction time (before download), but `EnsureAddon()` determines the actual path at download time. The current code works because the path is deterministic (version from manifest). With backends, the path depends on detection results, which aren't available at construction time.

The fix: `ServerLifecycle` should accept the binary path lazily (at `EnsureRunning` time, not at construction time). The simplest change: `NewServerLifecycleWithManager()` takes the manager and resolves the path when `EnsureRunning()` is called, after `EnsureAddon()` has already run. The design mentions this but doesn't spell out the timing dependency explicitly.

---

## 6. Directory layout analysis

**The proposed layout `$TSUKU_HOME/tools/tsuku-llm/<version>/<backend>/` is sound.**

Current layout:
```
$TSUKU_HOME/tools/tsuku-llm/<version>/tsuku-llm
```

Proposed layout:
```
$TSUKU_HOME/tools/tsuku-llm/<version>/<backend>/tsuku-llm
```

This is a clean extension. The version dimension already exists; adding the backend dimension underneath it is the right ordering because:

1. Version upgrades are more common than backend switches. Cleaning up old versions means deleting `<version>/` directories, which correctly removes all backend variants for that version.
2. The lock file (`<version>/<backend>/.download.lock`) scopes correctly -- two different backends can download concurrently without contention.
3. Disk usage is bounded: at most 3 variants per version (CUDA, Vulkan, CPU) on Linux. Most users will have 1.

One concern: the existing download lock path is `filepath.Join(dir, ".download.lock")` where `dir = filepath.Dir(destPath)`. With the new layout, this resolves to `<version>/<backend>/.download.lock`. Two processes downloading different backends would use different lock files, which is correct (no false contention). Two processes downloading the same backend would share a lock file, which is also correct (prevents duplicate downloads).

---

## 7. Caller inventory verification

The design says "every function that looks up platform info" must change. Verified callers of the current manifest API:

| Caller | Location | Impact |
|--------|----------|--------|
| `GetCurrentPlatformInfo()` | `platform.go:25` | Must change to accept backend parameter |
| `EnsureAddon()` | `manager.go:111` | Calls `GetCurrentPlatformInfo()`, needs backend |
| `verifyBinary()` | `manager.go:156` | Calls `GetCurrentPlatformInfo()`, needs backend |
| `GetPlatformInfo()` tests | `manager_test.go:30,40` | Must update to new API |
| `NewServerLifecycleWithManager()` | `lifecycle.go:71` | Calls `addon.AddonPath()`, must change |

No callers in `cmd/tsuku/`. The addon package is only consumed by `internal/llm/`. This limits the blast radius.

**Confirmed**: the design correctly accounts for all callers. The `lifecycle.go:71` caller via `AddonPath()` is explicitly called out for cleanup.

---

## 8. Scope boundary assessment

### Correctly deferred: Vulkan VRAM detection

The Vulkan VRAM bug is a Rust-side issue in `hardware.rs`. Fixing it requires changes to the Rust binary, not the Go download/selection code. The design correctly scopes this out.

### Correctly deferred: Automatic runtime fallback

The design argues this adds `BackendFailedError` types, exit code changes, process monitoring, and retry logic. Looking at `waitForReady()` in `lifecycle.go:218`, the current implementation polls for socket availability with a 10-second timeout. Adding fallback would mean: detect failure type -> re-run `EnsureAddon()` with different backend -> restart process. This touches `EnsureRunning()`, `waitForReady()`, and `EnsureAddon()` -- three methods across two packages. Deferring is the right call for initial scope.

### Correctly deferred: Windows GPU support

Only a CPU variant exists for Windows. No detection logic needed.

### Benchmark gate: Actionable

The 25% threshold on tokens/second is concrete and measurable. The benchmark doesn't need infrastructure -- it's "run both variants on the same machine, compare throughput." The gate is: don't mark the design Current until this benchmark is done. This prevents shipping a default that's too slow without blocking the implementation work.

---

## Findings Summary

### Blocking

**1. Dependency direction risk for DetectBackend config loading.**
The design should explicitly state that the config override for `llm.backend` is loaded by the caller (in `internal/llm/`) and passed as a string to `DetectBackend()` in the `addon` package. Without this, an implementer could import `userconfig` into the `addon` package, creating a dependency direction violation (lower-level package importing higher-level config). The `addon` package currently has zero imports outside stdlib + the `progress` package.

**Severity**: Blocking. If someone adds `userconfig` to `addon`'s imports, every future `addon` consumer inherits the `userconfig` dependency, and the package can no longer be used independently.

### Advisory

**2. `.variant` file is redundant with the directory layout.**
The proposed directory layout already encodes the backend: `<version>/<backend>/tsuku-llm`. The `.variant` file duplicates this information. Deriving the backend from `filepath.Base(filepath.Dir(binaryPath))` is simpler and eliminates a file coordination point. If the design wants to keep the `.variant` file for forward compatibility (e.g., future metadata), it should state that rationale explicitly.

**Severity**: Advisory. The `.variant` file works and doesn't break anything, but it's unnecessary complexity that a future implementer might copy as a pattern for other metadata tracking when the path convention would suffice.

**3. Phase 1 scope is large; consider splitting.**
Phase 1 combines legacy cleanup (AddonPath removal, lifecycle refactor) with schema changes (manifest types, variant lookup). These are independent changes. Splitting reduces risk and makes review easier. Not blocking because the combined phase is still coherent and a single PR could handle it.

**Severity**: Advisory.

**4. Missing `LLMConfig` interface update and userconfig registration.**
Phase 2 step 4 says "Add `llm.backend` config key to userconfig" but doesn't mention the `LLMConfig` interface in `factory.go:22` that `userconfig.Config` implements. Adding `LLMBackend() string` to the interface is necessary for the factory to thread the value to the addon manager. The design should list this explicitly.

**Severity**: Advisory. An implementer reading the factory code will discover this naturally, but listing it prevents a partial implementation in the first PR.

**5. Lazy path resolution in ServerLifecycle.**
The design says `NewServerLifecycleWithManager()` should accept the binary path from the caller, but the real problem is timing: the binary path isn't known until `EnsureAddon()` runs (which happens in `Complete()`), but `ServerLifecycle` is constructed in `NewLocalProviderWithTimeout()` (before any `Complete()` call). The design should specify that `ServerLifecycle.addonPath` is set lazily, either by having `EnsureRunning()` accept a path parameter or by having the lifecycle query the manager.

**Severity**: Advisory. The design's direction is correct ("accept the binary path from `EnsureAddon()` instead of computing it independently") but the mechanism needs clarification to avoid a construction-time vs. use-time mismatch.

### Out of Scope

- Code style and naming choices (maintainer concern)
- Whether the 25% benchmark threshold is the right number (product decision)
- Test coverage completeness (tester concern)
- Whether Vulkan default is the right product choice (product decision)

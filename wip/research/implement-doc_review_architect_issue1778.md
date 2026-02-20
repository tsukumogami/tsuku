# Architect Review: Issue #1778

**Commit:** 63669ca89205d8f0178f0b96c5a538cecb1f6e0f
**Scope:** Migrate addon from embedded manifest to recipe system

## Summary

This commit replaces the addon's self-contained download/verify/platform-detection pipeline with delegation to the recipe system via an `Installer` interface. Five files are deleted (manifest.go, manifest.json, platform.go, download.go, verify.go), and the remaining `manager.go` shrinks to binary location + recipe delegation. The lifecycle layer (`lifecycle.go`, `local.go`) adapts to receive the binary path from `EnsureAddon()` rather than from the old manager.

The change respects the existing architecture. Dependency direction is correct: `internal/llm/addon` defines the `Installer` interface with zero imports outside stdlib, and higher-level packages will provide implementations. The pattern is consistent with how the codebase handles extensibility -- interface defined at the point of use, implementation wired from above.

## Blocking Findings (count: 0)

No blocking structural violations found.

**Dependency direction**: Correct. `internal/llm/addon` depends only on stdlib. `internal/llm` depends on `internal/llm/addon` (downward). No circular dependencies introduced.

**Interface design**: The `Installer` interface in `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/llm/addon/manager.go:19-24` has a single method `InstallRecipe(ctx, recipeName, gpuOverride)`. This maps cleanly to `executor.Executor.GeneratePlan()` + `ExecutePlan()` composition. The interface is defined at the consumer site (addon package), matching Go convention. No production implementation exists yet, but this is expected -- the wiring commit comes next.

**No parallel pattern**: The old download/verify pipeline is fully removed. There's no dual path for installing the addon. `NewLocalProvider()` explicitly documents (line 38-40 of `local.go`) that it doesn't drive installation -- callers handle that separately.

**Factory wiring**: `NewFactory()` at `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/llm/factory.go:171` uses `NewLocalProviderWithTimeout()` which creates an AddonManager with nil installer. This is intentional and documented: the factory-created LocalProvider only locates pre-installed binaries. `NewLocalProviderWithInstaller()` exists for callers that need install capability. This is not a split pattern -- it's two valid construction modes for different contexts (fallback lookup vs. install-capable).

**Deleted file cleanup**: All references to `Manifest`, `DownloadAddon`, `VerifyAddon`, `VerifyBeforeExecution`, and `NewServerLifecycleWithManager` are gone. No orphan call sites.

## Advisory Findings (count: 2)

### 1. `isGPUVariantInstalled()` always returns true

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/llm/addon/manager.go:169-182`

The method always returns `true`, meaning every new `AddonManager` instance with `backendOverride == "cpu"` will trigger a reinstall on the first `EnsureAddon()` call, even if the CPU variant is already installed. The per-session cache (line 85-91) prevents repeated reinstalls within a single process lifetime, but across process restarts the redundant install recurs.

The comment at lines 170-180 documents this as intentional: directory names don't encode variant info, and reading `state.json` would create an unwanted dependency on `internal/install`. The method exists as an extension point for future improvement. This is contained -- no other code copies this pattern, and the recipe system's idempotency limits the blast radius.

**Advisory** because it doesn't create structural drift. The method signature and call site are correct; only the implementation body needs refinement.

### 2. Direct field assignment to `lifecycle.addonPath` from `local.go`

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/llm/local.go:80`

```go
p.lifecycle.addonPath = addonPath
```

`local.go` and `lifecycle.go` are in the same package (`llm`), so this compiles. However, `addonPath` is set at `NewServerLifecycle` construction time for all other call sites. Here, it's mutated after construction -- the `NewServerLifecycle(socketPath, "")` call on line 42 and 56 passes empty string, and the real path is injected later in `Complete()`. This works but creates a subtle temporal coupling: `EnsureRunning()` must not be called before `Complete()` sets `addonPath`.

This is internal to the `llm` package and has no external callers. A setter method (`lifecycle.SetAddonPath(path)`) would make the contract explicit, but the current approach has no structural impact beyond the package boundary.

**Advisory** because it's contained within a single package and doesn't affect the public interface.

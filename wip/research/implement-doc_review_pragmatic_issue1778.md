# Pragmatic Review: Issue #1778

refactor(llm): migrate addon from embedded manifest to recipe system

## Blocking Findings (count: 2)

### B1. Dead code: `NewLocalProviderWithInstaller` has zero callers

`/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/llm/local.go:53` -- `NewLocalProviderWithInstaller()` is defined but never called anywhere in the codebase. The factory at `factory.go:171` uses `NewLocalProviderWithTimeout`. This is speculative generality; it creates an exported constructor that no current code path exercises.

**Fix**: Remove `NewLocalProviderWithInstaller`. When a downstream issue needs to wire an installer into the factory, add it then.

### B2. `isGPUVariantInstalled()` is a one-line `return true` behind a named method

`/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/llm/addon/manager.go:169` -- `isGPUVariantInstalled()` always returns `true`. The 14-line doc comment explains why it can't distinguish variants, then concludes "always reinstall." This is an unnecessary abstraction around a boolean constant. The comment belongs at the call site.

**Fix**: Inline the comment at `manager.go:97-104` and replace `m.isGPUVariantInstalled()` with `true`. Delete the method. The call site becomes self-documenting: "if backend=cpu and any binary exists, reinstall (can't distinguish variants by directory name)."

## Advisory Findings (count: 3)

### A1. `HomeDir()` getter has no production callers

`/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/llm/addon/manager.go:65` -- `HomeDir()` is only called in `manager_test.go` to verify constructor behavior. No production code reads it. Minor -- the tests could assert on `findInstalledBinary()` behavior instead, but the method is small and harmless.

### A2. `mockInstaller.onInstall` takes an unused `homeDir` parameter

`/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/llm/addon/manager_test.go:19` -- `onInstall func(homeDir string)` is always called with `""`. Every test uses a closure over `tmpDir` instead. The parameter is dead weight but confined to test code.

### A3. Variant mismatch always triggers reinstall even when already on CPU

`/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/llm/addon/manager.go:97-104` -- When `backendOverride == "cpu"`, every `EnsureAddon()` call that finds an existing binary will reinstall. This includes calls where the CPU variant is already correctly installed. The recipe system's idempotency mitigates this, but it's unnecessary work on every LLM invocation after the first install. The comment acknowledges this is "rare" (switching from GPU to CPU), but the reinstall happens on every cold start, not just when switching. Acceptable for now since recipe install is likely no-op fast, but worth noting.

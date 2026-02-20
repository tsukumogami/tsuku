# Maintainer Review: Issue #1778

Reviewer focus: Can the next developer understand and modify this code confidently?

## Blocking Findings (count: 3)

### 1. Direct field access bypasses encapsulation and mutex -- `local.go:80`

```go
// local.go:80
p.lifecycle.addonPath = addonPath
```

`ServerLifecycle.addonPath` is a lowercase (unexported-to-package-but-shared) field protected by `ServerLifecycle.mu`. The `Complete()` method in `local.go` writes to it directly without holding the mutex. Meanwhile, `EnsureRunning()` reads `s.addonPath` under its own mutex on lines 124-126 and 155-163. This is a data race: concurrent `Complete()` calls can write `addonPath` while `EnsureRunning()` reads it.

Beyond the race, this creates an implicit contract: `LocalProvider` must set `lifecycle.addonPath` before calling `lifecycle.EnsureRunning()`. Nothing in the `ServerLifecycle` API signals this requirement. The next developer looking at `NewServerLifecycle(socketPath, addonPath)` will reasonably think the addon path is set at construction time and is immutable -- which is exactly how the lifecycle tests use it (lines 17-19 of `lifecycle_test.go`). Then they'll look at `local.go` and see a field being mutated between calls.

**Fix**: Add a `SetAddonPath(path string)` method on `ServerLifecycle` that acquires the mutex. Or better, pass the addon path into `EnsureRunning()` as a parameter so the contract is explicit in the function signature.

### 2. `isGPUVariantInstalled()` always returns true -- name-behavior mismatch -- `manager.go:169-182`

```go
func (m *AddonManager) isGPUVariantInstalled() bool {
    // [15 lines of comments explaining why we can't actually check]
    return true
}
```

The function name asks "is a GPU variant installed?" The answer is always "yes." The call site at line 98 reads:

```go
if binaryPath != "" && m.backendOverride == "cpu" {
    if m.isGPUVariantInstalled() {
```

The next developer will read this conditional and think "we only reinstall when we detect a GPU variant." They won't realize the check is a no-op. When they need to understand why every `backend=cpu` user reinstalls on every session, they'll debug the wrong branch.

The long comment explaining the rationale is good content, but it belongs at the call site, not hidden inside a function whose name contradicts its behavior. As written, the function is dead indirection -- the `if m.isGPUVariantInstalled()` branch is always taken, making the conditional misleading.

**Fix**: Inline the comment at the call site and remove the function. The code should read:

```go
if binaryPath != "" && m.backendOverride == "cpu" {
    // Can't distinguish GPU vs CPU variant by directory name alone (both install
    // to tsuku-llm-<version>/). Conservative: always reinstall when backend=cpu.
    // This is rare (only when switching from GPU to CPU) and the recipe system
    // handles idempotency.
    slog.Info(...)
    if err := m.installViaRecipe(ctx); err != nil { ... }
```

### 3. `isRunningLocked` comment says the opposite of what it does -- `lifecycle.go:92`

```go
// isRunningLocked checks daemon state without holding the mutex.
func (s *ServerLifecycle) isRunningLocked() bool {
```

The Go convention for `*Locked` suffix means "caller must hold the lock" (the function itself does NOT acquire it). The comment "without holding the mutex" describes the opposite situation. The actual semantics are correct for Go convention -- callers like `IsRunning()` at line 86 and `EnsureRunning()` at line 119 do hold the mutex before calling this. But the comment will make the next developer think this function is safe to call without the mutex, when in fact calling it without the mutex could race with `EnsureRunning`'s lock file logic.

**Fix**: Change the comment to: `// isRunningLocked checks daemon state. Caller must hold s.mu.`

## Advisory Findings (count: 4)

### 4. Factory doesn't wire `LLMBackend()` into `LocalProvider` -- `factory.go:171`

```go
provider := NewLocalProviderWithTimeout(o.idleTimeout)
```

The factory creates `LocalProvider` via `NewLocalProviderWithTimeout`, which internally creates an `AddonManager` with `backendOverride=""` and `installer=nil`. The `NewLocalProviderWithInstaller` constructor exists specifically to wire these, but the factory doesn't use it. This means:

- `llm.backend=cpu` in config is never propagated to the factory's local provider
- The installer is nil, so `EnsureAddon()` will fail with "no installer configured" if the binary isn't already present

The commit's `NewLocalProviderWithInstaller` is dead code -- nothing calls it. The next developer might assume the factory handles this and wonder why `backend=cpu` doesn't work. This is likely intentional staging (wired in a later issue), but a `// TODO: wire installer and backendOverride via factory in issue #NNNN` comment would prevent confusion.

### 5. `mockInstaller.onInstall` signature is misleading -- `manager_test.go:19`

```go
onInstall func(homeDir string)
```

The field is typed as `func(homeDir string)` but line 31 always calls it with `""`:

```go
m.onInstall("")
```

The comment says "The test doesn't know the homeDir; callers set onInstall with closure over it." So the parameter is never used and every test closure ignores it (e.g., line 85: `func(_ string)`). The signature suggests `onInstall` receives useful information; it doesn't. This is minor but will confuse someone writing new tests who tries to use the parameter.

**Fix**: Change to `onInstall func()` and update call sites.

### 6. `EnsureAddon` reinstalls on every call when `backend=cpu` -- performance trap

Because `isGPUVariantInstalled()` always returns true, every call to `EnsureAddon` with `backendOverride="cpu"` triggers `installViaRecipe()` even when the CPU variant is already installed. The caching at line 85-91 helps for subsequent calls in the same session, but only until the binary path is first resolved. The flow is:

1. First call: `findInstalledBinary()` finds existing binary
2. `backendOverride == "cpu"` and `isGPUVariantInstalled() == true`, so reinstall
3. After reinstall, `findInstalledBinary()` finds the binary again
4. Cache it in `cachedPath`
5. Second call: returns cached path (no reinstall)

So it reinstalls exactly once per `AddonManager` instance lifetime. This is acceptable but non-obvious. A brief comment like `// Reinstalls once per session when backend=cpu (conservative: can't detect installed variant)` at line 97 would help.

### 7. `cleanupLegacyPath` could accidentally delete a recipe-installed tool -- `manager.go:207`

```go
filepath.Join(m.homeDir, "tools", "tsuku-llm"), // old non-versioned layout
```

The legacy cleanup targets `$TSUKU_HOME/tools/tsuku-llm` (no version suffix). The recipe system installs to `$TSUKU_HOME/tools/tsuku-llm-<version>`. These are distinct paths, so no current collision. But if a future recipe or version scheme ever produces a directory named exactly `tsuku-llm` (no version), this cleanup would delete it. The comment on line 207 partially explains this, but a defensive check (e.g., verify the directory doesn't contain a `.tsuku-recipe-marker` or check that `tsuku-llm-*` directories also exist) would be more resilient. Low risk since the naming convention is well-established.

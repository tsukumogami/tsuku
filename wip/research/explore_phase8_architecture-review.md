# Architecture Review: Secrets Manager Design

**Reviewer**: architect-reviewer
**Date**: 2026-02-16
**Design**: `docs/designs/DESIGN-secrets-manager.md`
**Status**: Proposed

---

## 1. Is the Architecture Clear Enough to Implement?

**Verdict: Yes, with two gaps that need clarification before implementation.**

The design is well-structured. The component diagram, data flow, key interfaces, and phasing are all concrete. A developer could start Phase 1 from the design as-is. Two areas need more detail:

### Gap A: `secrets.Get()` Dependency on `userconfig.Load()`

The design proposes `secrets.Get()` as a package-level function (not a method on a struct). The resolution path calls `userconfig.Load()` on every invocation to check the config file fallback. This means:

- Every `secrets.Get()` call reads and parses `$TSUKU_HOME/config.toml` from disk.
- There's no way to inject a pre-loaded config for testing or to avoid repeated I/O.

The design should specify whether `Get()` caches the loaded config or accepts it as a parameter. The current package-level function signature (`Get(name string)`) hides the config dependency entirely, which makes the function hard to test without filesystem setup and imposes unnecessary I/O per call in hot paths (e.g., `NewFactory` calls `secrets.IsSet()` multiple times).

**Recommendation**: Define a `Manager` struct that holds a reference to the loaded config, with a package-level default for convenience:

```go
type Manager struct {
    config *userconfig.Config
}

func New(cfg *userconfig.Config) *Manager { ... }
func (m *Manager) Get(name string) (string, error) { ... }

// Package-level convenience (loads config once)
var defaultManager *Manager
func Get(name string) (string, error) { return Default().Get(name) }
```

This matches the pattern used by `log/slog` (default + instance) and keeps the design testable. The design's public API signature remains unchanged for callers.

### Gap B: Search Factory Integration Pattern

The design lists `internal/search/factory.go` among files to change but doesn't specify the pattern. Currently `search.NewSearchProvider()` both resolves the key and constructs the provider in one call:

```go
// Current pattern in internal/search/factory.go
key := os.Getenv("TAVILY_API_KEY")
if key == "" {
    return nil, fmt.Errorf("--search-provider=tavily requires TAVILY_API_KEY")
}
return NewTavilyProvider(key), nil
```

With `secrets.Get()`, the value is returned (not just checked). The migration is straightforward but the design should state this explicitly since the search factory uses the key value directly (unlike the LLM factory which just checks presence).

---

## 2. Are There Missing Components or Interfaces?

### 2a. Missing: `secrets.GetValue()` vs `secrets.Get()` Semantics -- Advisory

The design's `Get()` returns `(string, error)`, which works for both "get value" and "check if set" use cases. But callers like `cmd/tsuku/config.go` need to display `(set)` / `(not set)` without the value. The design provides `IsSet()` for this, which is good.

However, the config CLI's `Get` subcommand for secrets keys needs special handling: `tsuku config get secrets.anthropic_api_key` should show `(set)` not the actual key value. The design mentions this in the summary but the `userconfig.Get()` method's current switch-based dispatch (lines 220-244 of `userconfig.go`) needs a clear specification for how `secrets.*` keys route through. Currently `Get()` returns the actual value string. For secrets, it must return a redacted representation.

**Recommendation**: Specify that `userconfig.Get()` returns `"(set)"` / `"(not set)"` for `secrets.*` keys, and add a `userconfig.GetSecret()` or similar for internal callers that need the real value. Or, more consistently, have `secrets.Get()` be the only path to actual values and have `userconfig.Get("secrets.*")` always return status only.

### 2b. Missing: GITHUB_TOKEN Usage Scope is Broader Than Design Accounts For -- Blocking

The design lists these call sites for migration:
- `internal/llm/claude.go`
- `internal/llm/gemini.go`
- `internal/llm/factory.go`
- `internal/discover/llm_discovery.go`
- `cmd/tsuku/config.go`

But the grep results show `GITHUB_TOKEN` is read from environment in **at least six additional locations**:

| File | Line | Usage |
|------|------|-------|
| `internal/builders/github_release.go` | 830 | HTTP auth header for GitHub API |
| `internal/version/resolver.go` | 80 | Auth for version resolution API calls |
| `internal/version/provider_tap.go` | 168 | Auth for Homebrew tap API calls |
| `internal/discover/validate.go` | 78 | `NewGitHubValidator` constructor |
| `internal/discover/llm_discovery.go` | 798 | `defaultHTTPGet` helper |
| `internal/version/assets.go` | (multiple) | Error messages referencing GITHUB_TOKEN |

The design must either:
1. **Expand the migration scope** to cover all `GITHUB_TOKEN` call sites, or
2. **Explicitly exclude** certain call sites with rationale (e.g., "builders and version providers will migrate in a follow-up")

Leaving some call sites on `os.Getenv()` and others on `secrets.Get()` creates two resolution paths for the same secret, which directly contradicts the design's goal of "single resolution path."

**Recommendation**: Phase 3 should include a complete inventory. The design should state whether Phase 3 migrates all call sites or just the LLM-related ones, with a follow-up phase for the rest. If partial, document which sites remain and why.

### 2c. Missing: Error Message Consistency for Unmigrated Sites -- Advisory

Several files contain hardcoded strings like `"Set GITHUB_TOKEN environment variable"` in error messages (e.g., `internal/version/errors.go:236`, `internal/builders/errors.go:84`). After migration, these messages should say "Set GITHUB_TOKEN environment variable or add github_token to [secrets] in $TSUKU_HOME/config.toml." The design should note that error message strings in non-migrated code need updating even if the resolution call site doesn't change.

---

## 3. Are the Implementation Phases Correctly Sequenced?

**Verdict: Yes. The phasing is well-designed.**

- **Phase 1** (env-only resolution) is independently useful and testable. It centralizes the scattered `os.Getenv()` patterns into one place without requiring config file changes. Good.
- **Phase 2** (config file integration) builds on Phase 1 by adding the fallback path. The userconfig changes are isolated. Good.
- **Phase 3** (caller migration) is the riskiest phase but correctly comes last, after the infrastructure is solid.

**One sequencing concern**: Phase 3 lists updating `cmd/tsuku/config.go` alongside the LLM provider migrations. The config CLI changes are user-facing and should be tested as a separate sub-step. Consider splitting Phase 3 into:
- **3a**: Internal caller migration (llm, discover, search, builders, version)
- **3b**: CLI integration (`tsuku config set/get secrets.*`)

This lets you ship internal consistency first and CLI polish second.

---

## 4. Are There Simpler Alternatives We Overlooked?

### 4a. Considered and Correct: No Separate Secrets File

The design correctly rejected a separate `secrets.toml`. The permission tension with `config.toml` is minor, and having one config file is simpler for users. Agree.

### 4b. Alternative Not Considered: Dependency Injection Instead of Package-Level Functions

The design uses package-level functions (`secrets.Get()`, `secrets.IsSet()`). This works but creates a hidden global dependency. The existing `llm.Factory` already accepts configuration via `LLMConfig` interface and `FactoryOption` functions. The `search.NewSearchProvider()` and `discover.NewGitHubValidator()` accept explicit parameters.

A dependency-injection approach would pass a `secrets.Resolver` interface (or concrete `*secrets.Manager`) to constructors:

```go
// Instead of: secrets.Get("anthropic_api_key") inside NewClaudeProvider
// Do: NewClaudeProvider(apiKey string) -- already the pattern for search providers

// Or for factory-level:
func NewFactory(ctx context.Context, resolver secrets.Resolver, opts ...FactoryOption) (*Factory, error)
```

This is a heavier refactor but would make the dependency graph explicit and testing trivial. The current design's package-level functions are pragmatic and match how `os.Getenv()` works (which is what's being replaced), so this is a style preference rather than a blocking concern.

**Verdict**: The package-level function approach is acceptable for now. If testing becomes painful, refactor to DI later.

### 4c. Alternative Not Considered: Lazy One-Time Config Load

Rather than loading config on every `Get()` call (expensive) or requiring explicit initialization (ceremony), the package could load config lazily on first access with `sync.Once`:

```go
var (
    loadOnce sync.Once
    loaded   *userconfig.Config
    loadErr  error
)

func getConfig() (*userconfig.Config, error) {
    loadOnce.Do(func() {
        loaded, loadErr = userconfig.Load()
    })
    return loaded, loadErr
}
```

This is simple, efficient, and matches Go idioms. The tradeoff is that config changes after first load won't be picked up, but that's acceptable since secrets don't change during a single CLI invocation.

**Recommendation**: Specify this pattern in the design. It addresses the repeated I/O concern without adding a `Manager` struct.

---

## 5. Does the Design Fit the Existing Codebase Patterns?

### 5a. Pattern Consistency: Good

The design follows established patterns:
- **TOML config with struct tags**: Matches `userconfig.Config` pattern (line 18-25 of `userconfig.go`)
- **Package under `internal/`**: Consistent with `internal/llm`, `internal/search`, `internal/discover`
- **Factory-style resolution**: The `knownKeys` map with `KeySpec` entries mirrors how `search.NewSearchProvider()` dispatches by name
- **Env var precedence over config**: Matches `userconfig.LLMIdleTimeout()` which checks env var first (lines 177-191)

### 5b. Pattern Inconsistency: `saveToPath` Atomic Write -- Advisory

The design proposes atomic writes (temp file + rename) for `saveToPath`. Currently `saveToPath` uses `os.Create()` directly (line 132 of `userconfig.go`). The design correctly identifies this needs to change, but the atomic write pattern should be extracted as a helper since other future config writes may need it. Don't inline the temp-file logic in `saveToPath`.

**Recommendation**: Create a small helper like `internal/fsutil/atomic.go` or add an `atomicWrite(path string, mode os.FileMode, fn func(io.Writer) error) error` function. This avoids coupling the atomic write concern to userconfig specifically.

### 5c. Pattern Inconsistency: `Get/Set` String Dispatch -- Advisory

The current `userconfig.Get()` and `userconfig.Set()` use a `switch` statement on key strings (lines 220-318). Adding `secrets.*` keys will extend this switch with a dynamic prefix match, breaking the pattern. Today each key is a literal case; secrets keys are an open set (`secrets.anthropic_api_key`, `secrets.google_api_key`, etc.).

**Recommendation**: The design should specify that `Get`/`Set` handle the `secrets.` prefix as a special case that delegates to the `Secrets` map, rather than adding individual cases per secret. Something like:

```go
case strings.HasPrefix(key, "secrets."):
    secretName := strings.TrimPrefix(key, "secrets.")
    // delegate to secrets map
```

This is implicit in the design but should be explicit to prevent an implementer from adding individual switch cases.

### 5d. Coupling Concern: `secrets` Package Importing `userconfig` -- Blocking

The design has `internal/secrets` calling `userconfig.Load()` to read the config file. Meanwhile, `userconfig.Config` will gain a `Secrets map[string]string` field. This creates a dependency:

```
secrets -> userconfig (to load config and read Secrets map)
userconfig -> (no dependency on secrets)
```

This is a one-way dependency, which is fine. But consider what happens when `cmd/tsuku/config.go` routes `secrets.*` keys: it will call `userconfig.Set("secrets.foo", value)` which writes to `config.Secrets`, then `userconfig.Save()`. The `secrets` package itself is not involved in the write path at all -- it's read-only.

This separation is actually clean: `userconfig` owns the config file (read/write), `secrets` owns resolution logic (read-only, env + config). The concern would be if someone adds a `secrets.Set()` that writes directly, creating a second write path. The design should state explicitly that **secrets is a read-only resolution layer; all writes go through userconfig**.

---

## Summary of Findings

### Blocking (must address before implementation)

| # | File/Section | Issue | Impact |
|---|-------------|-------|--------|
| 1 | Design, "Changes to Existing Code" section | Incomplete migration inventory for `GITHUB_TOKEN`. At least 6 additional call sites in `builders/`, `version/`, `discover/` are not listed. | Two resolution paths for the same secret. Inconsistent error messages. Contradicts "single resolution path" goal. |
| 2 | Design, "Key Interfaces" section | `secrets.Get()` hides its dependency on `userconfig.Load()`. No specification for whether config is loaded per-call, cached, or injected. | Repeated disk I/O on every call, untestable without filesystem setup, unclear lifecycle. |

### Advisory (improvement opportunities)

| # | File/Section | Issue | Suggestion |
|---|-------------|-------|------------|
| 3 | Design, "Config CLI Integration" | No specification for how `userconfig.Get()`/`Set()` handles `secrets.*` prefix vs existing literal switch cases. | Specify prefix-based delegation pattern. |
| 4 | Design, Phase 3 | Config CLI changes bundled with internal migrations. | Split into 3a (internal) and 3b (CLI). |
| 5 | Design, "Atomic Writes" | Atomic write logic inlined in `saveToPath`. | Extract to reusable helper. |
| 6 | Design, "Consequences" | No explicit statement that `secrets` package is read-only. | Add to design to prevent future write-path duplication. |
| 7 | Design, "Changes to Existing Code" | Error message strings in `version/errors.go`, `builders/errors.go` reference only env vars. | Note that guidance strings need updating to mention config file option. |

---

## Verdict

The design is solid and fits the codebase well. The core abstraction (a thin resolution layer with env-var priority) is the right level of complexity. The two blocking items -- incomplete migration scope and unspecified config loading lifecycle -- should be resolved before implementation begins, but neither requires rethinking the architecture. They're specification gaps, not design flaws.

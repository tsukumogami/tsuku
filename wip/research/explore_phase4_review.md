# Architect Review: DESIGN-secrets-manager.md

**Reviewer**: architect-reviewer
**Date**: 2026-02-16
**Design**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/docs/designs/DESIGN-secrets-manager.md`

---

## 1. Problem Statement Assessment

**Is the problem statement specific enough to evaluate solutions against?**

The problem statement identifies three concrete issues (decentralized resolution, env-var-only storage, permissive file modes) and grounds each in code. This is strong. However, there are gaps:

### What's specific and verifiable

- The claim that each provider reads keys via `os.Getenv()` in its own constructor is confirmed. `claude.go:22`, `gemini.go:24-26`, `search/factory.go:17,24,35,38` all do this independently.
- The claim that error messages vary is confirmed. Compare `claude.go:24` ("ANTHROPIC_API_KEY environment variable not set") with `gemini.go:29` ("GOOGLE_API_KEY (or GEMINI_API_KEY) environment variable not set") with `factory.go:175` ("no LLM providers available: set ANTHROPIC_API_KEY or GOOGLE_API_KEY, or enable local LLM").
- The `os.Create()` claim about `config.toml` permissions is confirmed at `userconfig.go:133`.

### What's underspecified

**The scope boundary with `GITHUB_TOKEN` is ambiguous.** The problem statement mentions `GITHUB_TOKEN` in discovery but then the scope section only lists API keys for LLM providers. Meanwhile, `GITHUB_TOKEN` appears in at least five packages: `version/resolver.go:80`, `version/provider_tap.go:168`, `discover/llm_discovery.go:798`, `discover/validate.go:78`, and `builders/github_release.go:830`. The same ad-hoc `os.Getenv()` pattern exists there. If the secrets manager is supposed to centralize key resolution, the design must state whether `GITHUB_TOKEN` is in or out -- and if out, why. Currently this is implied but never stated.

**Search provider keys are not mentioned.** `search/factory.go` reads `TAVILY_API_KEY` and `BRAVE_API_KEY` using the exact same scattered pattern. These are API keys for external services. The design should explain whether they're covered or excluded.

**The "config file optional" driver contradicts the problem statement.** Problem #2 says "some users prefer storing keys in a config file." But the decision driver says "config file optional." These aren't contradictory in principle, but the design never clarifies: if the file doesn't exist and no env var is set, does the error message tell the user about both options? The scope says "consistent error guidance" but doesn't show what that guidance looks like.

---

## 2. Missing Alternatives

### Alternative: Separate resolution interface without a `[secrets]` section

The design jumps from "centralized resolution" to "where to store secrets persistently." But there's a zero-storage alternative that solves problem #1 and #3 without touching problem #2: create an `internal/secrets` package with a `Get(name string) (string, error)` function that wraps `os.Getenv()` with consistent error messages and conventional name mapping, but doesn't add file storage at all. File storage could be added later as a fallback in the same resolution chain.

This matters because the highest-value part of the design is the centralized interface (eliminating 15+ scattered `os.Getenv()` calls). That can ship independently of the config file changes, which carry the permission enforcement complexity. By coupling them, the design risks delaying the simpler high-value change.

**Recommendation**: Consider whether the design should be split into two phases -- (1) centralized resolution interface reading only env vars, (2) config file fallback with permission enforcement. This doesn't require separate design documents, just explicit phasing in the implementation plan.

### Alternative: OS keychain integration as a named future option

The design mentions "external secret manager integration (keychain, 1Password, pass)" as out of scope. This is fine. But it's worth stating whether the interface design should accommodate pluggable backends. If `Get(name string) (string, error)` is the interface, a keychain backend could be added later without API changes. If the interface is tightly coupled to TOML parsing, it's harder. The current design doesn't address this -- it says "future work" without confirming the chosen design won't make future work harder.

---

## 3. Rejection Rationale Fairness

### Decision 1: Storage Location

**`secrets.toml` rejection -- fair but incomplete.** The rationale says it "complicates the `tsuku config` CLI (which file does `tsuku config set` target?)." This is a real concern. But it understates the advantage: separate files let you have different permission models cleanly. The design acknowledges this ("avoids the permission tension entirely") and then dismisses the tension as "minor." I'd strengthen this by being explicit: tightening `config.toml` to 0600 means non-secret settings like `telemetry = false` also become owner-only readable. In practice this doesn't matter for a single-user tool, but it should be stated.

**Encrypted file rejection -- fair.** The complexity argument is sound for a tool whose threat model is local file permissions, not at-rest encryption. No issues here.

### Decision 2: Permission Strategy

**"Refuse to read" rejection -- fair.** The workflow described (set key, then next read fails) is a real UX problem.

**"Always enforce silently" rejection -- arguably a strawman.** The rationale says "silent changes to file permissions are surprising." But the chosen option ("warn and tighten on write") also tightens permissions -- it just logs first. The meaningful distinction is "tighten on read vs. tighten on write," not "silent vs. warned." The real reason to prefer write-time enforcement is that reads should be side-effect-free. The design would be stronger if it framed the rejection that way.

---

## 4. Unstated Assumptions

### Assumption: config.toml is always single-user

The permission enforcement strategy assumes `config.toml` is only accessed by the owning user. This is true for `$TSUKU_HOME` in `~/.tsuku/`, but `TSUKU_HOME` can be overridden to a shared directory. If a team sets `TSUKU_HOME=/opt/tsuku`, tightening to 0600 breaks other users' ability to read non-secret config. The design should note that `[secrets]` section with file-level 0600 is incompatible with shared `$TSUKU_HOME` directories, or document that secrets in shared installations should use env vars only.

### Assumption: TOML key naming convention is sufficient

The design says "the TOML key `anthropic_api_key` in `[secrets]` maps to the environment variable `ANTHROPIC_API_KEY` (uppercase with same name)." This works for `ANTHROPIC_API_KEY` and `GOOGLE_API_KEY` but breaks for `GEMINI_API_KEY` because the Gemini provider checks *two* env vars (`GOOGLE_API_KEY` first, then `GEMINI_API_KEY` as fallback). The mapping convention doesn't express "this secret can resolve from either of two env vars." The design needs to address multi-env-var providers.

### Assumption: Warning output goes to stderr

The design says "log a warning" about permissive files but doesn't specify where. The existing codebase uses `fmt.Fprintf(os.Stderr, ...)` for warnings (see `config/config.go:62,69,94,101`). The design should confirm this or specify an alternative. If the secrets manager uses a structured logger while the rest of the codebase uses stderr, that's a consistency issue.

### Assumption: config.toml is written atomically

The current `saveToPath` in `userconfig.go:125-144` opens the file, encodes, and closes. If the process crashes mid-write, the file is corrupted. Adding secrets to this path means a crash could leave secrets partially written. The design doesn't mention atomic writes (write-to-temp-then-rename), but it should, because the data is now more sensitive.

---

## 5. Strawman Assessment

**No option is a strawman.** All three storage options (same file, separate file, encrypted) represent genuine trade-offs. The encrypted option is the weakest contender but is included for completeness rather than to make others look good by comparison. The permission enforcement alternatives are also legitimate.

However, the "always enforce silently" option (Decision 2) is closer to a strawman than the others. The real distinction -- side effects on read vs. side effects on write -- is obscured by framing it as "silent vs. warned." A more honest framing would make the chosen option's advantage clearer without needing a weaker alternative.

---

## 6. Architectural Fit Assessment

### Positive alignment

- **Uses existing TOML infrastructure.** The design builds on `BurntSushi/toml` already in use, extending `userconfig.Config` rather than introducing a new config system.
- **Follows the pattern of `config` vs. `userconfig`.** System paths live in `internal/config`, user preferences in `internal/userconfig`. Adding secrets to `userconfig` is the right layer.
- **Centralized interface removes coupling.** The 15+ call sites currently import `os` directly for key resolution. A single `secrets.Get()` function removes this coupling.

### Concerns

**The design doesn't show the interface.** It describes behavior (resolution order, permission enforcement) but never shows the Go function signature. This matters because the interface determines how well the design fits the existing Factory pattern. Currently, `NewFactory` in `factory.go:125` calls `os.Getenv("ANTHROPIC_API_KEY")` directly. The refactoring needs to either:

  (a) Pass a `SecretsResolver` interface into `NewFactory` via `FactoryOption`, or
  (b) Have `NewClaudeProvider()` and `NewGeminiProvider()` accept explicit API key parameters instead of reading env vars themselves.

Option (a) keeps the providers unaware of the resolution mechanism. Option (b) pushes resolution to the caller. The design should pick one. Currently, the provider constructors (`claude.go:21-31`, `gemini.go:23-40`) both read env vars internally, which is the anti-pattern being fixed. The design should state whether providers will accept keys as parameters or use a shared resolver.

**Interaction with `search/factory.go` is unaddressed.** The search factory at `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/search/factory.go` does the same `os.Getenv()` pattern for `TAVILY_API_KEY` and `BRAVE_API_KEY`. If the secrets manager doesn't cover these, there will be two key resolution patterns: the new centralized one for LLM keys, and the old scattered one for everything else. This is a layering concern -- the design creates a new abstraction but only applies it partially.

**`GITHUB_TOKEN` creates a three-way split.** Even if search provider keys are excluded, `GITHUB_TOKEN` is used across `version/`, `discover/`, and `builders/` packages. Without clear scope boundaries, implementers will either over-apply the secrets manager (touching packages outside the design's scope) or leave an inconsistent codebase where some secrets go through the manager and others don't.

---

## 7. Summary of Findings

### Blocking

1. **Multi-env-var providers are unaddressed** (Section 4). The `GOOGLE_API_KEY`/`GEMINI_API_KEY` fallback pattern doesn't fit the stated TOML key naming convention. This will surface during implementation and force an ad-hoc decision. The design should define how providers with multiple possible env vars are represented in `[secrets]`.

2. **No interface definition** (Section 6). Without seeing `func Get(name string) (string, error)` or whatever the API is, we can't evaluate whether the design actually solves the stated problem. The interface determines how providers consume secrets, which is the core architectural question.

### Advisory

3. **Scope boundary for `GITHUB_TOKEN` and search API keys is unclear** (Sections 1, 6). The design risks creating two key resolution patterns that coexist indefinitely. Even if these are intentionally excluded, say so and explain why.

4. **Shared `$TSUKU_HOME` incompatibility** (Section 4). Tightening `config.toml` to 0600 breaks shared installations. Document this limitation or handle it.

5. **Atomic writes for sensitive data** (Section 4). The current `saveToPath` isn't crash-safe. Adding secrets to this path increases the cost of data corruption.

6. **"Always enforce silently" rejection is weakly framed** (Section 3). Reframe around "reads should be side-effect-free" rather than "silent is surprising."

7. **Consider phased implementation** (Section 2). The centralized resolution interface (env vars only) and the config file fallback are separable. Shipping them together couples high-value, low-risk work with medium-value, higher-risk work.

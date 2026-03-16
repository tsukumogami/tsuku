# Security Review: Distributed Recipes Design

## Scope

Review of `docs/designs/DESIGN-distributed-recipes.md`, focusing on the Security Considerations section and its interaction with the implementation design. Cross-referenced with existing security infrastructure in `internal/httputil/`, `internal/secrets/`, `internal/actions/download.go`, and `internal/discover/sanitize.go`.

---

## Finding 1: Arbitrary Code Execution via Recipe Actions (CRITICAL -- under-documented)

**Status: Attack vector not considered in Security Considerations section.**

The design's trust model paragraph says "Users who run `tsuku install owner/repo` are trusting that repository's maintainers with arbitrary file system access under their user account." This understates the actual risk. A distributed recipe has access to **all** action types, including:

- `run_command`: Executes arbitrary shell commands via `sh -c` (`internal/actions/run_command.go:104`)
- `cargo_build`, `go_build`, `configure_make`, `meson_build`: All execute arbitrary build commands
- `pip_install`, `npm_install`, `gem_install`: Install arbitrary packages from upstream registries
- `nix_realize`: Runs Nix expressions

This is not "file system access" -- it is unrestricted code execution under the user's account. The comparison to `go install` and `cargo install` is apt but incomplete: those tools compile source code, which at least in principle can be audited. A tsuku recipe's `run_command` action executes opaque shell commands with no compilation step. The obfuscation barrier is lower.

**Impact**: The trust warning ("This will execute recipe-defined actions with your user permissions") is technically correct but misleading by understatement. A user reading "recipe-defined actions" may not understand this means "arbitrary shell commands."

**Recommendation**: The trust warning should say "arbitrary commands" explicitly. The design doc should also document which action types are available to distributed recipes, or whether there should be an action allowlist for untrusted sources.

---

## Finding 2: No Action Sandboxing or Allowlisting for Distributed Sources (CRITICAL)

**Status: Not considered.**

Central registry recipes go through a PR review process. Distributed recipes have no review gate. Yet both have identical access to the full action set. The design does not consider restricting which actions distributed recipes can use.

At minimum, a distributed recipe should not be able to use `run_command` with unconstrained shell execution without explicit user consent (beyond the initial install confirmation). Actions like `group_add`, `service_enable`, and `service_start` (`internal/actions/system_config.go`) are also high-privilege operations that a distributed recipe could invoke.

**Recommendation**: Consider an action allowlist for distributed sources in a fast follow. Document this as a known gap in the design's Security Considerations. The current trust model delegates everything to the one-time install warning, which is insufficient for actions that modify system state.

---

## Finding 3: SSRF Protection Gap -- Initial Request vs. Redirects (MODERATE)

**Status: Partially mitigated by existing infrastructure.**

The design correctly specifies `httputil.NewSecureClient` for the distributed provider. This client has SSRF protection on redirects (`client.go:113-149`). However, the SSRF checks are only in the `CheckRedirect` function -- they validate redirect targets but not the initial request URL.

For the distributed provider, the initial URLs are constructed from `owner/repo` input, which maps to `api.github.com` and `raw.githubusercontent.com`. These are hardcoded hostnames so the initial request isn't vulnerable to SSRF. But the design says `download_url` values from the Contents API response should be validated for HTTPS. The concern is: what if GitHub's Contents API returns a `download_url` pointing to an unexpected host?

The existing `downloadFile` method (`download.go:351-352`) enforces HTTPS prefix. The `NewSecureClient` handles redirect-based SSRF. The gap is that there's no hostname allowlist for `download_url` values returned by the Contents API. A compromised or spoofed API response could return `download_url: "https://evil.com/malware.toml"` and the client would fetch it.

**Recommendation**: Add to the implementer requirements: validate that `download_url` hostnames match an allowlist (`raw.githubusercontent.com`, `objects.githubusercontent.com`). The design already says "Validate that `download_url` values use HTTPS" -- extend this to hostname validation.

---

## Finding 4: Token Leakage via download_url Following (MODERATE)

**Status: Partially addressed.**

The design says "Raw content fetches don't include authentication headers." This is the right intent but needs implementation specificity. If the distributed provider uses a single `http.Client` with a `Transport` that injects `Authorization` headers for all requests (a common pattern for GitHub API clients), then following a `download_url` to `raw.githubusercontent.com` would also send the token.

The existing `secrets` package (`secrets.go`) provides token retrieval but not injection. The concern is about the HTTP client construction in the new `internal/distributed/` package.

**Recommendation**: Add an explicit implementer requirement: the Contents API client (authenticated) and the raw content client (unauthenticated) must be separate `http.Client` instances, or the `Authorization` header must be set per-request rather than via `Transport`.

---

## Finding 5: owner/repo Input Validation Exists but Is Not Referenced (LOW)

`internal/discover/sanitize.go` already has `ValidateGitHubURL()` with `ownerRepoRegex`, path traversal detection, credential rejection, and port blocking. The design's name parsing section (lines 328-338) defines the input formats but doesn't reference this existing validation infrastructure.

**Recommendation**: The design should specify that `owner/repo` parsing reuses `discover.ValidateGitHubURL()` (or its core validation logic) rather than implementing a parallel parser. This is both a security concern (avoid duplicating validation) and an architectural concern (avoid parallel patterns).

---

## Finding 6: Recipe Hash in state.json -- Correct Trade-off, Missing Threat Model (LOW)

**Question from reviewer**: "The design stores recipe hash in state.json but doesn't act on changes in v1. Is this the right trade-off?"

**Assessment**: Yes, this is the right trade-off for v1. The hash creates an audit trail without introducing a blocking UX that would need careful design. The gap is that the design doesn't specify what happens in the threat scenario it's tracking:

1. User installs from `owner/repo`, recipe hash H1 is stored
2. Attacker compromises `owner/repo`, pushes malicious recipe
3. User runs `tsuku update <tool>`, fetches recipe with hash H2
4. H1 != H2, but v1 takes no action

The "future `--accept-recipe-changes` flow" is mentioned but not specified. Without knowing the planned behavior, there's no way to evaluate whether the hash format is sufficient. For example: should the hash cover just the TOML bytes, or should it include the branch/commit SHA?

**Recommendation**: Add a single sentence: "The recorded hash is `sha256(raw_toml_bytes)` of the recipe as fetched. Future versions will compare this hash on update and block execution if it changes, requiring `--accept-recipe-changes` or `tsuku trust <tool>` to proceed." This makes the intent concrete enough that v1 implementers store the right thing.

---

## Finding 7: Cache Poisoning via Shared Filesystem (LOW)

The cache layout at `$TSUKU_HOME/cache/distributed/{owner}/{repo}/` stores fetched recipe TOML. If another process (or a malicious tool installed by tsuku itself) can write to `$TSUKU_HOME`, it can modify cached recipes. On next use, the modified recipe executes with user permissions.

This is the same risk as the existing `$TSUKU_HOME/registry/` cache, so it's not a new vulnerability introduced by this design. The design's use of `sha256(recipe_toml_bytes)` in state.json could detect this if the hash were checked at execution time, but v1 doesn't do that.

**Recommendation**: No action needed for v1. When recipe hash checking is implemented, it should check cached TOML against the stored hash before execution, not just on fetch.

---

## Finding 8: Trust Warning -- Display Only vs. Confirmation Prompt

**Question from reviewer**: "Is the trust warning on first install sufficient, or should there be a confirmation prompt?"

**Assessment**: A display-only warning is insufficient for a tool that executes arbitrary code. The comparison to `go install` is instructive: Go doesn't show a warning because it compiles source code -- there's no expectation of safety from arbitrary modules. But tsuku positions itself as a curated package manager (central registry with PR review). Users may not expect `tsuku install owner/repo` to have a fundamentally different trust model than `tsuku install ripgrep`.

`strict_registries = true` addresses this for CI/teams, but interactive users in default mode get a warning they can miss in terminal scroll.

**Recommendation**: First install from a new distributed source should require explicit confirmation (`-y` / `--yes` to skip). This is a one-time cost per source and matches the pattern of `apt` adding a new repository. The design already has the auto-registration mechanism -- gate it behind a prompt.

---

## Finding 9: Telemetry Privacy -- Correct but Incomplete

The design says telemetry events should use an opaque "distributed" tag rather than `owner/repo`. This is good. But the state.json `Source` field stores the full `owner/repo`, and state.json is a local file. If tsuku ever adds state sync, diagnostics upload, or crash reporting, the `Source` field becomes a privacy leak.

**Recommendation**: Note in the design that `Source` is a local-only field and must not be included in any future telemetry or remote diagnostics payload.

---

## Summary of Residual Risk

| Risk | Severity | Mitigated in v1? | Recommendation |
|------|----------|-------------------|----------------|
| Arbitrary code execution via recipe actions | Critical | Partially (trust warning) | Confirmation prompt, not just warning |
| No action allowlist for distributed sources | Critical | No | Document as known gap; plan fast follow |
| download_url hostname not validated | Moderate | No | Add hostname allowlist to implementer requirements |
| Token leakage via shared HTTP client | Moderate | Not specified | Require separate clients for API and raw content |
| Input validation duplication risk | Low | Existing code available | Reference `discover.ValidateGitHubURL()` |
| Recipe hash format under-specified | Low | N/A (future feature) | Specify intended future behavior |
| Cache poisoning via filesystem | Low | No (matches existing risk) | Address when hash checking is implemented |

**Overall assessment**: The design's security section is honest about the trust model but underestimates the severity of what "recipe-defined actions" means in practice. The two critical gaps are: (1) no distinction between the privilege level of central vs. distributed recipes, and (2) the trust warning should be a confirmation prompt, not informational output. Network-level security (SSRF, HTTPS enforcement, token handling) builds well on existing infrastructure.

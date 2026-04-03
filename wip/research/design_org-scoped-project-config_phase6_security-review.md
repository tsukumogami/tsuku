# Security Review: DESIGN-org-scoped-project-config

## Scope

Review of `docs/designs/DESIGN-org-scoped-project-config.md` security considerations section. Evaluated against source code in `cmd/tsuku/install_distributed.go`, `cmd/tsuku/install_project.go`, `internal/project/config.go`, `internal/project/resolver.go`, `internal/index/rebuild.go`, and `internal/discover/sanitize.go`.

## 1. Attack Vectors Not Considered

### 1a. Malicious `.tsuku.toml` in cloned repositories (HIGH)

The design does not address the primary threat model: a developer clones a repository containing a `.tsuku.toml` with org-scoped tools pointing to an attacker-controlled GitHub org. Running `tsuku install` (or `tsuku install -y` as recommended for CI) would:

1. Auto-register the attacker's source in the user's `config.toml` (persistent state mutation)
2. Download and execute recipes from the attacker's repository
3. Install arbitrary binaries from those recipes

This is the supply-chain attack vector. The existing CLI path requires the user to type the org/repo name explicitly, which provides intent signal. The project config path removes that signal -- the `.tsuku.toml` becomes the sole authority.

**Existing mitigations and their gaps:**
- `strict_registries` blocks this, but it is opt-in and off by default
- The interactive confirmation prompt protects interactive users, but CI pipelines use `--yes`/`-y` which bypasses it
- `checkSourceCollision` only fires when replacing an existing tool from a different source, not on first install

**Recommendation:** The design should explicitly document this threat and consider:
- Warning when auto-registering sources from project config (distinct from CLI-initiated registration)
- A `--trust-config` flag that must be passed explicitly for project configs to auto-register new sources (separate from `--yes` which currently overloads confirmation-skip with trust-grant)
- At minimum, document that CI environments SHOULD enable `strict_registries` and pre-register sources

### 1b. Resolver name collision as a shadowing attack (MEDIUM)

The dual-key resolver uses first-match semantics for name collisions. The design acknowledges this in Consequences but doesn't analyze it as a security risk. An attacker could craft a `.tsuku.toml` that declares both `legitimate-org/tool` and `attacker-org/tool` where both provide a binary named `tool`. Since Go map iteration order is non-deterministic, the resolver could intermittently resolve to either org's version.

More concretely: if a project legitimately uses `acme/koto` and an attacker submits a PR adding `evil/koto` to `.tsuku.toml`, the resolver's `bareToOrg` map stores both entries. Which one wins depends on map iteration order, creating a non-deterministic shadowing attack.

**Recommendation:** The design should require that `splitOrgKey` detect duplicate bare names across different orgs during resolver construction and either error or warn. This is a straightforward check when building the `bareToOrg` map.

### 1c. TOML key injection via unicode or escape sequences (LOW)

TOML quoted keys support escape sequences (`\uXXXX`, `\UXXXXXXXX`). A malicious `.tsuku.toml` could use unicode escapes to construct key strings that appear as one thing in a text editor but parse differently. For example, right-to-left override characters could make `"evil/repo"` display as something benign.

**Recommendation:** Low practical risk since `validateRegistrySource` -> `validateOwnerRepo` -> `ownerRepoRegex` (`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`) rejects non-ASCII characters. This is sufficient. No action needed, but worth noting in the security section as an explicitly-covered case.

### 1d. Race condition in batch source registration (LOW)

The pre-scan calls `ensureDistributedSource` for each unique source sequentially. Each call loads user config, potentially modifies it, and saves. If two tsuku processes run concurrently (e.g., parallel CI jobs sharing a home directory), the config file writes could race, potentially losing a registration entry.

**Recommendation:** Low risk since CI typically uses isolated home directories. The existing `ensureDistributedSource` already has this race condition in the CLI path, so this is not a new attack surface. No action needed beyond noting it.

## 2. Sufficiency of Identified Mitigations

### 2a. Path traversal via `parseDistributedName` -- SUFFICIENT

The design correctly identifies that `parseDistributedName` rejects `..` patterns. Verified in source: line 43 of `install_distributed.go` checks `strings.Contains(name, "..")` and returns nil. Additionally, `validateRegistrySource` -> `validateOwnerRepo` enforces the `owner/repo` format with `ownerRepoRegex`, which only allows `[a-zA-Z0-9._-]` characters. The `isValidRecipeName` guard in `internal/index/rebuild.go` rejects `/` and `..` in bare names. The layered defense is solid.

### 2b. `strict_registries` enforcement -- SUFFICIENT but WEAK DEFAULT

The mitigation works correctly: when enabled, `ensureDistributedSource` returns an error at line 109-114. However, the default is off, which means out-of-the-box users get auto-registration from project configs. The design should recommend changing the default for project-config-triggered registrations (see 1a above).

### 2c. Source collision detection -- PARTIALLY SUFFICIENT

`checkSourceCollision` correctly checks whether a tool is already installed from a different source. However, the design describes calling it with `dArgs.RecipeName` (the bare name), not the org-scoped key. This means two org-scoped tools with the same bare recipe name would trigger collision detection against each other and against central registry tools. This is correct behavior but the design doesn't make the interaction explicit.

One gap: `checkSourceCollision` uses the `force` flag to skip collision checks. In the project install path, there's no `--force` flag mentioned in the design. Need to clarify whether project installs can ever pass `force=true` and what the implications are.

## 3. "Not Applicable" Justifications

The design states: "No new attack surfaces are introduced."

This is **partially incorrect**. The design does introduce a new attack surface:

- **Config-driven auto-registration**: Previously, auto-registration required explicit user action (typing `tsuku install org/repo`). The project config path allows a checked-in file to trigger registration. This is a qualitatively different trust model -- the `.tsuku.toml` author, not the user, decides which sources to register. This IS a new attack surface even though it reuses existing functions.

- **Resolver expansion**: The dual-key lookup adds a new code path in the resolver that didn't exist before. While it reuses existing config data, the `bareToOrg` reverse map is new logic that could have bugs (e.g., the non-deterministic collision behavior described in 1b).

The design should replace "No new attack surfaces are introduced" with an honest assessment that the trust boundary has shifted from "user types it" to "config file declares it."

## 4. Residual Risk Assessment

### Should be escalated

**Config-driven auto-registration without explicit trust (1a)**: This is the most significant residual risk. The combination of `.tsuku.toml` in a cloned repo + `tsuku install -y` in CI creates a path where arbitrary GitHub repositories can be registered as recipe sources and their binaries installed. While `strict_registries` mitigates this, the off-by-default posture means most users are exposed. This warrants a design decision about whether project-config-triggered registration should require a distinct trust signal.

### Acceptable residual risk

- **Name collision non-determinism (1b)**: Low likelihood, and adding a duplicate-bare-name check during resolver construction is a small implementation fix.
- **Unicode/escape sequence keys (1c)**: Already mitigated by `ownerRepoRegex`.
- **Concurrent config writes (1d)**: Pre-existing condition, not introduced by this design.

## 5. Specific Code-Level Observations

### `splitOrgKey` must validate, not just split

The design describes `splitOrgKey` as a pure splitting function. It should also call `validateRegistrySource` on the source portion, or the pre-scan must do so. Currently the design says the pre-scan calls `parseDistributedName` which does NOT call `validateRegistrySource` -- that validation happens inside `ensureDistributedSource`. If `splitOrgKey` is used by the resolver (which doesn't call `ensureDistributedSource`), the resolver path could process malformed source strings.

The resolver doesn't fetch anything, so the practical risk is low -- a bad key just won't match anything in config. But defense in depth suggests `splitOrgKey` should reject obviously malformed inputs.

### `installYes` propagation needs clarity

The design mentions `installYes` propagates to `ensureDistributedSource` for CI auto-approval. The current `runProjectInstall` code doesn't reference `installYes` for per-tool operations (it only uses it for the batch confirmation prompt). The design should specify exactly where `installYes` is passed in the modified code to ensure the confirmation bypass is intentional and auditable.

## Summary of Recommendations

1. **Add threat model for malicious `.tsuku.toml`** in cloned repos. Consider separating `--yes` (skip confirmation) from source trust. At minimum, document that CI should enable `strict_registries`.

2. **Detect duplicate bare names** in the resolver's `bareToOrg` map construction. Warn or error when two org-scoped keys resolve to the same bare name.

3. **Replace "no new attack surfaces"** with accurate characterization of the shifted trust boundary.

4. **Ensure `splitOrgKey` validates** the source portion, or document why validation is deferred to `ensureDistributedSource`.

5. **Clarify `installYes` propagation** path in the modified `runProjectInstall`.

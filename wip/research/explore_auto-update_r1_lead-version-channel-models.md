# Lead: What version channel/pinning models exist in the wild?

## Findings

### 1. Exact Version Pinning (Strictest)

**Tools**: pip freeze, Go modules, Cargo.lock, package-lock.json

The most restrictive model: a tool is locked to an exact version (e.g., `1.29.3`) and never changes unless the user explicitly updates. This is what tsuku does today. The `VersionState.Requested` field in `internal/install/state.go:17` stores what the user asked for (e.g., `"17"`, `"@lts"`, `""`), and `ActiveVersion` stores the resolved exact version. But there's no mechanism to auto-resolve a newer patch within a requested constraint.

**Trade-offs**: Maximum reproducibility, zero surprise updates. Downside: security patches don't arrive unless the user runs `tsuku update`.

### 2. Prefix/Partial Version Pinning (Fuzzy Match)

**Tools**: nvm, volta, asdf, mise, tsuku (partially)

Users specify a partial version like `20` or `1.29`, and the tool resolves to the latest matching release. nvm's `.nvmrc` accepts `20` to mean "latest 20.x.y". Volta's `package.json` `volta.node` field accepts `20.16.0` (exact) but `volta pin node@20` resolves to latest 20.x.

Tsuku already has this capability in its version providers. `ResolveVersion` on every provider supports fuzzy matching -- e.g., passing `"1.29"` resolves to `"1.29.3"` (see `internal/version/provider.go:15`). The `Requested` field in state already distinguishes what the user typed vs. what resolved. The `.tsuku.toml` project config (`internal/project/config.go`) stores a `Version` string per tool, but doesn't distinguish between exact pins and prefix constraints.

**Key insight**: tsuku already has the resolution infrastructure. The missing piece is a policy layer that says "this tool is pinned at the `1.29` level, so auto-update within `1.29.x` but not to `1.30`."

### 3. Semver Range Constraints (npm/Cargo Style)

**Tools**: npm, Cargo, Composer, Bundler, pub

The most expressive model. Users write semver ranges: `^1.29.0` (any 1.x >= 1.29.0), `~1.29.0` (any 1.29.x >= 1.29.0), `>=1.29, <2`. The resolver finds the best match within the constraint.

npm's `package.json` uses `^` (caret) as default: `^1.2.3` allows `>=1.2.3 <2.0.0`. Cargo uses the same convention. This is the most powerful model but adds significant cognitive load and implementation complexity.

**Trade-offs**: Very flexible, but requires a semver constraint parser and a constraint satisfaction solver. Most developer tool users don't need this power -- it's designed for dependency graphs, not standalone tool installations.

### 4. Named Channels (Stable/Beta/Nightly)

**Tools**: Rust (rustup), Chrome, Electron, Flutter, Claude Code

Rustup offers three channels: `stable`, `beta`, `nightly`. Users subscribe to a channel, and updates arrive automatically within that channel. `rustup default stable` means "always give me the latest stable." Switching channels is a separate operation from updating within a channel.

Claude Code uses a similar model with `stable` and `beta` channels. The channel determines which release track the user follows.

Flutter has `stable`, `beta`, `dev`, and `master` channels, each representing a different stability level.

**Trade-offs**: Very simple mental model for users -- pick a stability level, updates happen within it. Doesn't work well when users need granular version control (e.g., "I want Node 20 stable, not Node 22 stable"). Channels and version pinning are orthogonal concerns.

### 5. Hybrid: Channel + Version Pin

**Tools**: proto, mise

Proto (by moonrepo) combines channels with version pinning. Users can specify:
- `latest` -- always latest stable
- `20` -- latest within major 20
- `20.16` -- latest within minor 20.16
- `20.16.0` -- exact pin
- `canary` -- latest unstable build

Mise (formerly rtx) similarly supports partial versions in `.mise.toml`:
```toml
[tools]
node = "20"       # latest 20.x
python = "3.12"   # latest 3.12.x
go = "latest"     # always latest
```

This is the model closest to what tsuku needs. It maps naturally to semver granularity levels without introducing range syntax.

### 6. The Homebrew Model (No Pinning by Default)

**Tools**: Homebrew, apt (without holds)

Homebrew doesn't pin versions by default. `brew upgrade` upgrades everything to latest. Users can `brew pin <formula>` to prevent upgrades. There's no partial pinning -- a formula is either pinned to its current version or eligible for any upgrade.

Homebrew Cask has no built-in version pinning at all. Applications update to whatever the latest cask version specifies.

**Trade-offs**: Simplest possible model. Works when users trust upstream releases. Falls apart when users need to stay on a specific major version (e.g., Node 20 LTS vs Node 22).

### 7. Tsuku's Current State

Reviewing the codebase, tsuku has several relevant pieces already:

- **`VersionState.Requested`** (`internal/install/state.go:17`): Stores what the user typed at install time. Values like `"17"`, `"@lts"`, `""`. This is the seed for a pinning model -- it captures user intent.

- **`ToolState.ActiveVersion`** (`internal/install/state.go:79`): The resolved version that's symlinked. This is the "actual" version.

- **`.tsuku.toml` project config** (`internal/project/config.go`): Per-project tool declarations with a `Version` string field. Currently treated as an exact version or empty.

- **`ResolveVersion` with fuzzy matching** (`internal/version/provider.go:14-16`): All version providers support prefix matching. "1.29" resolves to "1.29.3".

- **`tsuku update <tool>`** (`cmd/tsuku/update.go`): Manual update that resolves latest and reinstalls. No awareness of version constraints -- it always goes to absolute latest.

- **`tsuku outdated`** (`cmd/tsuku/outdated.go`): Checks for newer versions but only compares against absolute latest, not against any pinning constraint.

The infrastructure for fuzzy resolution exists, but there's no policy layer connecting `Requested` to update behavior. The `update` command ignores `Requested` entirely.

## Implications

### The natural fit: Prefix-level pinning (Model 5)

Tsuku's philosophy ("self-contained package manager for developer tools") aligns best with the proto/mise hybrid model. Here's why:

1. **It matches existing infrastructure.** `Requested` already captures user intent at install time. Version providers already support prefix matching. The gap is purely in update policy.

2. **It avoids range syntax complexity.** Semver ranges (npm/Cargo model) solve dependency graphs. Tsuku installs standalone tools -- users don't need `>=1.29, <2` when `1.29` communicates the same intent more simply.

3. **It's composable with channels.** Named channels (`stable`, `lts`) can coexist as special values in the same `Requested` field. `@lts` for Node is already partially supported in the state format.

4. **The granularity maps to natural user intent:**
   - `""` or `"latest"` -- always latest (Homebrew model)
   - `"20"` -- latest within major (useful for Node, Java)
   - `"1.29"` -- latest within minor (useful for kubectl, terraform)
   - `"1.29.3"` -- exact pin, no auto-update (pip freeze model)

### Recommended pinning semantics

The `Requested` field should drive update behavior:

| Requested | Meaning | Auto-updates to |
|-----------|---------|-----------------|
| `""` / `"latest"` | Track latest stable | Any newer stable version |
| `"20"` | Pin to major 20 | 20.x.y where x or y is newer |
| `"1.29"` | Pin to minor 1.29 | 1.29.z where z is newer |
| `"1.29.3"` | Exact pin | Nothing (manual update only) |
| `"@lts"` | Track LTS channel | Next LTS release (tool-specific) |

The number of dot-separated components in `Requested` determines the pinning level. This is a zero-new-syntax approach -- users already type these values naturally.

### What needs to change

1. **`tsuku update` should respect `Requested`.** Instead of resolving absolute latest, it should resolve latest-within-constraint. A tool pinned to `"1.29"` should update to `1.29.4` but not `1.30.0`.

2. **`tsuku outdated` should show two columns.** "Available within pin" and "Available overall" so users can see both in-constraint updates and whether they're falling behind their pin.

3. **`.tsuku.toml` version strings should follow the same semantics.** `node = "20"` means "latest 20.x", not "exactly version 20".

4. **A new `tsuku pin` command (or `--pin` flag on install)** could let users change the pinning level after installation without reinstalling.

## Surprises

1. **Tsuku already has 80% of the infrastructure.** The `Requested` field, fuzzy version resolution, and project config format are all in place. The missing piece is a thin policy layer -- roughly a function that takes `(Requested, CandidateVersion) -> bool` to determine if an update is within constraint.

2. **No existing tool cleanly separates "channel" from "pin level."** Rustup has channels but no version pinning. nvm has version pinning but no channels. Proto comes closest but doesn't have a formal channel concept either. Tsuku could be distinctive by treating channels (`@lts`, `@stable`) and prefix pins (`20`, `1.29`) as values in the same dimension.

3. **The `outdated` command currently only checks GitHub repos.** It iterates through tools but skips any without a `github_archive` or `github_file` action (see `cmd/tsuku/outdated.go:76-88`). This means npm, PyPI, crates.io, and other provider-sourced tools are invisible to outdated checks. This is a pre-existing gap that auto-update would need to fix by using the provider factory instead.

## Open Questions

1. **Should exact-version pins (`"1.29.3"`) prevent `tsuku update` from doing anything, or should update still offer to move to latest?** A strict reading says "never update." A pragmatic reading says "warn and offer."

2. **How should pre-release channels interact with pinning?** If a user installs `node@22-rc`, should that track the RC channel for Node 22 only? The `Requested` field would need to encode both the version prefix and the pre-release track.

3. **Should `.tsuku.toml` support an explicit `pin` field separate from `version`?** E.g., `node = { version = "20.16.0", pin = "20" }` to separate "install this specific version" from "auto-update within this range." The current `ToolRequirement` struct (`internal/project/config.go:36`) only has a `Version` field.

4. **What happens when a tool's version numbering doesn't follow semver?** Some tools use calver (e.g., `2024.01.15`), date-based versions, or single-number versions. The prefix-pinning model works for calver (`"2024"` pins to year 2024) but might surprise users.

5. **Does the `@lts` channel need tool-specific resolution logic?** Node.js has a well-defined LTS schedule. Most other tools don't have an LTS concept. Should tsuku invent LTS semantics per-tool, or only support it where upstream defines it?

## Summary

The dominant version pinning models in the wild fall into five categories: exact pins, prefix/partial matching, semver ranges, named channels, and hybrid approaches -- with proto and mise's prefix-level hybrid being the closest fit for tsuku's use case. Tsuku already has the core infrastructure (the `Requested` field in state, fuzzy resolution in version providers, and project config format), so the primary gap is a policy layer that connects user-specified pin levels to update eligibility decisions. The biggest open question is whether to keep the pin level purely implicit (derived from the number of version components the user typed) or add an explicit pin/constraint field to both state and `.tsuku.toml`.

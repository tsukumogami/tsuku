# DX Engineer Analysis: Contributor Isolation vs Per-Directory Activation

## The Constraint

"I wouldn't like to add a flag or feature in the tsuku binary if it wasn't moving toward per-directory activation."

This constraint is sound. Every CLI flag is permanent API surface that must be supported indefinitely. The question is: can we solve the contributor problem without adding that surface area?

## Immediate Problem: Contributor Isolation

Contributors testing recipe changes need to avoid polluting their production `$TSUKU_HOME/bin` and `state.json`. The solution is trivial and requires zero binary changes:

**Create `scripts/dev-env` wrapper:**

```bash
#!/usr/bin/env bash
# Isolated development environment for recipe testing
export TSUKU_HOME="${TSUKU_DEV_HOME:-$(pwd)/.tsuku-dev}"
mkdir -p "$TSUKU_HOME"

# Share download cache (content-addressed, safe to share)
if [ -d "$HOME/.tsuku/cache/downloads" ]; then
  mkdir -p "$TSUKU_HOME/cache"
  ln -sf "$HOME/.tsuku/cache/downloads" "$TSUKU_HOME/cache/downloads"
fi

exec tsuku "$@"
```

**Usage:** `./scripts/dev-env install serve`

This gives contributors complete isolation instantly. No waiting for design, no binary changes, no new flags. Document it in CONTRIBUTING.md and ship it tomorrow.

## Per-Directory Activation: UX Analysis

The best UX for per-directory activation is **shims with optional shell hooks**. Here's why:

### Shim-First Approach

```
$TSUKU_HOME/
├── bin/           # Shim directory (add to PATH once)
│   ├── go         # Thin wrapper script
│   ├── node       # Reads .tsuku.toml in $PWD
│   └── python     # Execs correct version
├── tools/
│   ├── go-1.21.5/
│   ├── go-1.22.0/
│   └── node-20.10.0/
└── cache/         # Shared downloads
```

**Activation flow:**
1. User runs `go version`
2. Shim in `$TSUKU_HOME/bin/go` executes
3. Walks up from `$PWD` to find `.tsuku.toml`
4. Reads: `[tools]\ngo = "1.21.5"`
5. Execs `$TSUKU_HOME/tools/go-1.21.5/bin/go version`

**UX properties:**
- Zero shell integration required (works in scripts, cron, CI)
- Automatic version switching on `cd`
- Small overhead (~5ms for directory walk + TOML parse)
- Clean error messages when version not installed

### Config File: `.tsuku.toml`

Use TOML from day one. Plain text is a dead end once you need env vars or metadata.

```toml
[tools]
go = "1.21.5"
node = "20"          # Prefix match: latest 20.x installed
serve = "latest"     # Latest installed version

[env]
GOPATH = "${PWD}/go-workspace"
NODE_ENV = "development"
```

Commit this to repos for team-wide reproducibility.

## Bridging Both Problems: One Mechanism

Here's the elegant part: **`.tsuku.toml` solves both problems.**

### For contributors testing recipes:

Create `tsuku-1/.tsuku.local.toml` (gitignored):

```toml
# Override TSUKU_HOME for isolated testing
[config]
home = "./.tsuku-dev"

[tools]
serve = "dev"  # Version you're testing
```

Now `tsuku install serve` in this directory uses the isolated home automatically. No wrapper script needed.

### For production users:

Projects commit `.tsuku.toml` with required versions. Teams get consistent tool versions. Zero manual coordination.

## Migration Path

**Phase 1 (now):** Ship `scripts/dev-env` wrapper. Solves contributor problem today.

**Phase 2 (parallel):** Design `.tsuku.toml` spec. Focus questions:
- Version resolution rules (prefix matching? semver ranges?)
- Env var interpolation syntax
- Fallback when no config exists
- Auto-install behavior (prompt? fail? silent?)

**Phase 3:** Implement shim layer. Replace `$TSUKU_HOME/bin/` symlinks with generated shims.

**Phase 4:** Add `[config]` section to `.tsuku.toml`. Contributors can now use `.tsuku.local.toml` instead of wrapper scripts.

**Phase 5 (optional):** Shell hook for performance. Pre-compute version map on `cd`, skip shim overhead.

## Why Not `--env`?

The `--env` flag solves explicit isolation (CI jobs, parallel tests). That's useful, but orthogonal to per-directory activation.

**`--env` problems:**
- Explicit activation (`tsuku --env dev install`) vs implicit (`cd` triggers switch)
- Session-scoped (env var) vs directory-scoped (config file)
- Separate state files vs unified state + context-aware shims

If you build `--env` now, you'll either abandon it when implementing per-directory activation (because mise's model is config-based, not state-isolated), or you'll compromise the per-directory design to remain compatible with `--env` semantics. Neither outcome is good.

## Recommendation

1. **Immediate:** Merge `scripts/dev-env` wrapper. Document in CONTRIBUTING.md. Problem solved.
2. **Next sprint:** Design `.tsuku.toml` spec via `/explore`. Focus on version resolution UX and config precedence.
3. **Implementation:** Shim layer first (works everywhere), shell hook later (optimization).
4. **Validation:** Beta test with contributors using `.tsuku.local.toml` for isolation.

This path avoids premature CLI flags, solves the contributor problem immediately, and builds toward the right per-directory UX.

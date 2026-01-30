# Systems Engineering Analysis: Shared-Install Model vs. --env Proposal

## Executive Summary

The `--env` proposal creates architectural debt that conflicts with future per-directory activation. The "current" symlink model (`$TSUKU_HOME/tools/current/<name>`) is fundamentally incompatible with context-dependent activation. A refactor now avoids rewriting the isolation feature later.

## Core Architectural Conflict

**Current model**: One active version per tool, tracked via symlink (`tools/current/gh → gh-2.60.1`). State.json records which version is "current".

**--env proposal**: Isolates state by duplicating the entire structure under `envs/<name>/`, including separate `state.json` and `tools/current/` directories. Each environment has its own "current" pointer.

**Future requirement**: Per-directory activation means multiple versions coexist and "current" becomes context-dependent (cwd-based lookup). The symlink in `tools/current/` no longer represents truth.

**The problem**: If we implement `--env` as proposed, we're building isolation on top of a state model that will be replaced. When we add per-directory activation, we'll need to:
1. Rewrite how environments track active versions (no more `current/` symlinks)
2. Change state.json schema to support multiple active versions
3. Update all code that assumes one active version per tool

## What Mise Does Differently

Mise installs all versions to a shared directory (`~/.local/share/mise/installs/<tool>/<version>/`). Config files (`.mise.toml`, `.tool-versions`) select which versions are active in a given directory tree. The install directory has no "current" concept.

**Key insight**: Mise decouples installation location from activation state. Installation is global and version-agnostic. Activation is local and config-driven.

## Minimal Refactor Path

### 1. Decouple "installed" from "active" (now)

**Change**: Remove `tools/current/` symlinks. State.json tracks installed versions, not "current" version. Path resolution happens at runtime via config lookup.

**Impact on config.go**:
```go
// Remove CurrentDir from Config struct
// Remove CurrentSymlink() method
// Add ActivationLookup(toolName string, cwd string) (version string, err)
```

This change is backward-compatible with existing installations (symlinks become vestigial, ignored).

### 2. Implement --env on top of decoupled model

**Change**: Environments get separate `state.json` (installed versions) but share `tools/` directory. No duplicate installations.

**Structure**:
```
~/.tsuku/
├── tools/              # Global install directory
│   ├── gh-2.60.1/
│   └── gh-2.62.0/
├── envs/
│   ├── dev/
│   │   └── state.json   # Records "gh: 2.60.1 installed"
│   └── ci/
│       └── state.json   # Records "gh: 2.62.0 installed"
```

**Isolation**: Environments have separate state files (what's installed, what's active). They share the tools directory (actual binaries).

**DefaultConfig() changes**:
- `ToolsDir` remains `$TSUKU_HOME/tools` (not under envs/)
- `StateFile` moves to env-specific path when `TSUKU_ENV` is set
- Remove `CurrentDir` entirely

### 3. Future per-directory activation builds on this

**When adding mise-style activation**:
- Add config file discovery (`.tsuku.toml`, walk up from cwd)
- Implement config-based version resolution
- Environments continue to work: they just define a different "activation context" (via env var instead of cwd)

**No rework needed**: State.json already tracks installed versions. Activation is already decoupled from installation path.

## Compatibility Analysis

**Is --env (as proposed) compatible with shared installs?**

No. The proposal duplicates `tools/` per environment, which conflicts with the shared-install model. If we keep separate `envs/dev/tools/` directories, we can't later merge them without data migration.

**Can we fix this during implementation?**

Yes, but it requires changing the core state model now (remove `current/` symlinks) rather than layering isolation on top of the existing model.

## Recommended Changes to DESIGN-dev-environment-isolation.md

1. **Directory layout**: Remove `tools/` from under `envs/<name>/`. Keep one global `$TSUKU_HOME/tools/`.

2. **State isolation**: Environments only get separate `state.json`, `registry/`, `cache/versions/`. They share `tools/` and `cache/downloads/`.

3. **Config.go refactor**: Remove `CurrentDir` and `CurrentSymlink()`. Add activation logic that respects `TSUKU_ENV`.

4. **Migration path**: Document how this enables future per-directory activation without rework.

## Concrete Code Impact

**Files requiring changes**:
- `internal/config/config.go`: Remove CurrentDir, change ToolsDir to always be global
- `internal/state/state.go`: Remove "current version" tracking, track only installed versions
- `cmd/tsuku/install.go`: Don't create/update current symlinks
- `cmd/tsuku/list.go`: Query state.json instead of scanning `tools/current/`

**Lines of code**: ~200 LOC across 4 files. Roughly the same effort as implementing --env on the current model, but with no future rework.

## Conclusion

The `--env` proposal solves contributor isolation but bakes in the single-active-version assumption. Refactoring to a shared-install model now costs the same but avoids rewriting the feature when per-directory activation arrives. The minimal change is removing `tools/current/` and making activation config-driven from day one.

# CLI Architecture Analysis: Environment Isolation & Per-Directory Versioning

## Core Insight

The contributor isolation problem and per-directory versioning are **the same problem** viewed at different scales. Both need: version selection, isolated execution, and state management. Build one mechanism that serves both.

## Proposed Architecture: Config-Based Activation (Mise Model)

**Reject** isolated state directories (`$TSUKU_HOME/envs/<name>/`). They're a dead-end for per-directory versioning.

**Adopt** mise's model: shared install directory, config-based activation.

### Structure

```
$TSUKU_HOME/
├── tools/           # All versions, all contexts (shared)
│   ├── gh-2.40.0/
│   ├── gh-2.41.0/
│   └── serve-14.2.0/
├── state/
│   └── default.json      # Default context state
│   └── dev.json          # Dev context state (optional)
└── bin/             # Symlinks (points to default context versions)
```

### Two-Level Config System

1. **Context config** (`tsuku.toml` or env var `TSUKU_CONTEXT=dev`)
   ```toml
   [context]
   name = "dev"

   [tools]
   gh = "2.40.0"
   serve = "14.2.0"
   ```

2. **State tracking** (`state/dev.json`)
   ```json
   {
     "installed": {
       "gh": "2.40.0",
       "serve": "14.2.0"
     }
   }
   ```

### Execution Flow

```bash
# Contributor workflow
export TSUKU_CONTEXT=dev
tsuku install gh@2.40.0    # Writes to state/dev.json, symlinks to tools/gh-2.40.0
tsuku exec gh --version    # Reads state/dev.json, executes tools/gh-2.40.0/bin/gh

# Per-directory workflow (future)
cd ~/project
cat > tsuku.toml <<EOF
[tools]
node = "18.0.0"
EOF
tsuku exec node            # Reads tsuku.toml, executes tools/node-18.0.0/bin/node
```

## Minimal In-Binary Features (Phase 1)

1. **Context-aware state** (`--context` flag or `TSUKU_CONTEXT` env var)
   - `tsuku install --context=dev gh` → writes to `state/dev.json`
   - Default context: `default` (backward compatible)

2. **`tsuku exec`** command
   - Reads active context (env var or flag)
   - Executes tool from `$TSUKU_HOME/tools/<name>-<version>/bin/<binary>`
   - **Critical**: This is the foundation for shims later

3. **State file per context** (`state/<context>.json`)

## External Tooling (Phase 1)

- Shell alias helper: `alias tdev='TSUKU_CONTEXT=dev tsuku'`
- CI wrapper script (kept external, uses `TSUKU_CONTEXT=ci`)

## Migration Path to Per-Directory Versioning (Phase 2)

Phase 1 gives us:
- Shared install directory ✓
- Context-based execution ✓
- `tsuku exec` command ✓

Phase 2 adds:
- Config file discovery (walk up directory tree for `tsuku.toml`)
- Shim generation (symlinks that wrap `tsuku exec <tool>`)
- Auto-install missing versions

**Key**: Phase 1's contributor isolation IS a manual version of Phase 2's auto-activation.

## Why This Works

- Contributor problem: Set `TSUKU_CONTEXT=dev`, get isolated state
- Per-directory versioning: Same execution path, config source changes
- No wasted work: Every Phase 1 feature is used in Phase 2
- Complexity budget: Phase 1 is ~300 lines (state file + exec command)

## What NOT to Build

- Separate state directories (wrong abstraction)
- Shell hooks in Phase 1 (defer to Phase 2)
- Complex config merging (start with single-file)

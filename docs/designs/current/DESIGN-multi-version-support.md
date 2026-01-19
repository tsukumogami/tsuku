---
status: Current
problem: Tsuku currently supports only one installed version per tool, forcing developers to reinstall when switching between projects that require different versions.
decision: Implement multi-version support with automatic state.json migration, new activate command, and modified install/remove/list behaviors to keep existing versions while managing an active version via symlinks.
rationale: Developers working with language runtimes frequently need multiple versions for different projects. This approach minimizes breaking changes through automatic migration while maintaining security with atomic operations and file locking.
---

# Design Document: Multi-Version Tool Support

**Status**: Current

<a id="implementation-issues"></a>
**Implementation Issues**:

| Issue | Title | Design Section | Dependencies |
|-------|-------|----------------|--------------|
| [#294](https://github.com/tsukumogami/tsuku/issues/294) | Add multi-version schema and migration | [State.json Schema Change](#statejson-schema-change) | None |
| [#295](https://github.com/tsukumogami/tsuku/issues/295) | Add file locking for concurrent access | [Concurrency Protection](#concurrency-protection) | None |
| [#296](https://github.com/tsukumogami/tsuku/issues/296) | Add atomic symlink updates | [Concurrency Protection](#concurrency-protection) | None |
| [#297](https://github.com/tsukumogami/tsuku/issues/297) | Add activate command for version switching | [Command Behavior Changes](#command-behavior-changes) | #294, #296 |
| [#298](https://github.com/tsukumogami/tsuku/issues/298) | Preserve existing versions on install | [Command Behavior Changes](#command-behavior-changes) | #294 |
| [#299](https://github.com/tsukumogami/tsuku/issues/299) | Support version-specific removal | [Command Behavior Changes](#command-behavior-changes) | #294, #296 |
| [#300](https://github.com/tsukumogami/tsuku/issues/300) | Show all installed versions in list | [Command Behavior Changes](#command-behavior-changes) | #294 |

```
Dependency Graph:

#294 (State schema) ─────┬──> #297 (Activate)
                         ├──> #298 (Install)
#295 (File locking)      ├──> #299 (Remove)
                         └──> #300 (List)
#296 (Atomic symlinks) ──┴──> #297, #299
```

## Context and Problem Statement

Tsuku currently supports only one installed version per tool. Installing a new version removes the previous one, forcing developers to reinstall when switching between projects that require different versions.

Developers working with language runtimes (Java, Node.js, Go) frequently need multiple versions for different projects. The current model breaks workflow and wastes time on repeated downloads.

## Scope

**In scope:**
- State.json schema migration to support multiple versions per tool
- New `tsuku activate <tool> <version>` command
- Modified install behavior (keep existing versions)
- Modified remove behavior (support `tool@version` syntax)
- Modified list output (show all versions with active indicator)
- File locking for concurrent access
- Atomic symlink updates

**Out of scope (future work):**
- Per-directory version selection (`.tsuku-versions` files)
- Automatic version switching on directory change
- Version constraint satisfaction across dependencies
- Disk space management / automatic cleanup

## Decision Drivers

- **Minimize breaking changes**: Existing state.json should migrate automatically
- **Leverage existing patterns**: Libraries already support multi-version
- **Security first**: Atomic operations, input validation, file locking

## Assumptions

1. Single active version is sufficient for v1
2. Dependencies use whatever version is currently active
3. Migration is one-way (no downgrade support)
4. Users manage disk space by removing old versions manually

## Solution Design

### State.json Schema Change

**Current schema:**
```json
{
  "installed": {
    "liberica-jdk": {
      "version": "21.0.1",
      "is_explicit": true,
      "binaries": ["java", "javac"]
    }
  }
}
```

**New schema:**
```json
{
  "installed": {
    "liberica-jdk": {
      "active_version": "21.0.5",
      "is_explicit": true,
      "versions": {
        "17.0.12": {
          "requested": "17",
          "binaries": ["java", "javac"],
          "installed_at": "2025-11-01T10:00:00Z"
        },
        "21.0.5": {
          "requested": "@lts",
          "binaries": ["java", "javac"],
          "installed_at": "2025-12-01T14:30:00Z"
        }
      }
    }
  }
}
```

Key changes:
- `version` field replaced by `active_version` (currently symlinked)
- New `versions` map containing all installed versions
- Per-version metadata: `requested` (what user asked for), `binaries`, `installed_at`

### Migration Strategy

On state.json load, detect old format (has `version` but no `versions`) and migrate automatically. The single installed version becomes both the active version and the only entry in the versions map.

### Command Behavior Changes

**`tsuku activate <tool> <version>`** (new command)
- Switches the active version by updating symlinks
- Fails if the specified version is not installed
- Lists available versions on error

**`tsuku install <tool>[@version]`**
- No longer removes existing versions
- Adds new version alongside existing ones
- Makes newly installed version active
- Skips if exact version already installed

**`tsuku remove <tool>[@version]`**
- Without version: removes ALL installed versions
- With version (`tool@1.0`): removes only that version
- If active version is removed, switches to most recently installed remaining version
- If last version is removed, removes the entire tool entry from state

**`tsuku list`**
- Shows one line per installed version (not per tool)
- Marks active version with `(active)` indicator

Example output:
```
liberica-jdk  17.0.12
liberica-jdk  21.0.5  (active)
nodejs        18.20.0
nodejs        20.10.0 (active)
```

### Concurrency Protection

- **File locking**: Use flock(2) on state.json.lock during read-modify-write operations
- **Atomic writes**: Write to temporary file, then rename
- **Atomic symlinks**: Create temporary symlink, then rename over existing

## Security Considerations

### TOCTOU Race Conditions (MEDIUM-HIGH)

**Risk:** During version switching, a malicious process could swap symlink targets.

**Mitigation:** Atomic symlink updates (create tmp, rename), verify targets after creation, hold exclusive lock during activate.

### State.json Poisoning (MEDIUM)

**Risk:** Attacker modifies state.json to inject malicious version entries.

**Mitigation:** Validate version strings (reject `..`, `/`, `\`), verify symlink targets are within `$TSUKU_HOME/tools/`.

### Concurrent Write Corruption (HIGH)

**Risk:** Multiple tsuku processes modifying state.json simultaneously.

**Mitigation:** File locking with flock(2), atomic write pattern.

## Breaking Changes

| Before | After |
|--------|-------|
| `tsuku install tool@v2` removes v1 | Keeps v1, makes v2 active |
| `tsuku remove tool` removes one version | Removes ALL versions |
| `tsuku list` shows one line per tool | Shows one line per version |
| state.json has `"version": "X"` | Has `"active_version"` and `"versions"` |

## References

- `internal/install/state.go` - Current state management
- `internal/install/manager.go` - Current installation manager
- `internal/install/library.go` - Library multi-version model (reference)


---
summary:
  constraints:
    - Use Go's debug/elf and debug/macho packages only - no external tools
    - RPATH count limit: maximum 100 entries per binary
    - Path length limit: maximum 4096 characters per path
    - Expanded paths must resolve to $TSUKU_HOME/tools/ (canonical validation)
    - Unexpanded variables ($, @) after expansion must error
  integration_points:
    - internal/verify/rpath.go (new file for ExtractRpaths and ExpandPathVariables)
    - internal/verify/types.go (may need new error categories)
    - Consumed by #989 (recursive validation) for dependency path resolution
  risks:
    - Security-sensitive: path traversal, symlink attacks, DoS via large RPATH lists
    - Cross-platform differences between ELF ($ORIGIN) and Mach-O (@rpath, @loader_path)
    - Symlink resolution edge cases
    - Error messages must not leak internal path structures
  approach_notes: |
    Create two main functions:
    1. ExtractRpaths(path) - uses debug/elf.DynString(DT_RUNPATH) with DT_RPATH fallback,
       and debug/macho.File.Loads for LC_RPATH
    2. ExpandPathVariables(dep, binaryPath, rpaths) - expands $ORIGIN, @rpath, @loader_path,
       @executable_path with filepath.Clean() and filepath.EvalSymlinks()

    Security checklist required for tier:critical issues.
---

# Implementation Context: Issue #982

**Source**: docs/designs/DESIGN-library-verify-deps.md (Step 5, Security Mitigations)

## Key Design Points

- RPATH via Go stdlib: `debug/elf.DynString(DT_RUNPATH)`, `debug/macho.Rpath`
- Symlink handling: `filepath.EvalSymlinks()` consistent with PR #963
- Path normalization: `filepath.Clean()` on all paths

## Security Mitigations (from design)

| Risk | Mitigation |
|------|------------|
| Path traversal via symlinks | `filepath.EvalSymlinks()` before validation |
| Path normalization tricks | `filepath.Clean()` on all paths |
| Parser vulnerabilities | Go stdlib with panic recovery |
| Dependency count exhaustion | Limit 1000 deps per binary |

## This Issue's Specific Security Requirements

- RPATH limit: 100 entries per binary
- Path length limit: 4096 characters
- Canonical path validation: must resolve to $TSUKU_HOME/tools/
- Reject unexpanded variables after expansion
- Error messages must not leak internal paths

# Issue 220 Implementation Plan

## Summary

Implement the `link_dependencies` action that creates symlinks from a tool's `lib/` directory to the shared library location (`libs/{name}-{version}/lib/`).

## Context

From the design doc: Tools need local symlinks so RPATH resolution finds libraries in a predictable location relative to the binary. The action creates symlinks like:
```
ruby-3.4.0/lib/libyaml.so.2 -> ../../../libs/libyaml-0.2.5/lib/libyaml.so.2
```

## Approach

Create a new action following existing action patterns. The action will:
1. Accept library name and version parameters
2. Find all library files in the source libs location
3. Create symlinks in the tool's lib directory pointing to the shared location
4. Detect collisions: error if destination exists and is not our symlink

### Parameters
- `library` (required): Library name (e.g., "libyaml")
- `version` (required): Library version (e.g., "0.2.5")

The action uses:
- `ctx.ToolInstallDir` for the tool's installation directory (creates lib/ subdirectory)
- `ctx.ToolsDir` parent to find `../libs/` relative path

### Alternatives Considered
- **Copy files instead of symlink**: Rejected - wastes disk space, symlinks enable shared library deduplication
- **Absolute symlinks**: Rejected - relative symlinks are portable if `$TSUKU_HOME` moves

## Files to Create
- `internal/actions/link_dependencies.go` - Action implementation
- `internal/actions/link_dependencies_test.go` - Unit tests

## Files to Modify
- `internal/actions/action.go` - Register the new action
- `internal/actions/dependencies.go` - Add action to ActionDependencies map

## Implementation Steps
- [ ] Create LinkDependenciesAction struct with Name() method
- [ ] Implement Execute() that:
  - Parses library and version parameters
  - Constructs source path (libs/{name}-{version}/lib/)
  - Constructs relative symlink target from tool/lib/ to libs/
  - Enumerates library files in source
  - Creates symlinks with collision detection
- [ ] Register action in action.go
- [ ] Add to dependencies.go
- [ ] Add unit tests for:
  - Basic symlink creation
  - Collision detection (error when file exists)
  - Safe overwrite (when existing symlink points to correct target)
  - Missing library directory error

## Key Implementation Details

### Relative Path Calculation
From `tools/ruby-3.4.0/lib/` to `libs/libyaml-0.2.5/lib/`:
```
../../../libs/libyaml-0.2.5/lib/libyaml.so.2
```

### Collision Detection Logic
```go
if info, err := os.Lstat(destPath); err == nil {
    if info.Mode()&os.ModeSymlink != 0 {
        // Check if symlink points to our target
        existingTarget, _ := os.Readlink(destPath)
        if existingTarget == expectedTarget {
            // Already linked correctly, skip
            continue
        }
    }
    // File exists but is not our symlink - collision
    return fmt.Errorf("collision: %s already exists", destPath)
}
```

## Testing Strategy
- Unit tests:
  - Create symlinks to library files
  - Preserve symlink chains (libyaml.so.2 -> libyaml.so.2.0.9)
  - Error on collision (non-symlink file exists)
  - Skip when correct symlink already exists
  - Error when source library doesn't exist

## Risks and Mitigations
- **Path traversal**: Validate library name doesn't contain `..` or `/`
- **Symlink escape**: Ensure symlinks only point within libs directory

## Success Criteria
- [ ] Action creates symlinks from tool/lib/ to libs/{name}-{version}/lib/
- [ ] Collision detection: error if destination file exists and is not our symlink
- [ ] Unit tests including collision detection
- [ ] All existing tests continue to pass

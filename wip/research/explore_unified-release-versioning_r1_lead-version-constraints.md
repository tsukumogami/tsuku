# Lead: Does tsuku have a mechanism to enforce version constraints between itself and companion binaries?

## Findings

### No existing version constraint mechanism
tsuku has no built-in way to enforce that dltest and llm must be installed at the exact same version as tsuku itself. There is no `version_constraint`, `requires_version`, or `version_lock` field in the recipe format.

### Recipe dependencies are version-agnostic
The `MetadataSection` in `internal/recipe/types.go` (lines 159-162) defines `Dependencies`, `RuntimeDependencies`, `ExtraDependencies`, and `ExtraRuntimeDependencies` fields, but these are simple string arrays with no version specifications. Dependencies are declared as bare names like `["curl"]`, not as `["curl@1.0.0"]`.

### Install state tracks versions but not dependency versions
The `VersionState` and `ToolState` structures in `internal/install/state.go` track installed versions and dependencies via `RequiredBy` field, but do not store which version of a dependency was required. When dltest installs curl v7.0, there's no record that dltest specifically needs curl v7.0 vs v8.0.

### Version resolution is independent per tool
In `cmd/tsuku/install_deps.go` (line 77), version resolution runs independently for each tool. When installing tsuku v0.5.0 and its dependency dltest, each tool resolves its own version separately from GitHub releases or other sources.

### Telemetry captures constraints (but not programmatic ones)
The `VersionConstraint` field exists in telemetry events (`internal/telemetry/event.go` line 14), but this is only for user-facing version requests like `@latest` or `@lts`, not for programmatic constraints between tools.

## Implications

To implement same-version locking, a new mechanism is needed. Options include:
- A new recipe field like `version_lock = "self"` that pins to tsuku's own version at install time
- Compile-time embedding of the expected companion versions into the tsuku binary
- A runtime version check when tsuku invokes dltest/llm, rejecting mismatches

The compile-time approach is simplest since it avoids recipe schema changes and guarantees lockstep without relying on install-time resolution.

## Surprises

The dependency system is entirely version-agnostic -- not just for companion binaries, but for ALL dependencies. Adding version constraints for just the companion tools would be a special case, not a general feature.

## Open Questions

- Should version constraints be a general recipe feature, or a special case for "self" dependencies?
- Is compile-time version embedding (hardcoding expected dltest/llm versions in tsuku) simpler than a recipe-level constraint?
- What should happen at runtime when a version mismatch is detected -- error, warning, or auto-update?

## Summary
tsuku has no mechanism for version-locked dependencies -- all dependency declarations are version-agnostic string arrays. Implementing same-version enforcement for companion binaries would require either a new recipe schema field or compile-time version embedding in the tsuku binary. The compile-time approach may be simplest since it avoids schema changes and guarantees lockstep.

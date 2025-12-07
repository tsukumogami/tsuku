# Issue 236 Summary

## What Was Implemented

Override and extension mechanisms for dependency resolution, allowing recipes to replace or extend implicit dependencies at both step and recipe levels.

## Changes Made

- `internal/recipe/types.go`: Added ExtraDependencies and ExtraRuntimeDependencies fields to MetadataSection
- `internal/actions/resolver.go`: Extended ResolveDependencies to handle:
  - Step-level dependencies/runtime_dependencies (replace)
  - Recipe-level Dependencies/RuntimeDependencies (replace all)
  - Recipe-level ExtraDependencies/ExtraRuntimeDependencies (extend)
- `internal/actions/resolver_test.go`: Added 7 new tests for replace/extend behavior

## Key Decisions

- **Replace vs extend semantics**: Empty slice `[]` for replace means "no deps"; extend adds without clearing
- **Precedence order**: Step → Recipe replace → Recipe extend (as per design doc)
- **TOML field naming**: Used snake_case for TOML tags, CamelCase for Go fields

## Trade-offs Accepted

- Step-level replace only affects that step's contribution to deps; other steps still contribute their implicit deps

## Test Coverage

- New tests added: 7 tests
- All 17 packages pass

## Known Limitations

- No transitive resolution yet (handled in #237)

## Future Improvements

- Issue #237: Transitive resolution with cycle detection

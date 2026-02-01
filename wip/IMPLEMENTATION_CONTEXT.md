---
summary:
  constraints:
    - Validation happens at parse time in ParseRegistry, not at lookup time
    - Invalid entries should fail the entire load (not silently skip)
    - Error messages must include tool name and which field is invalid
    - Registry load failures are warnings, not fatal — resolver continues to next stage
  integration_points:
    - internal/discover/registry.go — ParseRegistry needs validation after unmarshal
    - internal/discover/registry_lookup.go — NewRegistryLookup handles nil registry gracefully
    - internal/discover/chain.go — chain logs warning and continues on registry load failure
  risks:
    - The issue mentions a validate-registry CLI command that doesn't exist yet
    - The issue references unresolved ISSUE placeholders in dependencies
  approach_notes: |
    Add a validateEntries method to ParseRegistry that iterates tools and
    rejects empty builder/source. The chain already handles nil registry
    (returns nil,nil on miss). For load failures, ensure the chain logs
    a warning. Skip the validate-registry CLI command — it's not in the AC.
---

# Implementation Context: Issue #1313

**Source**: docs/designs/DESIGN-discovery-resolver.md

## Key Facts

- `ParseRegistry` in `internal/discover/registry.go` currently only checks schema_version
- `RegistryEntry` has Builder, Source, Binary (optional) fields
- The validation script in the issue uses a `validate-registry` CLI command that doesn't exist
- Core AC: validate entries at parse time, reject empty builder/source, clear error messages

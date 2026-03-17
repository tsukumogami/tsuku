# Validation Results: Issue 9 - List Shows Source for Distributed Tools

## Scenario 26: List shows source for distributed tools

**Status**: PASSED

### Tests Executed

| Test | Package | Result |
|------|---------|--------|
| `TestListWithOptions_SourceField` | `internal/install` | PASS |
| `TestListWithOptions_MigratedSource` | `internal/install` | PASS |
| `TestListWithOptions_MultiVersion` | `internal/install` | PASS |
| `TestListWithOptions_StaleStateEntries` | `internal/install` | PASS |
| `TestListWithOptions_EmptyVersionsMap` | `internal/install` | PASS |
| `TestListWithOptions_HiddenToolFiltering` | `internal/install` | PASS |
| `TestMigrateSourceTracking_DefaultsCentral` | `internal/install` | PASS |
| `TestMigrateSourceTracking_InfersFromPlan` | `internal/install` | PASS |
| `TestMigrateSourceTracking_Idempotent` | `internal/install` | PASS |
| `TestMigrateSourceTracking_SkipsExisting` | `internal/install` | PASS |

### Acceptance Criteria Verification

1. **Source field populated in list output**: VERIFIED
   - `InstalledTool.Source` is set from `toolState.Source` in `internal/install/list.go:60`
   - `TestListWithOptions_SourceField` confirms central ("central"), distributed ("alice/tools"), and local ("local") sources are all correctly populated

2. **Distributed tools show [owner/repo] suffix**: VERIFIED
   - `cmd/tsuku/list.go:149-152` adds `[owner/repo]` suffix when `strings.Contains(tool.Source, "/")` is true
   - Central tools ("central") and local tools ("local") do NOT contain "/" so they render without a suffix (backward compatible)

3. **JSON output includes "source" field**: VERIFIED
   - `cmd/tsuku/list.go:71` defines `Source string json:"source,omitempty"` in the JSON struct
   - `cmd/tsuku/list.go:97` maps `t.Source` into the JSON output

4. **Pre-migration state handled**: VERIFIED
   - `TestListWithOptions_MigratedSource` confirms tools without a Source field get "central" after lazy migration
   - `TestMigrateSourceTracking_InfersFromPlan` confirms inference from `Plan.RecipeSource` (registry->central, embedded->central, local->local)
   - `TestMigrateSourceTracking_Idempotent` confirms migration is safe to run multiple times

### Full Test Suite (`go test ./... -count=1`)

**Result**: 1 pre-existing FAIL in root package (lint issues), all other packages PASS

The root package failure is from `TestGolangCILint` catching pre-existing lint issues in files unrelated to Issue 9:
- `internal/distributed/client_test.go` - bodyclose
- `cmd/tsuku/install_distributed_test.go` - errcheck (3 occurrences)
- `cmd/tsuku/install_distributed.go` - misspell ("cancelled" vs "canceled")
- `cmd/tsuku/install_deps_test.go` - staticcheck (nil pointer dereference pattern)
- `internal/seed/audit_test.go` - staticcheck (nil pointer dereference pattern)

These are all on the `main` branch and not introduced by Issue 9. All packages relevant to Issue 9 pass:
- `cmd/tsuku`: PASS
- `internal/install`: PASS
- `internal/distributed`: PASS
- `internal/recipe`: PASS
- `internal/userconfig`: PASS
- `internal/config`: PASS

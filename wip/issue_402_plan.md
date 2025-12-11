# Issue 402 Implementation Plan

## Summary

Implement the `GeneratePlan` method on `Executor` that evaluates a recipe and produces an `InstallationPlan`. The generator resolves versions, expands templates, downloads files for checksum computation using PreDownloader, and classifies steps by evaluability.

## Approach

Add `GeneratePlan` as a method on `Executor` following the existing pattern of `Execute()` and `DryRun()`. The method will:
1. Resolve version using existing infrastructure
2. Compute recipe hash from TOML content
3. Process each step: expand templates, evaluate `when` clauses, download for checksums
4. Return an `InstallationPlan` with all resolved steps

For checksum computation, we'll reuse PreDownloader from `internal/validate/predownload.go` for security (HTTPS enforcement, SSRF protection).

### Alternatives Considered

- **New package for plan generation**: Rejected because the logic is tightly coupled with Executor (version resolution, template expansion). The plan types already live in executor package.
- **Separate PlanGenerator struct**: Rejected to avoid duplication - Executor already has all the necessary context (recipe, version resolver, platform).

## Files to Modify

- `internal/executor/executor.go` - Add helper method for shouldExecute with platform override, export expandVars
- `internal/executor/plan.go` - Add GeneratePlan method and helper types

## Files to Create

- `internal/executor/plan_generator.go` - Plan generation logic (keeps plan.go focused on types)
- `internal/executor/plan_generator_test.go` - Unit tests for plan generation

## Implementation Steps

- [x] Add PlanConfig struct and GeneratePlan method signature to plan_generator.go
- [x] Implement version resolution integration
- [x] Implement recipe hash computation (SHA256 of TOML)
- [x] Implement step processing with template expansion
- [x] Implement download checksum computation using PreDownloader
- [x] Add non-evaluable action warning callback
- [x] Implement platform-specific when clause filtering with override support
- [x] Add unit tests with mock recipes
- [x] Run tests and verify build

## Testing Strategy

- **Unit tests**:
  - GeneratePlan with simple evaluable recipe (no downloads)
  - GeneratePlan with non-evaluable actions (verify warnings)
  - When clause filtering (verify steps excluded)
  - Template expansion verification
  - Recipe hash computation
  - Cross-platform generation (--os, --arch override simulation)

- **Integration considerations**:
  - Download tests would require network or mocking
  - Focus on mock/fake testing for unit tests

## Key Implementation Details

### PlanConfig struct
```go
type PlanConfig struct {
    OS         string // Override OS (default: runtime.GOOS)
    Arch       string // Override Arch (default: runtime.GOARCH)
    RecipeHash string // Pre-computed recipe hash (optional)
    RecipeSource string // "registry" or file path
    OnWarning  func(step string, msg string) // Callback for warnings
}
```

### Download actions to process for checksums
- `download` - direct URL
- `download_archive` - URL from params
- `github_archive` - construct URL from repo + asset_pattern
- `github_file` - construct URL from repo + file
- `hashicorp_release` - construct URL from product + version
- `homebrew_bottle` - construct URL from formula

### Template expansion reuse
Reuse existing `expandVars` from executor.go (need to export or duplicate).

## Risks and Mitigations

- **Risk**: Network calls in tests slow/flaky
  - **Mitigation**: Mock PreDownloader for unit tests, use test fixtures

- **Risk**: Complex composite actions (github_archive) have internal download logic
  - **Mitigation**: For plan generation, resolve the URL ourselves without executing. The composite action logic can be replicated for URL construction.

## Success Criteria

- [ ] `GeneratePlan` function implemented accepting recipe, version, and platform
- [ ] PreDownloader integration for checksum computation
- [ ] Recipe hash computation (SHA256)
- [ ] Template expansion for all action parameters
- [ ] Step resolution with evaluability marking
- [ ] Non-evaluable action warnings via callback
- [ ] Platform-specific `when` clause filtering
- [ ] Cross-platform generation support via PlanConfig
- [ ] Unit tests pass
- [ ] `go build ./...` succeeds
- [ ] `go test ./internal/executor/...` passes

## Open Questions

None - the design is clear. Implementation follows existing patterns.

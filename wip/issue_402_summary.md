# Issue 402 Implementation Summary

## Changes

### New Files
- `internal/executor/plan_generator.go` - Plan generation logic
- `internal/executor/plan_generator_test.go` - Unit tests for plan generation

### Implementation Details

**PlanConfig struct** - Configuration for plan generation:
- `OS` / `Arch` - Override target platform (defaults to runtime.GOOS/GOARCH)
- `RecipeSource` - Recipe provenance ("registry" or file path)
- `OnWarning` - Callback for non-evaluable action warnings
- `Downloader` - PreDownloader instance for checksum computation

**GeneratePlan method** - Core plan generation:
1. Resolves version using existing infrastructure
2. Computes recipe hash (SHA256 of TOML content)
3. Filters steps based on `when` clauses for target platform
4. Expands template variables ({version}, {os}, {arch}) in all parameters
5. For download actions, computes checksums via PreDownloader
6. Marks steps as evaluable/non-evaluable

**Helper functions**:
- `computeRecipeHash` - SHA256 hash of recipe TOML
- `shouldExecuteForPlatform` - When clause evaluation for plan generation
- `resolveStep` - Single step resolution with template expansion
- `isDownloadAction` - Identifies download action types
- `extractDownloadURL` - Constructs URLs for various action types
- `expandParams` / `expandValue` / `expandVarsInString` - Template expansion
- `GetStandardPlanVars` - Standard variable map construction
- `ApplyOSMapping` / `ApplyArchMapping` - Platform mapping helpers

**Download actions supported**:
- `download` / `download_archive` - Direct URL
- `github_archive` / `github_file` - GitHub release URL construction
- `hashicorp_release` - HashiCorp release URL construction
- `homebrew_bottle` - Skips checksum (bottles have upstream checksums)

## Testing

All tests pass:
- `TestComputeRecipeHash` - Recipe hash computation
- `TestShouldExecuteForPlatform` - When clause filtering
- `TestIsDownloadAction` - Download action detection
- `TestExtractDownloadURL` - URL extraction for various action types
- `TestExpandParams` - Template expansion
- `TestExpandVarsInString` - String variable expansion
- `TestGetStandardPlanVars` - Standard variable map
- `TestApplyOSMapping` / `TestApplyArchMapping` - Platform mapping
- `TestGeneratePlan_BasicRecipe` - Basic plan generation
- `TestGeneratePlan_NonEvaluableWarnings` - Warning callback
- `TestGeneratePlan_WhenFiltering` - Platform filtering
- `TestGeneratePlan_TemplateExpansion` - Template expansion in plans

## Build Status
- `go build ./...` - Passes
- `go test ./internal/executor/...` - Passes

## Related Issues
- Depends on: #401 (installation plan data types) - Merged
- Blocks: #403 (plan format JSON serialization)

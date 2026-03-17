# Scrutiny Review: Completeness - Issue 9

**Issue:** #9 - feat(cli): add source display to info, list, and recipes commands
**Focus:** completeness
**Reviewer:** scrutiny-completeness

## Acceptance Criteria from Issue Body

1. `list`: human-readable output shows source suffix for distributed tools (e.g., `ripgrep 14.1.1 [alice/tools]`); `--json` includes `"source"` field
2. `info`: human-readable output includes `Source:` line; `--json` includes `"source"` field; shown for both installed and uninstalled tools
3. `recipes`: recipes from all registered distributed sources appear alongside central recipes; each entry shows its source; `--local` flag continues to show only local recipes

## Mapping Evaluation

### AC 1: `list` command - human-readable source suffix and JSON source field

**Claimed status:** implemented
**Claimed evidence:** list.go: strings.Contains check adds [source] suffix; Source field in itemJSON struct

**Verification:**

The diff shows:
- `list.go` line 150: `if strings.Contains(tool.Source, "/")` adds `[source]` suffix -- confirmed in diff
- `list.go` line 71: `Source string \`json:"source,omitempty"\`` added to `itemJSON` struct -- confirmed in diff
- `list.go` line 97: `Source: t.Source` populates the JSON field -- confirmed in diff
- `internal/install/list.go` line 60: `Source` field added to `InstalledTool` struct, populated from `toolState.Source` -- confirmed in diff

The human-readable suffix uses `strings.Contains(tool.Source, "/")` which means only distributed sources (formatted as "owner/repo") get the bracket suffix. Central and local sources are not shown, which matches the AC text ("source suffix for distributed tools"). This is correct behavior.

**Assessment:** PASS - Evidence verified in diff.

### AC 2: `info` command - Source line, JSON field, installed and uninstalled

**Claimed status:** implemented
**Claimed evidence:** info.go: Source line after Type; Source field in infoOutput struct; installed reads ToolState.Source, uninstalled uses loader.GetWithSource

**Verification:**

The diff shows:
- `info.go` line 192: `Source string \`json:"source,omitempty"\`` in `infoOutput` struct -- confirmed
- `info.go` line 218: `Source: source` populates JSON -- confirmed
- `info.go` lines 251-253: `if source != "" { fmt.Printf("Source: %s\n", source) }` -- confirmed, placed after Type line
- Installed path: `info.go` line 157: `toolSource = toolState.Source` reads from state -- confirmed
- Uninstalled path: `info.go` lines 115-127: calls `loader.GetWithSource(toolName, ...)` and maps `providerSource` to a string -- confirmed

The logic at lines 178-182 resolves the final source: installed tools use `toolSource` (from state), falling back to `recipeSource` (from provider) when empty. This handles both installed and uninstalled tools correctly.

One nuance: the `GetWithSource` call for uninstalled tools is guarded by `if recipePath == ""`, meaning `--recipe` flag usage won't show source. This is reasonable since a local recipe file doesn't come from any provider.

**Assessment:** PASS - Evidence verified in diff.

### AC 3: `recipes` command - distributed alongside central, source shown, --local unchanged

**Claimed status:** implemented
**Claimed evidence:** recipes.go: ListAllWithSource iterates all providers; distributed sources show as [owner/repo]; recipesLocalOnly logic unchanged

**Verification:**

The diff shows:
- `recipes.go` line 40: `loader.ListAllWithSource()` replaces the previous listing call -- confirmed in diff. `ListAllWithSource` in `internal/recipe/loader.go:530` iterates all providers including distributed ones.
- `recipes.go` lines 132-135: distributed sources get `[owner/repo]` indicator via `strings.Contains(string(r.Source), "/")` -- confirmed
- `recipes.go` line 33-34: `recipesLocalOnly` still calls `loader.ListLocal()` -- this path is unchanged in the diff (only the else branch changed to `ListAllWithSource`)
- JSON output added (lines 70-91) with source field for each recipe -- this is bonus functionality not explicitly in the AC but consistent with the pattern

The `--local` flag logic is preserved: when `recipesLocalOnly` is true, `loader.ListLocal()` is called, which only returns local provider recipes. This is unchanged from the previous implementation.

**Assessment:** PASS - Evidence verified in diff.

## Coverage Check

### Missing ACs: None

All three acceptance criteria from the issue body have corresponding mapping entries with verified evidence.

### Phantom ACs: None

All mapping entries correspond to criteria in the issue body.

## Test Coverage

The diff includes two new test functions in `internal/install/list_test.go`:
- `TestListWithOptions_SourceField`: tests that Source field is populated correctly for central, distributed ("alice/tools"), and local sources
- `TestListWithOptions_MigratedSource`: tests that tools without Source field get "central" after lazy migration

These tests cover the data layer (`InstalledTool.Source` field propagation). There are no command-level tests for `info`, `list`, or `recipes` output formatting, but the issue ACs don't explicitly require tests, and the existing codebase doesn't have command-level test files for these commands.

## Summary

All three acceptance criteria are fully covered and verified against the diff. No blocking or advisory findings.

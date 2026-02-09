# Coverage Dashboard Documentation

## Overview

The coverage dashboard provides visibility into which recipes support which platforms (glibc, musl, darwin). It helps contributors identify platform coverage gaps and understand the current state of platform support across the recipe catalog.

**Dashboard URL**: `/coverage/` (relative to website root)

**Purpose**: Help maintainers and contributors:
- Identify recipes missing platform support
- Track progress on platform coverage improvements
- Understand which recipes work on Alpine/musl environments
- See the distribution of library vs tool recipe coverage

## Dashboard Usage

The dashboard presents three complementary views of platform coverage data.

### Coverage Matrix

The Coverage Matrix shows all recipes in a sortable table with platform support indicators.

**Columns**:
- **Recipe**: Recipe name (clickable to sort alphabetically)
- **Type**: `library` or `tool`
- **glibc**: Support for glibc-based Linux (Debian, Ubuntu, Fedora, etc.)
- **musl**: Support for musl-based Linux (Alpine)
- **darwin**: Support for macOS

**Indicators**:
- `✓` (green): Recipe has explicit support for this platform
- `✗` (red): Recipe does not support this platform

**Sorting**: Click column headers to sort by recipe name or platform support.

**Use Cases**:
- Find all recipes that support Alpine (filter by musl column)
- Identify recipes missing macOS support
- Compare library vs tool platform coverage

### Gap List

The Gap List highlights recipes with incomplete platform support.

**Sections**:
1. **Missing Platform Support**: Recipes that don't support one or more platforms
2. **M47 Library Gaps**: Libraries specifically missing musl support (historical tracking)

**Display**: Each gap entry shows:
- Recipe name and type
- Which platform(s) are missing
- Empty state message if all recipes have full platform support

**Use Cases**:
- Prioritize which recipes to add platform support for
- Track progress on closing platform gaps
- Identify patterns in missing support (e.g., libraries vs tools)

### Category Breakdown

The Category Breakdown provides aggregate statistics split by recipe type.

**Metrics Shown**:
- **Total Recipes**: Overall count
- **Libraries**: Count and percentage with platform support statistics
- **Tools**: Count and percentage with platform support statistics

**Visual Elements**:
- Progress bars showing percentage of musl support per category
- Color coding: Red if less than 100% musl support, green otherwise
- Detailed breakdown of glibc vs musl support counts

**Use Cases**:
- Understand overall platform coverage health
- Compare library vs tool coverage (libraries have stricter requirements)
- Track improvement over time

## Data Sources

### How Coverage Data is Generated

Coverage data comes from static analysis of recipe files, not from test execution.

**Analysis Process**:
1. The `cmd/coverage-analytics` tool reads all recipe files from `recipes/` and `internal/recipe/recipes/`
2. For each recipe, it analyzes the `[[step]]` entries and their `when` clauses
3. A platform is "supported" if the recipe has at least one step that can execute on that platform
4. Results are aggregated and written to `website/coverage/coverage.json`

**Example Recipe Analysis**:
```toml
[[step]]
action = "install_homebrew_package"
package = "libcurl"
when = { libc = ["glibc"] }
# Analyzer detects: glibc ✓

[[step]]
action = "install_system_package"
apk = ["curl-dev"]
when = { libc = ["musl"] }
# Analyzer detects: musl ✓
```

### CI Automation

The dashboard data is automatically updated whenever recipes change.

**Workflow**: `.github/workflows/coverage-update.yml`

**Triggers**:
- **Automatic**: Push to `main` branch when files in `recipes/` or `internal/recipe/recipes/` change
- **Manual**: Workflow dispatch via GitHub Actions UI

**Process**:
1. GitHub Actions checks out the repository
2. Runs `go run cmd/coverage-analytics/main.go`
3. Checks if `website/coverage/coverage.json` changed
4. If changed, commits and pushes using `github-actions[bot]` account
5. Uses `[skip ci]` in commit message to prevent workflow loops

**Monitoring**: Check the [Actions tab](https://github.com/tsukumogami/tsuku/actions/workflows/coverage-update.yml) to see recent workflow runs.

## Manual Regeneration

You may need to manually regenerate coverage data when:
- Testing recipe changes locally before pushing
- Debugging coverage analysis behavior
- Verifying that a recipe change will show correct platform support

### Regeneration Command

```bash
# From repository root
go run cmd/coverage-analytics/main.go
```

**Default Behavior**:
- Reads recipes from `recipes/` and `internal/recipe/recipes/`
- Reads exclusions from `testdata/golden/execution-exclusions.json`
- Writes output to `website/coverage/coverage.json`

**Custom Paths** (optional):
```bash
go run cmd/coverage-analytics/main.go \
  --recipes recipes/ \
  --exclusions testdata/golden/execution-exclusions.json \
  --output website/coverage/coverage.json
```

### Verifying Generated Data

After regeneration, verify the JSON is valid:

```bash
# Check JSON syntax
jq '.' website/coverage/coverage.json > /dev/null && echo "Valid JSON"

# View summary statistics
jq '.summary' website/coverage/coverage.json

# Count recipes with gaps
jq '[.recipes[] | select(.gaps and (.gaps | length) > 0)] | length' website/coverage/coverage.json
```

**Expected Structure**:
```json
{
  "generated_at": "2026-02-08T12:00:00Z",
  "total_recipes": 274,
  "summary": {
    "by_platform": { ... },
    "by_category": { ... }
  },
  "recipes": [ ... ]
}
```

## Platform Support Indicators

### How Support is Determined

The analyzer determines platform support by examining recipe steps:

**Supported Platform**: Recipe has at least one `[[step]]` that can execute on the platform
- Example: Step has `when = { libc = ["musl"] }` → musl supported
- Example: Step has no `when` clause → all platforms supported

**Unsupported Platform**: Recipe has no steps that can execute on the platform
- Example: All steps have `when = { libc = ["glibc"] }` → only glibc supported, musl/darwin unsupported

**Platform Detection Logic**:
1. Check `when` clauses in all recipe steps
2. For `libc`:
   - `glibc` matches: Debian, Ubuntu, Fedora, RHEL, Arch, SUSE
   - `musl` matches: Alpine
3. For `os`:
   - `linux` matches: glibc and musl
   - `darwin` matches: macOS
4. Steps without `when` clauses run on all platforms

### Examples

**Full Platform Support**:
```toml
[[step]]
action = "download_file"
url = "https://example.com/tool-${version}.tar.gz"
# No 'when' clause → runs on all platforms: glibc ✓, musl ✓, darwin ✓
```

**Linux-Only Support**:
```toml
[[step]]
action = "install_homebrew_package"
package = "gh"
when = { os = ["linux"] }
# glibc ✓, musl ✓, darwin ✗
```

**glibc-Only Support (Common for Libraries)**:
```toml
[[step]]
action = "install_homebrew_package"
package = "openssl"
when = { libc = ["glibc"] }
# glibc ✓, musl ✗, darwin ✗
```

### Interpreting the Dashboard

**Green `✓` means**: The recipe has installation steps for this platform. This doesn't guarantee the recipe works perfectly, but it has been designed to support the platform.

**Red `✗` means**: The recipe has no installation steps for this platform. The recipe will fail if installation is attempted on this platform.

**Gaps**: Recipes with `✗` indicators are candidates for adding platform support. See the recipe file to understand what steps would be needed (e.g., adding Alpine package names for musl support).

## CI Integration Details

### Workflow Behavior

The coverage update workflow ensures the dashboard data stays current without manual intervention.

**When It Runs**:
- Every time a PR merges to `main` that modifies recipe files
- Can be triggered manually via GitHub Actions UI

**What It Does**:
1. Analyzes all recipes in the repository
2. Generates fresh `website/coverage/coverage.json`
3. Commits the file if it changed (using `github-actions[bot]`)
4. Skips CI on the commit to prevent infinite loops

**Commit Message Pattern**:
```
chore(website): regenerate coverage.json [skip ci]
```

### Checking Data Freshness

To verify coverage data is up-to-date:

```bash
# Check last generation time in the JSON
jq '.generated_at' website/coverage/coverage.json

# Check last commit to coverage.json
git log -1 --format="%ci" -- website/coverage/coverage.json
```

If the coverage data seems stale:
1. Check if recent recipe changes triggered the workflow (Actions tab)
2. Verify the workflow completed successfully
3. Manually trigger the workflow via GitHub Actions UI if needed

### Troubleshooting

**Coverage data not updating after recipe change**:
- Verify recipe file path matches trigger patterns (`recipes/**/*.toml` or `internal/recipe/recipes/**/*.toml`)
- Check Actions tab for workflow run status
- Look for workflow failures (tool compilation errors, permission issues)

**Dashboard shows incorrect support**:
- Verify recipe `when` clauses are correctly formatted
- Regenerate locally to debug: `go run cmd/coverage-analytics/main.go`
- Check analyzer output for warnings or errors

**Workflow not appearing in Actions tab**:
- Workflows only appear after merging to `main` branch
- PR branches don't show workflows until merged

## Related Resources

- **Dashboard**: `/coverage/` (view live dashboard)
- **Workflow**: [`.github/workflows/coverage-update.yml`](https://github.com/tsukumogami/tsuku/blob/main/.github/workflows/coverage-update.yml)
- **Analyzer Tool**: [`cmd/coverage-analytics/main.go`](https://github.com/tsukumogami/tsuku/blob/main/cmd/coverage-analytics/main.go)
- **Data File**: [`website/coverage/coverage.json`](https://github.com/tsukumogami/tsuku/blob/main/website/coverage/coverage.json)
- **Design Document**: [`docs/designs/DESIGN-recipe-coverage-system.md`](https://github.com/tsukumogami/tsuku/blob/main/docs/designs/DESIGN-recipe-coverage-system.md)

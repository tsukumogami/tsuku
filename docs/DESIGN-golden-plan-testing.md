# DESIGN: Golden Plan Testing

## Status

Proposed

## Context and Problem Statement

Tsuku's test suite relies heavily on integration tests that execute full installation workflows. These tests download files from external services (GitHub releases, npm registry, PyPI, crates.io, RubyGems, Homebrew bottles), perform actual installations, and verify the results. While comprehensive, this approach has significant drawbacks:

1. **Non-determinism**: Every test run makes network calls to resolve versions and download artifacts. External service availability, rate limiting, and content changes cause unpredictable failures.

2. **Slow execution**: Builder tests (cargo, pypi, npm, gem) take 30+ minutes each and are relegated to nightly schedules rather than PR validation. The main test suite takes 15+ minutes per matrix combination.

3. **Limited platform coverage**: Only 4 tools are tested on macOS versus 10 on Linux. arm64 Linux has no coverage due to Homebrew limitations.

4. **No recipe regression safety net**: Recipe changes can silently alter plan generation in unexpected ways. There is no mechanism to preview what a recipe change will produce across all supported platforms.

The `tsuku eval` command now supports generating deterministic installation plans from local recipe files (`--recipe`) with cross-platform targeting (`--os`, `--arch`). This creates an opportunity for a comprehensive golden plan testing system that validates **every recipe** across **all supported platforms** on **every PR**.

### Vision

Every recipe in the tsuku registry should have golden plans generated for all its supported platforms. These golden plans serve as:

1. **Regression detection** - Any code change that affects plan generation is immediately visible
2. **Recipe validation** - Recipe authors see exactly what their recipe produces before merging
3. **Execution gatekeeping** - Changed golden files must pass installation tests before merge

### Scope

**In scope:**
- Generating golden plans for every recipe at pinned versions
- Cross-platform plan generation (all 4 platform combinations per recipe)
- CI workflow to detect recipe changes and require golden file updates
- Execution validation when golden files change

**Out of scope:**
- Replacing execution tests entirely (some execution testing remains valuable)
- Version resolution testing (inherently dynamic)
- Testing "latest" version plans (non-deterministic)

## Decision Drivers

1. **Comprehensive coverage**: Every recipe should be validated, not just a representative sample.

2. **Change visibility**: Recipe and code changes should produce visible diffs in golden files, enabling meaningful code review.

3. **Execution validation**: Changed golden plans should be proven executable before merge.

4. **Determinism**: Golden file generation must be reproducible across environments.

5. **Maintainability**: The system should be self-maintaining - recipe changes naturally flow to golden file updates.

## Considered Options

### Option 1: Selective Golden Files (3-5 Reference Recipes)

Store golden plans only for a small set of representative recipes.

**Pros:**
- Low maintenance burden
- Small fixture footprint

**Cons:**
- Most recipes have no regression protection
- Recipe authors don't see plan output before merge
- Silent regressions possible for untested recipes

### Option 2: Comprehensive Golden Files (All Recipes)

Generate and store golden plans for every recipe across all supported platforms.

**Pros:**
- Complete regression coverage
- Recipe authors see exactly what changes
- Visible diffs for code review
- Catches edge cases in non-representative recipes

**Cons:**
- Large file count (~600 golden files for 155 recipes × 4 platforms)
- Requires automated regeneration workflow
- Storage overhead (~50-100 MB)

### Option 3: Hash-Only Validation

Store only checksums of expected plans, not full plan content.

**Pros:**
- Minimal storage
- Fast comparison

**Cons:**
- No visibility into what changed
- Debugging regressions requires regeneration
- Cannot review plan changes in PRs

## Decision Outcome

**Chosen: Option 2 - Comprehensive Golden Files**

The comprehensive approach provides the safety net needed for a growing recipe registry. While the file count is significant, the benefits outweigh the costs:

- Recipe authors see complete plan output in PR diffs
- Silent regressions are eliminated
- Code review becomes meaningful for recipe changes
- The pattern mirrors `test-changed-recipes.yml` which is already proven

### Trade-offs Accepted

- ~600 golden files to maintain in version control
- Storage overhead of ~50-100 MB
- Initial generation effort for all existing recipes

## Solution Architecture

### Overview

The golden plan system consists of four components:

1. **Golden file storage** in `testdata/golden/plans/` organized by recipe
2. **Plan generation tooling** using `tsuku eval --recipe`
3. **CI workflow** to enforce golden file updates on recipe changes
4. **Execution validation** for changed golden files

### Directory Structure

Golden files are organized with first-letter subdirectories, mirroring the recipe registry structure for scalability:

```
testdata/
├── golden/
│   └── plans/                          # Golden plan files
│       ├── b/
│       │   └── btop/                   # Linux-only recipe
│       │       ├── v1.3.0-linux-amd64.json
│       │       └── v1.3.0-linux-arm64.json
│       ├── f/
│       │   └── fzf/
│       │       ├── v0.46.0-linux-amd64.json
│       │       ├── v0.46.0-linux-arm64.json
│       │       ├── v0.46.0-darwin-amd64.json
│       │       └── v0.46.0-darwin-arm64.json
│       ├── t/
│       │   └── terraform/
│       │       └── ...
│       └── ...
├── recipes/                            # Test recipe copies (version-pinned)
│   └── ...
└── states/                             # Existing state fixtures
```

This structure enables scaling to tens of thousands of recipes without creating directories with thousands of entries.

### Naming Convention

Golden files follow the pattern: `{first-letter}/{recipe}/{version}-{os}-{arch}.json`

- `{first-letter}` - First letter of recipe name (matches `internal/recipe/recipes/{letter}/`)
- `{recipe}` - Recipe name matching the TOML filename (kebab-case)
- `{version}` - Pinned version with `v` prefix (e.g., `v0.46.0`)
- `{os}` - Target OS (`linux` or `darwin`)
- `{arch}` - Target architecture (`amd64` or `arm64`)

### Platform Filtering

Not all recipes support all platforms. Golden files are generated only for supported platforms:

| Recipe Type | Platforms | Example |
|-------------|-----------|---------|
| Cross-platform | All 4 | fzf, terraform, ripgrep |
| Linux-only | linux-amd64, linux-arm64 | btop, nix-portable |
| Darwin-only | darwin-amd64, darwin-arm64 | (rare) |
| System requirement | None | docker, cuda |

Recipes using `require_system` action cannot have plans generated and are excluded.

### Checksum Handling

Plan generation requires downloading files to compute SHA256 checksums. For golden file generation:

**Option A: Real downloads with caching**
- Generate plans with real network access
- Cache downloads to avoid re-downloading on regeneration
- Checksums reflect actual file content

**Option B: Deterministic mock downloader**
- Generate checksums from URL hashing
- No network access required
- Checksums are deterministic but don't reflect actual content

**Chosen: Option A for CI, Option B for unit tests.**

- CI golden file generation uses real downloads (validates actual checksums)
- Unit tests use mock downloader (fast, deterministic, offline)
- Both approaches produce consistent plans given the same version

### Version Pinning Strategy

Golden files are generated at pinned versions to ensure determinism:

1. **Version selection**: Use the latest stable version at time of initial golden file creation
2. **Version storage**: The version is encoded in the filename and plan content
3. **Multi-version support**: Each recipe directory can contain golden files for multiple versions
4. **Version updates**: A separate workflow can propose version bumps by adding new golden files

Example structure supporting multiple versions:
```
testdata/golden/plans/f/fzf/
├── v0.45.0-linux-amd64.json    # Previous version (kept for regression testing)
├── v0.45.0-linux-arm64.json
├── v0.46.0-linux-amd64.json    # Current version
├── v0.46.0-linux-arm64.json
├── v0.46.0-darwin-amd64.json
└── v0.46.0-darwin-arm64.json
```

The regeneration script reads existing versions from the directory to determine which versions to regenerate.

### Trust Boundaries

The golden plan system relies on the following trust assumptions:

| Trusted | What It Means |
|---------|--------------|
| CI infrastructure | GitHub Actions runners are uncompromised |
| TLS/PKI | HTTPS connections to upstream registries are secure |
| Upstream registries | GitHub, npm, PyPI, crates.io serve authentic content |
| Reviewers | Humans reviewing PRs detect malicious changes |

| Verified (Not Trusted) |  |
|------------------------|--|
| Checksums | SHA256 computed during download and stored in plans |
| Plan structure | Validated against format version and invariants |
| Platform support | Recipes generate plans only for supported platforms |

**Important**: Initial golden file generation MUST occur in CI, not on developer machines. A compromised developer workstation could inject malicious checksums.

### Residual Risks

1. **Network-enabled sandbox tests**: When `RequiresNetwork=true`, sandbox containers have full network access (`--network=host`). This is necessary for ecosystem builds (cargo, npm, pip) but means a compromised dependency could exfiltrate data. Future improvement: egress filtering to known registries only.

2. **No upstream signature verification**: Tsuku relies on HTTPS + SHA256 checksums without verifying GPG/Sigstore signatures. This is acceptable for the current threat model but could be enhanced for high-security deployments.

3. **Version bump trust window**: When versions are bumped, new checksums are computed from upstream. Reviewers must verify version bumps are legitimate.

### CI Workflow: Recipe Changes

The workflow mirrors `test-changed-recipes.yml`:

```yaml
name: Validate Golden Plans

on:
  pull_request:
    paths:
      - 'internal/recipe/recipes/**/*.toml'
      - 'testdata/golden/plans/**/*.json'

jobs:
  detect-changes:
    runs-on: ubuntu-latest
    outputs:
      recipes: ${{ steps.changed.outputs.recipes }}
      golden-changed: ${{ steps.changed.outputs.golden }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - id: changed
        run: |
          # Detect changed recipes
          RECIPES=$(git diff --name-only origin/main...HEAD -- 'internal/recipe/recipes/**/*.toml' | \
            xargs -I {} basename {} .toml | sort -u | jq -R -s -c 'split("\n")[:-1]')
          echo "recipes=$RECIPES" >> $GITHUB_OUTPUT

          # Detect changed golden files
          GOLDEN=$(git diff --name-only origin/main...HEAD -- 'testdata/golden/plans/**/*.json' | wc -l)
          echo "golden=$GOLDEN" >> $GITHUB_OUTPUT

  regenerate-golden:
    needs: detect-changes
    if: needs.detect-changes.outputs.recipes != '[]'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: 'go.mod'

      - name: Cache downloads
        uses: actions/cache@v4
        with:
          path: ~/.tsuku/cache/downloads
          key: golden-downloads-${{ hashFiles('testdata/golden/plans/**/*.json') }}
          restore-keys: |
            golden-downloads-

      - name: Build tsuku
        run: go build -o tsuku ./cmd/tsuku

      - name: Regenerate golden files for changed recipes
        run: |
          for recipe in $(echo '${{ needs.detect-changes.outputs.recipes }}' | jq -r '.[]'); do
            ./scripts/regenerate-golden.sh "$recipe"
          done

      - name: Check for uncommitted golden file changes
        run: |
          if ! git diff --exit-code testdata/golden/plans/; then
            echo "::error::Golden files are out of date. Please run './scripts/regenerate-golden.sh <recipe>' and commit the changes."
            git diff testdata/golden/plans/
            exit 1
          fi

  validate-execution:
    needs: detect-changes
    if: needs.detect-changes.outputs.golden > 0
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: 'go.mod'

      - name: Build tsuku
        run: go build -o tsuku ./cmd/tsuku

      - name: Install changed golden plans
        run: |
          # Find golden files changed in this PR that match current platform
          PLATFORM="${{ runner.os == 'Linux' && 'linux' || 'darwin' }}-${{ runner.arch == 'X64' && 'amd64' || 'arm64' }}"
          git diff --name-only origin/main...HEAD -- 'testdata/golden/plans/**/*.json' | \
            grep "$PLATFORM" | while read plan; do
              echo "Testing: $plan"
              ./tsuku install --plan "$plan" --sandbox
            done
```

### CI Workflow: Code Changes

When tsuku code changes (plan generation logic), all golden files must be validated:

```yaml
name: Validate All Golden Plans

on:
  pull_request:
    paths:
      - 'internal/executor/**'
      - 'internal/actions/**'
      - 'internal/recipe/**'
      - 'internal/validate/**'
      - 'cmd/tsuku/eval.go'
      - 'cmd/tsuku/plan*.go'

jobs:
  regenerate-all:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: 'go.mod'

      - name: Build tsuku
        run: go build -o tsuku ./cmd/tsuku

      - name: Regenerate all golden files
        run: ./scripts/regenerate-all-golden.sh

      - name: Check for uncommitted changes
        run: |
          if ! git diff --exit-code testdata/golden/plans/; then
            echo "::error::Code changes affect golden plan output. Please regenerate golden files and commit the changes."
            git diff --stat testdata/golden/plans/
            exit 1
          fi
```

### Golden File Generation Script

```bash
#!/bin/bash
# scripts/regenerate-golden.sh - Regenerate golden files for a recipe

set -euo pipefail

RECIPE="$1"
FIRST_LETTER="${RECIPE:0:1}"
RECIPE_PATH="internal/recipe/recipes/${FIRST_LETTER}/${RECIPE}.toml"
GOLDEN_DIR="testdata/golden/plans/${FIRST_LETTER}/${RECIPE}"

if [[ ! -f "$RECIPE_PATH" ]]; then
    echo "Recipe not found: $RECIPE_PATH"
    exit 1
fi

mkdir -p "$GOLDEN_DIR"

# Get supported platforms from recipe metadata (format: "linux-amd64", "darwin-arm64", etc.)
PLATFORMS=$(./tsuku info --recipe "$RECIPE_PATH" --metadata-only --json | jq -r '.supported_platforms[]')

if [[ -z "$PLATFORMS" ]]; then
    echo "No supported platforms found for $RECIPE"
    exit 0
fi

# Get all versions from existing golden files, or use latest if none exist
if [[ -d "$GOLDEN_DIR" ]] && ls "$GOLDEN_DIR"/*.json >/dev/null 2>&1; then
    VERSIONS=$(ls "$GOLDEN_DIR"/*.json | sed 's/.*\/v\([^-]*\)-.*/\1/' | sort -u)
else
    VERSIONS=$(./tsuku info "$RECIPE" --json | jq -r '.latest_version')
fi

# Regenerate for each version
for VERSION in $VERSIONS; do
    echo "Regenerating $RECIPE@$VERSION..."

    # Generate only for supported platforms
    for platform in $PLATFORMS; do
        os="${platform%-*}"
        arch="${platform#*-}"
        OUTPUT="$GOLDEN_DIR/v${VERSION}-${platform}.json"

        if ./tsuku eval --recipe "$RECIPE_PATH" --os "$os" --arch "$arch" \
            "$RECIPE@$VERSION" > "$OUTPUT.tmp" 2>/dev/null; then
            mv "$OUTPUT.tmp" "$OUTPUT"
            echo "  Generated: $OUTPUT"
        else
            rm -f "$OUTPUT.tmp"
            echo "  Failed: $OUTPUT"
        fi
    done
done

# Clean up golden files for platforms no longer supported
find "$GOLDEN_DIR" -name "*.json" | while read -r file; do
    platform=$(basename "$file" | sed 's/v[^-]*-//' | sed 's/\.json//')
    if ! echo "$PLATFORMS" | grep -qx "$platform"; then
        echo "  Removing unsupported: $file"
        rm -f "$file"
    fi
done
```

### Plan Comparison for Tests

Unit tests use a comparison utility that handles unstable fields:

```go
// internal/testutil/golden.go

type GoldenOptions struct {
    IgnoreFields    []string // Fields to mask before comparison
    UpdateOnFailure bool     // Write actual output as new golden file
}

var DefaultGoldenOptions = GoldenOptions{
    IgnoreFields: []string{"generated_at", "recipe_source"},
}

func ComparePlanToGolden(t *testing.T, actual *executor.InstallationPlan, goldenPath string, opts GoldenOptions) {
    t.Helper()

    actualJSON := marshalWithMasking(actual, opts.IgnoreFields)

    if opts.UpdateOnFailure || os.Getenv("UPDATE_GOLDEN") == "1" {
        writeGoldenFile(t, goldenPath, actualJSON)
        return
    }

    expectedJSON := readGoldenFile(t, goldenPath)
    expectedJSON = maskFields(expectedJSON, opts.IgnoreFields)

    if !bytes.Equal(actualJSON, expectedJSON) {
        diff := computeDiff(expectedJSON, actualJSON)
        t.Errorf("Golden file mismatch:\n%s\n\nRun with UPDATE_GOLDEN=1 to update", diff)
    }
}
```

## Implementation Approach

### Phase 0: Recipe Metadata Command

**Implemented**: #706 added `--recipe` and `--metadata-only` flags to `tsuku info`:

```bash
# From registry
tsuku info fzf --json

# From local file (fast, no dependency resolution)
tsuku info --recipe path/to/recipe.toml --metadata-only --json
```

Output includes `supported_platforms` array:
```json
{
  "name": "fzf",
  "description": "A command-line fuzzy finder",
  "type": "tool",
  "supported_platforms": ["linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64"],
  ...
}
```

This enables programmatic tooling for golden plan management and other automation.

### Phase 1: Infrastructure

1. Create `scripts/regenerate-golden.sh` for single-recipe regeneration
2. Create `scripts/regenerate-all-golden.sh` for full regeneration
3. Add `internal/testutil/golden.go` with comparison utilities
4. Add `internal/testutil/mock_downloader.go` for unit tests

### Phase 2: CI Workflows (Before Bulk Generation)

1. Add `validate-golden-recipes.yml` for recipe change validation
2. Add `validate-golden-code.yml` for code change validation
3. Add download caching using `actions/cache` keyed by recipe+version
4. Add execution validation step using `--sandbox` flag

### Phase 3: Tiered Golden File Generation

**Tier 1 (Pilot)**: Generate golden files for ~30 recipes covering all action types:
- GitHub archive downloads (fzf, ripgrep, lazygit)
- Direct URL downloads (terraform, golang, nodejs)
- Ecosystem builds (cargo-audit, amplify, jekyll)
- Dependencies (sqlite-source → readline)
- Platform-specific (btop, nix-portable)

Validate approach and iterate on tooling.

**Tier 2 (Expansion)**: Generate remaining ~125 recipes in batches, running in CI to ensure checksums are computed on trusted infrastructure.

### Phase 4: Documentation and Automation

1. Document golden file update workflow in CONTRIBUTING.md
2. Add pre-commit hook option for local validation
3. Create GitHub Action for automated version bump PRs
4. Update PR template to note golden file requirements

## Security Considerations

### Download Verification

Golden files contain checksums computed from real downloads. When a golden file changes, the CI workflow validates the plan can be executed with `tsuku install --plan --sandbox`. This ensures:

- Checksums match actual downloadable artifacts
- Plans are executable, not just syntactically valid
- Sandbox isolation prevents malicious payload execution

### Execution Isolation

Execution validation uses the `--sandbox` flag which runs installations in an isolated container. This provides:

- No modification to host system
- Resource limits (memory, CPU, timeout)
- Network isolation for non-download actions

### Supply Chain Risks

Golden files act as a cryptographic snapshot of expected artifacts:

- Checksum changes are visible in PR diffs
- Unexpected checksum changes indicate potential supply chain attack
- Version pinning prevents "latest" from silently changing

If an upstream artifact is republished with different content:
1. CI fails (checksum mismatch)
2. Developer investigates the change
3. If legitimate, golden file is updated with review

### User Data Exposure

Golden plan tests do not access user data. All operations are confined to:

- Reading recipe files from the repository
- Writing JSON files to testdata directory
- Generating deterministic output from public inputs

## Consequences

### Positive

- **Complete regression coverage**: Every recipe has validated golden files
- **Recipe change visibility**: PR diffs show exactly how plans change
- **Execution confidence**: Changed golden files must pass installation
- **Cross-platform validation**: All 4 platforms tested from single runner
- **Self-documenting**: Golden files show expected plan structure

### Negative

- **Storage overhead**: ~50-100 MB of JSON files in repository
- **Initial effort**: Generating golden files for 155 recipes
- **Regeneration time**: Full regeneration requires downloading artifacts
- **Version management**: Pinned versions need occasional updates

### Mitigations

- **Selective regeneration**: Only regenerate changed recipes on most PRs
- **Caching**: Download cache reduces regeneration time
- **Automation**: Scripts handle regeneration complexity
- **Version bump workflow**: Automated PRs for version updates

## Plan Validity and Forward Compatibility

### Format Version

Plans include a `format_version` field that enables forward compatibility:

```go
const PlanFormatVersion = 3

func ValidatePlan(plan *InstallationPlan) error {
    if plan.FormatVersion < 2 {
        return fmt.Errorf("plan format version %d is too old", plan.FormatVersion)
    }
    // ...
}
```

When format changes:
1. Increment `PlanFormatVersion`
2. Regenerate all golden files
3. PR shows diff of format changes across all recipes

### Execution Validation

Golden files are validated through execution on every change:

| Validation Type | When | How |
|-----------------|------|-----|
| Structural | Every PR | Go tests with `AssertPlanInvariants()` |
| Checksum freshness | Golden file change | `tsuku install --plan --sandbox` |
| Cross-platform | Recipe change | Generate for all 4 platforms |

### What Happens When Tsuku Changes

| Change Type | Effect | Action |
|-------------|--------|--------|
| New plan field | Golden files unchanged | None needed |
| Field removed | Regeneration changes files | Commit updated files |
| Step order change | Regeneration changes files | Review and commit |
| Format version bump | All files change | Regenerate all, review |
| New action type | New recipes may use it | Add structural tests |

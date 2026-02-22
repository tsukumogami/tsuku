# Recipe Validation Workflow Guide

This guide explains how to use the `validate-all-recipes` workflow to identify recipes that fail on specific platforms and automatically add platform constraints.

## Prerequisites

- The workflow must exist in the main branch (from PR #1529)
- User must have write access to the repository
- GitHub Actions must be enabled

## Two-Phase Validation Process

### Phase 1: Identify Failures (Review Mode)

**Purpose**: Discover which recipes fail on which platforms without making changes.

1. Navigate to GitHub Actions tab: https://github.com/tsukumogami/tsuku/actions/workflows/validate-all-recipes.yml
2. Click "Run workflow" button (top right)
3. Select branch: `main`
4. Set `auto_constrain`: **`false`**
5. Click "Run workflow"

**What happens:**
- Workflow tests all non-library, non-excluded recipes across 11 platforms:
  - Linux x86_64: Debian (glibc), RHEL (glibc), Arch (glibc), openSUSE (glibc), Alpine (musl)
  - Linux arm64: Debian (glibc), RHEL (glibc), Arch (glibc), Alpine (musl)
  - macOS: arm64 (darwin), x86_64 (darwin)
- Results show in workflow summary with pass/fail status per platform
- No changes are made to the repository

**Expected results:**
- Most recipes pass on glibc-based Linux (Debian, RHEL, Arch, openSUSE)
- Some recipes may fail on Alpine (musl) due to library compatibility
- Some recipes may fail on specific architectures (arm64 vs x86_64)
- Platform-specific tools may fail on macOS or Linux

**Review findings:**
- Check the workflow run summary for failure patterns
- Note which recipes consistently fail on Alpine/musl
- Identify architecture-specific failures (arm64-only or x86_64-only)
- Document major patterns in issue #1540

### Phase 2: Auto-Generate Constraints (PR Creation)

**Purpose**: Automatically create a PR that adds platform constraints to failing recipes.

1. Navigate to the same workflow: https://github.com/tsukumogami/tsuku/actions/workflows/validate-all-recipes.yml
2. Click "Run workflow"
3. Select branch: `main`
4. Set `auto_constrain`: **`true`**
5. Click "Run workflow"

**What happens:**
- Workflow re-runs validation on all platforms
- For each failing recipe, invokes `scripts/write-platform-constraints.sh`
- Script adds `constraints.platforms` fields to recipe TOML files
- Creates a new branch: `auto-constrain-YYYYMMDD-HHMMSS`
- Commits all constraint changes
- Opens a pull request with the changes

**Expected PR contents:**
- Changes only to recipe files (`recipes/*/*.toml`)
- Added fields like:
  ```toml
  [constraints]
  platforms = [
    "linux-glibc-x86_64",
    "linux-glibc-arm64",
    "darwin-arm64",
    "darwin-x86_64"
  ]
  ```
- Recipes that failed on Alpine will exclude musl platforms
- Recipes that failed on specific architectures will exclude those archs
- PR description summarizes which recipes were constrained

**Next steps:**
- Do NOT merge the PR immediately
- Link the PR in issue #1540
- Review the PR in issue #1543 (validate constraints match actual failures)
- Merge only after review confirms constraints are accurate

## Platform Naming Convention

Platforms use the format: `{os}-{family}-{libc}-{arch}` or `{os}-{family}-{arch}`

| Platform ID | OS | Family | Libc | Architecture |
|-------------|----|----|------|--------------|
| `linux-debian-glibc-x86_64` | Linux | Debian | glibc | x86_64 |
| `linux-rhel-glibc-x86_64` | Linux | RHEL/Fedora | glibc | x86_64 |
| `linux-arch-glibc-x86_64` | Linux | Arch | glibc | x86_64 |
| `linux-suse-glibc-x86_64` | Linux | openSUSE | glibc | x86_64 |
| `linux-alpine-musl-x86_64` | Linux | Alpine | musl | x86_64 |
| `linux-debian-glibc-arm64` | Linux | Debian | glibc | arm64 |
| `linux-rhel-glibc-arm64` | Linux | RHEL/Fedora | glibc | arm64 |
| `linux-arch-glibc-arm64` | Linux | Arch | glibc | arm64 |
| `linux-alpine-musl-arm64` | Linux | Alpine | musl | arm64 |
| `darwin-arm64` | macOS | - | - | arm64 |
| `darwin-x86_64` | macOS | - | - | x86_64 |

## Constraint Logic

When a recipe fails on a platform, the auto-constraint script:

1. Reads existing `constraints.platforms` (if any)
2. Builds list of all validated platforms (11 total)
3. Removes platforms where the recipe failed
4. Writes updated `constraints.platforms` with only passing platforms

**Example**: If a recipe works everywhere except Alpine:
```toml
[constraints]
platforms = [
  "linux-debian-glibc-x86_64",
  "linux-rhel-glibc-x86_64",
  "linux-arch-glibc-x86_64",
  "linux-suse-glibc-x86_64",
  "linux-debian-glibc-arm64",
  "linux-rhel-glibc-arm64",
  "linux-arch-glibc-arm64",
  "darwin-arm64",
  "darwin-x86_64"
]
```

## Troubleshooting

**Workflow doesn't appear in Actions tab:**
- Ensure PR #1529 is merged to main
- Workflow files only show up when they exist in the default branch
- Check `.github/workflows/validate-all-recipes.yml` exists in main

**Workflow fails during validation:**
- Check individual job logs for specific errors
- Common issues: network timeouts, package manager failures, binary download errors
- Retry the workflow - transient failures happen

**Auto-constraint PR not created:**
- Check workflow logs for PR creation step
- Ensure `auto_constrain` was set to `true`
- Verify GitHub token has permissions to create PRs
- Check if a PR already exists from a previous run

**Too many recipes constrained:**
- Review Phase 1 results before running Phase 2
- If many recipes fail, investigate if there's a systematic issue (e.g., registry URL broken)
- Consider fixing root cause before adding constraints

## CI Batch Configuration

Recipe CI workflows group multiple recipes into batched jobs instead of running one job per recipe. This keeps job counts manageable when a PR touches many recipes at once. A PR changing 60 recipes produces ~4 batched jobs rather than 60 individual ones.

### How Batching Works

The detection job in each workflow splits the list of changed recipes into fixed-size groups using ceiling division. Each group becomes one matrix entry. The execution job builds tsuku once, then loops through all recipes in its batch with per-recipe `$TSUKU_HOME` isolation and `::group::` log annotations.

For example, with a batch size of 15 and 47 changed recipes: `ceil(47 / 15) = 4` jobs, each handling 11-15 recipes.

### Configuration File

Batch sizes are configured in `.github/ci-batch-config.json`:

```json
{
  "batch_sizes": {
    "test-changed-recipes": {
      "linux": 15
    },
    "validate-golden-recipes": {
      "default": 20
    }
  }
}
```

Each workflow reads its batch size from this file during the detection step. If the file is missing, workflows fall back to built-in defaults (15 for test-changed-recipes, 20 for validate-golden-recipes).

**Why the sizes differ:** `test-changed-recipes` installs and runs each tool, which takes longer per recipe. Golden file validation just regenerates plans and compares them, so it can handle more recipes per batch.

### Manual Override via workflow_dispatch

Both `test-recipe-changes.yml` and `validate-recipe-golden-files.yml` accept a `batch_size_override` input when triggered manually:

1. Go to Actions and select the workflow
2. Click "Run workflow"
3. Enter a value in the `batch_size_override` field (1-50, or 0 to use the config default)
4. The override applies only to that run and doesn't change the config file

This is useful for experimenting with different batch sizes to find the right balance for your workload. Values outside the 1-50 range are clamped automatically with a warning in the workflow log.

### Tuning Guidelines

The batch size controls a tradeoff: larger batches mean fewer jobs (less queue and cold-start overhead) but longer individual job durations.

**When to decrease the batch size:**
- Batched jobs regularly take longer than 10 minutes
- You're hitting workflow timeout limits
- A single slow recipe is bottlenecking entire batches

**When to increase the batch size:**
- Most batched jobs finish in under 3 minutes
- You're still seeing many small jobs for large PRs
- Queue pressure from too many concurrent jobs is slowing things down

To change a batch size, edit `.github/ci-batch-config.json` and commit the change. No workflow YAML modifications are needed.

### Finding Per-Recipe Results

Because each batched job covers multiple recipes, individual recipe results appear inside the job's log rather than as separate check entries. To find results for a specific recipe:

1. Open the workflow run in the GitHub Actions UI
2. Click on the batch job (e.g., "Linux (batch 2/4)")
3. Expand the `::group::` section for the recipe you're interested in

Failed recipes also produce `::error::` annotations that appear in the job summary.

### Follow-Up: validate-golden-execution.yml

The `validate-golden-execution.yml` workflow still uses per-recipe matrix jobs for three of its steps (`validate-coverage`, `execute-registry-linux`, `validate-linux`). These have the same scaling problem but weren't included in the initial batching work because their matrix shapes differ from the other workflows. Batching these jobs is tracked as follow-up work.

## Related Issues

- #1529: PR that introduces this workflow
- #1540: Issue for triggering the workflow (this process)
- #1543: Issue for reviewing and merging the auto-generated PR
- #1092: Musl library support (explains why Alpine failures are expected)

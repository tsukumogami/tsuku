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

## Related Issues

- #1529: PR that introduces this workflow
- #1540: Issue for triggering the workflow (this process)
- #1543: Issue for reviewing and merging the auto-generated PR
- #1092: Musl library support (explains why Alpine failures are expected)

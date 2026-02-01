---
status: Proposed
problem: Batch-generated recipes are validated only on Linux x86_64 but claim all-platform support, so users on arm64 or macOS can get broken installs.
decision: Add platform validation jobs to the batch workflow that test on 12 target environments (5 linux families x 2 architectures + 2 macOS), then write platform constraints for partial-coverage recipes before creating the PR.
rationale: In-workflow validation catches platform failures before the PR exists, enables the merge job to write accurate platform constraints, and keeps progressive promotion from cheap Linux runners to expensive macOS runners.
---

# Batch Multi-Platform Validation

**Status**: Proposed

## Upstream Design Reference

This design implements the platform validation jobs (Job 3) and merge job platform-constraint writing (Job 4) from [DESIGN-batch-recipe-generation.md](DESIGN-batch-recipe-generation.md).

**Relevant sections:**
- Job Architecture (Jobs 3-4)
- Decision 2A: Progressive validation strategy
- Validation Flow: Platform promotion logic

## Context and Problem Statement

The batch recipe generation pipeline creates recipes from ecosystem package managers (Homebrew, npm, crates.io, etc.) and validates them on a single platform: Linux x86_64 glibc on `ubuntu-latest`. Recipes that pass get included in an auto-generated PR.

These recipes claim to support all platforms by default. A recipe's `os_mapping` and `arch_mapping` resolve to platform-specific download URLs at install time, but nobody verifies those URLs point to working binaries on each platform. Homebrew bottles resolve differently per linux family (debian, rhel, arch, suse, alpine), so a recipe that works on debian may fail on fedora due to different bottle availability or shared library dependencies. When a user on macOS, ARM64, or a non-debian distro runs `tsuku install helm`, the recipe may download a binary that doesn't exist, is the wrong architecture, or fails to execute.

Manually-submitted recipe PRs already get multi-platform validation via `test-changed-recipes.yml`, which runs `tsuku install` on Linux (per-recipe matrix) and macOS (aggregated). But batch-generated PRs skip this effective validation because the batch workflow creates the PR after only Linux x86_64 testing. The PR does trigger `test-changed-recipes.yml`, but auto-merge can happen before those checks complete if the merge job doesn't wait for them.

The core question is where multi-platform validation should happen: inside the batch workflow (before creating the PR) or via PR CI (after creating the PR, before merge).

### Scope

**In scope:**
- Platform validation for batch-generated recipes across 12 target environments (5 linux families x 2 architectures + 2 macOS)
- Platform constraint writing for partial-coverage recipes
- Integration with the merge job (Job 4) from the batch design

**Out of scope:**
- Sandbox container validation for non-homebrew builders (#1287)
- Circuit breaker integration (#1255)
- SLI metrics collection (#1257, consumes platform results but is separate)
- PR-time golden file diff validation for new registry recipes (separate concern; new recipes can't have diff-based regression detection without a baseline, and execution validation already happens via `test-changed-recipes.yml`)

## Decision Drivers

- **macOS CI budget**: 1000 minutes/week. macOS runners cost 10x Linux. A 25-recipe batch uses ~110 macOS minutes (2 jobs); Linux jobs are cheap regardless of family count since containers run on the same runner.
- **Progressive savings**: Most failures are platform-independent (bad URL patterns, missing deps). Catching them on Linux first avoids spending macOS minutes on known-broken recipes.
- **Partial coverage acceptable**: A recipe that works on Linux but not macOS is still useful to Linux users. Better to merge with accurate constraints than discard entirely.
- **Platform constraint timing**: The merge job needs to know which platforms each recipe supports so it can write `supported_os`/`unsupported_platforms` fields before creating the PR.
- **Consistency**: Batch and manual recipe PRs should ideally use the same validation, or at least produce equivalent results.
- **CLI boundary**: The batch pipeline shells out to `tsuku` CLI commands, exercising the same code path users run.

## Implementation Context

### Existing Patterns

**Batch orchestrator** (`internal/batch/orchestrator.go`): Runs `tsuku create` then `tsuku install --force --recipe` sequentially per package. Classifies failures by exit code (5=network/retry, 8=missing dep, 9=deterministic insufficient). Records results to JSONL.

**Platform constraint fields** (`internal/recipe/platform.go`): Recipes support `supported_os`, `supported_arch`, `supported_libc`, and `unsupported_platforms`. The planner already respects these. Fully implemented.

**test-changed-recipes.yml**: Runs `tsuku install` on Linux (matrix per recipe) and macOS (aggregated) for PRs that change recipe files. Detects Linux-only recipes and skips macOS for them. Has execution-exclusions.json for recipes that can't be tested.

**publish-golden-to-r2.yml**: Generates golden files on 3 platforms post-merge and uploads to R2. Triggered automatically when recipe files change on main.

### Conventions to Follow

- Workflow jobs pass data via uploaded artifacts
- Failure JSONL uses `schema_version` field and structured categories
- Environment names: `{os}-{libc}-{arch}` (e.g., `linux-glibc-x86_64`)
- Skip flags: `workflow_dispatch` inputs with boolean type

## Considered Options

### Option 1: Platform Matrix Jobs in Batch Workflow

Add platform validation jobs to `batch-generate.yml` that run after generation, plus a merge job that aggregates results and writes platform constraints. This is the Job 3-4 architecture from the batch design doc.

Each Linux platform job runs all 5 family containers (debian, rhel, arch, suse, alpine) sequentially on a single runner, using Docker for family isolation — the same pattern `platform-integration.yml` already uses for alpine on arm64. Each macOS job validates directly on the runner. All jobs produce JSON artifacts with per-recipe, per-family pass/fail results. The merge job collects all results, writes `supported_os`/`unsupported_platforms`/`supported_libc` for partial-coverage recipes, and creates the PR.

This gives 12 target environments (5 families x 2 linux architectures + 2 macOS) using only 4 workflow jobs.

**Pros:**
- Validates before PR creation; broken recipes never enter the PR queue
- Full family coverage catches family-specific bottle resolution failures
- Platform results available in batch artifacts for analysis and metrics
- Merge job can write accurate constraints based on actual results
- Aligns with batch design doc architecture (Jobs 3-4)
- Only 4 jobs despite 12 environments (container reuse on Linux runners)

**Cons:**
- macOS jobs cost 10x (mitigated by progressive strategy)
- Linux jobs take longer due to sequential family testing (~5 families x recipe count)
- Duplicates some validation that `test-changed-recipes.yml` would also do on the PR
- Two parallel multi-platform validation systems to maintain

### Option 2: Batch PR Triggers test-changed-recipes.yml

The batch merge job creates a PR with all Linux-passing recipes. `test-changed-recipes.yml` triggers on that PR (it already watches `recipes/**/*.toml`). Auto-merge waits for required checks to pass.

Recipes that fail macOS validation in PR CI block the batch PR for review. The merge job doesn't write platform constraints itself; instead, failing recipes are investigated manually or the batch is re-run with those recipes excluded.

**Pros:**
- Zero new validation infrastructure (reuses existing workflow)
- Single validation system for manual and batch PRs
- Already handles Linux matrix + macOS aggregation, execution exclusions, Linux-only detection
- Same macOS cost (validation runs either way)
- Less code to maintain

**Cons:**
- Can't write platform constraints before PR creation (test-changed-recipes.yml doesn't produce structured per-platform results)
- A single macOS failure blocks the entire batch PR (no partial-coverage merge)
- Slower batch cycle (generation finishes, PR created, then waits for PR CI)
- No ARM64, musl, or non-debian family validation (`test-changed-recipes.yml` only tests on `ubuntu-latest` and `macos-latest`)
- Batch-specific logic (constraint writing, failure recording) would need to live in a separate post-CI step

### Option 3: Tiered Validation (Plans + URL Pre-filter, Then Install)

Generate installation plans for all 5 platforms on a single Linux runner using `tsuku eval`. Validate that:
1. Plans generate without errors
2. Download URLs return HTTP 200 (HEAD request)
3. Platform mappings produce valid URLs for each target

Recipes that pass the plan check get promoted to install validation on a subset of platforms (linux-amd64 + darwin-arm64 only, to save cost).

**Pros:**
- Plan validation is very cheap (no downloads, no extraction, one Linux runner)
- Catches URL pattern errors, architecture mapping mistakes, missing platform variants
- Install validation only on the subset saves ~60% vs full matrix
- Could be combined with Option 1 as a pre-filter stage

**Cons:**
- Plan generation alone misses runtime failures (binary wrong format, missing shared libs)
- URL HEAD checks can be slow at scale (hundreds of requests, rate limiting)
- Two-stage validation adds workflow complexity
- The subset of platforms for install validation still needs the same runner infrastructure as Option 1

### Option 4: Container Matrix on Linux Only

Run all validations inside Docker containers on Linux runners, using QEMU for ARM64 emulation. Skip macOS entirely.

**Pros:**
- No macOS runners needed
- All validation on cheap Linux runners

**Cons:**
- QEMU emulation is slow and sometimes unreliable
- Can't validate macOS binaries (different binary format, dylibs, codesigning)
- Most Homebrew bottles have macOS variants that would go untested
- Marking everything Linux-only defeats multi-platform support

### Evaluation Against Decision Drivers

| Driver | Option 1: Batch Matrix | Option 2: PR CI Reuse | Option 3: Tiered | Option 4: Containers |
|--------|----------------------|----------------------|-----------------|---------------------|
| macOS budget | Fair (progressive) | Fair (same cost) | Good (subset only) | Good (none) |
| Progressive savings | Good | N/A (no progression) | Good (plan pre-filter) | N/A |
| Partial coverage | Good (per-platform) | Poor (blocks PR) | Fair (subset only) | Poor (no macOS) |
| Constraint timing | Good (before PR) | Poor (after PR) | Good (before PR) | Poor (no macOS data) |
| Consistency | Fair (parallel system) | Good (single system) | Fair (parallel) | Poor (no macOS) |
| CLI boundary | Good | Good | Fair (plan, not install) | Good |

### Uncertainties

- ARM64 runner availability on GitHub Actions (`ubuntu-24.04-arm` is relatively new)
- What fraction of failures are platform-independent vs platform-specific (determines progressive savings)
- How many recipes will need partial-coverage constraints in practice

## Decision Outcome

**Chosen option: Option 1 (Platform Matrix Jobs in Batch Workflow)**

This is the right approach because the merge job needs per-platform pass/fail results to write accurate platform constraints before creating the PR. None of the other options produce structured per-platform results that the merge job can consume.

### Rationale

Option 1 was chosen because:
- **Platform constraint timing** is the deciding factor. The merge job must know which platforms each recipe supports to write `supported_os`/`unsupported_platforms` before the PR is created. Option 2 (PR CI reuse) can't provide this because `test-changed-recipes.yml` doesn't produce structured per-platform artifacts.
- **Partial coverage** is a key requirement. When a recipe works on Linux but fails on macOS, the merge job should add `supported_os = ["linux"]` and still include the recipe. Option 2 blocks the entire PR on any failure, losing the partial-coverage benefit.
- **Progressive validation** directly addresses the macOS budget constraint by only running expensive macOS jobs on recipes that pass Linux first.

Alternatives were rejected because:
- **Option 2 (PR CI reuse)**: Can't write platform constraints before PR creation, and a single platform failure blocks the whole batch. Good idea in principle but doesn't support the partial-coverage workflow.
- **Option 3 (Tiered)**: Plan pre-filtering is a good optimization but doesn't eliminate the need for install validation infrastructure. Could be added later as a pre-filter stage within Option 1.
- **Option 4 (Containers)**: Can't validate macOS binaries. Not viable for a multi-platform tool manager.

### Trade-offs Accepted

- **Two validation systems**: Batch workflow has its own platform matrix alongside `test-changed-recipes.yml`. This means two places to update when validation logic changes.
- **macOS budget pressure**: Progressive validation mitigates this (~80% savings) but large batches still consume significant budget.
- **Workflow complexity**: `batch-generate.yml` grows from 1 job to 6 jobs (preflight, generate, 2 Linux validators, 2 macOS validators, merge).

These are acceptable because the batch pipeline has fundamentally different needs from PR CI: it must produce per-platform results for constraint writing and support partial-coverage merging, which PR CI wasn't designed for.

## Solution Architecture

### Overview

The batch workflow gains four platform validation jobs (one per target environment) and a merge job. The generation job (existing) validates on Linux x86_64 glibc and uploads passing recipes as artifacts. Platform jobs download those artifacts, validate on their target environment, and upload per-platform results. The merge job aggregates all results, writes platform constraints for partial-coverage recipes, and creates the PR.

### Workflow Structure

```
batch-generate.yml:

  preflight (#1252)
      │
  generate (existing, Linux x86_64 glibc/debian)
      │
      ├── validate-linux-x86_64   (ubuntu-latest, 5 family containers, if !skip_linux_families)
      ├── validate-linux-arm64    (ubuntu-24.04-arm, 5 family containers, if !skip_arm64)
      ├── validate-darwin-arm64   (macos-14, if !skip_macos)
      └── validate-darwin-x86_64  (macos-13, if !skip_macos)
      │
  merge (aggregates results, writes constraints, creates PR)
```

Each Linux validation job runs all 5 family containers (debian, rhel, arch, suse, alpine) sequentially on the same runner using Docker, following the same pattern as `platform-integration.yml`'s alpine-arm64 job.

### Platform Job Specification

Each platform job:

1. Downloads recipe artifacts from the generation job
2. Builds or downloads `tsuku` for the target platform
3. Validates recipe paths contain no `..` segments (path traversal protection)
4. For each recipe, runs `tsuku install --force --recipe <path>` with a 5-minute per-recipe timeout
5. Classifies exit codes:
   - 0: pass
   - 5 (ExitNetwork): retry up to 3 times with exponential backoff (2s, 4s, 8s)
   - All other non-zero: fail (no retry)
   - Timeout: fail (no retry)
6. Writes results to `validation-results-<platform>.json`
7. Uploads results as workflow artifact
8. Total job timeout: 120 minutes (GitHub Actions `timeout-minutes`)

**Result artifact format:**
```json
[
  {"recipe": "helm", "platform": "linux-debian-glibc-arm64", "status": "pass", "exit_code": 0, "attempts": 1},
  {"recipe": "helm", "platform": "linux-rhel-glibc-arm64", "status": "pass", "exit_code": 0, "attempts": 1},
  {"recipe": "helm", "platform": "linux-alpine-musl-arm64", "status": "fail", "exit_code": 7, "attempts": 1},
  {"recipe": "xz", "platform": "linux-debian-glibc-arm64", "status": "fail", "exit_code": 7, "attempts": 1}
]
```

Platform IDs use the format `{os}-{family}-{libc}-{arch}` for Linux and `{os}-{arch}` for macOS.

### Platform Environments

| Job | Runner | Environments | Skip Flag |
|-----|--------|-------------|-----------|
| generate (existing) | `ubuntu-latest` | `linux-debian-glibc-x86_64` | N/A (always runs) |
| validate-linux-x86_64 | `ubuntu-latest` | 5 family containers: `linux-{debian,rhel,arch,suse}-glibc-x86_64`, `linux-alpine-musl-x86_64` | `skip_linux_families` |
| validate-linux-arm64 | `ubuntu-24.04-arm` | 5 family containers: `linux-{debian,rhel,arch,suse}-glibc-arm64`, `linux-alpine-musl-arm64` | `skip_arm64` |
| validate-darwin-arm64 | `macos-14` | `darwin-arm64` | `skip_macos` |
| validate-darwin-x86_64 | `macos-13` | `darwin-x86_64` | `skip_macos` |

**Container images for Linux family validation:**

| Family | Container Image | Libc |
|--------|----------------|------|
| debian | `debian:bookworm-slim` | glibc |
| rhel | `fedora:41` | glibc |
| arch | `archlinux:base` | glibc |
| suse | `opensuse/tumbleweed` | glibc |
| alpine | `alpine:3.21` | musl |

These are the same images used by `platform-integration.yml`. ARM64 variants exist for all of them on Docker Hub.

### Merge Job Logic

The merge job runs after all platform jobs complete (including skipped ones). It distinguishes between **skipped** platforms (skip flag was true, no result artifact exists) and **failed** platforms (job ran, recipe failed). Skipped platforms are excluded from constraint derivation; the recipe is treated as "untested" on those platforms rather than "unsupported."

1. **Downloads all result artifacts** from platform jobs (absent artifacts = skipped platform)
2. **Builds the result matrix**: per-recipe, per-platform pass/fail/skipped
3. **For each recipe**:
   - If passed all platforms: include in PR with no constraint changes
   - If passed some platforms (partial coverage): write platform constraints to recipe TOML, include in PR
   - If passed only linux-glibc-x86_64 (generation platform) and failed all validation platforms: still include with restrictive constraints
   - If has `run_command` action: exclude from PR (security gate)
4. **Creates PR** with passing/constrained recipes, failure JSONL, and queue updates

### Platform Constraint Derivation

When a recipe has partial coverage, the merge job derives the minimum constraint set. The algorithm aggregates family-level results up to the dimensions that recipe constraints support (`supported_os`, `supported_libc`, `unsupported_platforms`).

**All macOS fails, all Linux passes:**
```
Passed: all 10 linux environments
Failed: darwin-arm64, darwin-x86_64

Result: supported_os = ["linux"]
```

**All musl fails (alpine on both architectures):**
```
Passed: 8 glibc environments + darwin-arm64, darwin-x86_64
Failed: linux-alpine-musl-x86_64, linux-alpine-musl-arm64

Result: supported_libc = ["glibc"]
```

**Specific family fails (e.g., arch has no bottle for this tool):**
```
Passed: all except arch
Failed: linux-arch-glibc-x86_64, linux-arch-glibc-arm64

Result: unsupported_platforms = ["linux/arch"]
```

**ARM64 fails across all families:**
```
Passed: all x86_64 linux + darwin environments
Failed: all arm64 linux environments

Result: unsupported_platforms = ["linux/arm64"]
```

The algorithm prefers broader constraints (`supported_os`) over fine-grained exclusions (`unsupported_platforms`) when the failure pattern aligns to a single dimension. When failures span multiple dimensions, it falls back to explicit per-platform exclusions. Family-level failures use the `unsupported_platforms` field with the `linux/{family}` format.

### Data Flow

```
generation job:
  → recipes/*.toml (new recipe files)
  → data/failures/<eco>.jsonl (generation failures)
  → artifact: passing-recipes (list of recipe paths)

platform jobs:
  ← artifact: passing-recipes
  → artifact: validation-results-<platform>.json

merge job:
  ← artifact: passing-recipes
  ← artifact: validation-results-* (all platforms)
  → recipes/*.toml (with platform constraints added)
  → data/failures/<eco>.jsonl (platform failures appended)
  → data/priority-queue.json (status updates)
  → PR
```

## Implementation Approach

### Phase 1: Platform Validation Jobs (#1254)

Add the four platform validation jobs to `batch-generate.yml`:
- Conditional execution based on skip flags
- Retry logic for network errors
- Structured JSON result artifacts
- Job summaries in `$GITHUB_STEP_SUMMARY`

Dependencies: #1252 (preflight job provides recipe list)

### Phase 2: Merge Job with Constraint Writing (#1256)

Add the merge job that:
- Aggregates platform results
- Derives and writes platform constraints
- Creates the PR with accurate metadata

Dependencies: #1254 (platform validation results)

### Phase 3: Validation Coverage for Batch PRs

After the batch PR is created, `test-changed-recipes.yml` still triggers as a secondary validation layer. This provides defense-in-depth: the batch workflow catches platform issues and writes constraints, and PR CI validates that the constrained recipes install correctly on the platforms they claim to support.

No code changes needed for this phase; it's the existing behavior once the PR is created with proper constraints.

## Security Considerations

### Download Verification

Platform validation jobs run `tsuku install`, which downloads binaries from upstream sources (Homebrew bottles, GitHub releases, etc.). These downloads use the same checksum verification as normal installs: the recipe specifies expected checksums, and `tsuku install` verifies them after download.

The batch pipeline doesn't introduce new download sources or bypass existing verification. Platform jobs validate on real platforms, so any checksum mismatches surface as install failures.

### Execution Isolation

Platform validation runs on ephemeral GitHub Actions runners. Each job gets a fresh VM that's destroyed after the run. Installed binaries execute in the runner's user context with no elevated privileges.

The `run_command` security gate in the merge job prevents recipes with arbitrary command execution from being auto-merged. These require manual review.

### Supply Chain Risks

The platform validation jobs don't change the supply chain model. Recipes still download from the same upstream sources (GHCR for Homebrew bottles, GitHub releases, etc.). The validation adds defense: if an upstream binary is malformed or the wrong architecture for a platform, validation catches it before the recipe ships to users.

One risk specific to this design: the platform validation results determine which platforms a recipe claims to support. If a validation runner is compromised, it could report false passes, causing a broken recipe to ship. This is mitigated by the ephemeral runner model and by `test-changed-recipes.yml` running as a secondary check on the PR.

### Resource Exhaustion

A malicious or pathological recipe could consume excessive CI time (large downloads, slow extraction, infinite loops in post-install). Per-recipe timeouts (5 minutes) and per-job timeouts (120 minutes) bound the blast radius. The circuit breaker (#1255) will halt batch runs if failure rates spike, preventing runaway cost.

### User Data Exposure

Platform validation doesn't access or transmit user data. It runs in CI on synthetic environments. The only data produced is pass/fail results per recipe per platform, which are committed to the public repository as failure JSONL and reflected in recipe metadata.

## Consequences

### Positive

- Users on all supported platforms (macOS, ARM64, musl, and every linux family) get recipes that are proven to work on their platform
- Partial-coverage recipes have accurate metadata, so `tsuku install` can give a clear "not supported on this platform" error instead of a cryptic download failure
- Platform failures are caught before merge, not discovered by users
- Failure JSONL provides data for analyzing which ecosystems have platform-specific issues

### Negative

- Batch workflow wall-clock time increases (platform validation runs in parallel but adds ~10 minutes for macOS)
- macOS CI budget consumed by validation (~110 minutes per 25-recipe batch across 2 macOS jobs)
- Linux jobs take longer due to sequential family container testing (5 families per recipe per runner)
- Two multi-platform validation systems to maintain (`batch-generate.yml` and `test-changed-recipes.yml`)

### Mitigations

- Progressive validation minimizes macOS cost (only recipes that pass Linux get promoted)
- Skip flags allow disabling expensive platforms for budget-constrained runs
- `test-changed-recipes.yml` on the PR provides defense-in-depth without additional cost (it runs anyway)
- The two validation systems serve different purposes: batch produces structured per-platform results for constraint writing; PR CI validates the final constrained recipes

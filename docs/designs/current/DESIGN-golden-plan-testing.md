---
status: Current
problem: Recipe changes and code updates can cause silent regressions in plan generation, and integration tests are slow and non-deterministic due to network dependencies on external services.
decision: Implement golden plan testing by generating and validating deterministic installation plans for every recipe across all supported platforms, with CI workflows to detect and enforce updates.
rationale: Golden plans provide complete regression coverage, enable meaningful code review through visible diffs, and eliminate silent regressions while being faster and more reliable than network-dependent integration tests.
---

# Golden Plan Testing

## Status

Current

## Context and Problem Statement

Tsuku's original test suite relied on integration tests that downloaded files from external services (GitHub releases, npm, PyPI, crates.io, RubyGems, Homebrew bottles), performed actual installations, and verified results. This approach was non-deterministic (network failures, rate limiting), slow (builder tests took 30+ minutes), and offered limited platform coverage. Recipe changes could silently alter plan generation with no way to preview the effect across platforms.

The `tsuku eval` command supports generating deterministic installation plans from local recipe files (`--recipe`) with cross-platform targeting (`--os`, `--arch`, `--linux-family`). This made it possible to build a golden file system that validates every recipe across all supported platforms on every PR.

## Decision Drivers

- Every recipe should be validated, not just a representative sample
- Recipe and code changes should produce visible diffs in golden files for code review
- Changed golden plans should be proven executable before merge
- Generation must be reproducible across environments
- The system should be self-maintaining as recipes evolve

## Considered Options

The main choice was between selective golden files (3-5 reference recipes), full golden files (all recipes), and hash-only validation (storing checksums instead of full plans). Selective coverage left most recipes without regression protection. Hash-only validation provided no visibility into what changed, making debugging regressions harder. Full golden files were chosen because they catch regressions in any recipe and make plan changes visible in PR diffs, at the cost of ~600 golden files and 50-100 MB of storage.

For Linux family support, the choice was between always generating all 5 family variants (wasteful for most recipes), auto-detecting differences by comparing generated plans (duplicates knowledge in the recipe), manual metadata declaration (error-prone), or deriving family awareness from recipe metadata via static analysis. Metadata derivation was chosen because it's automatically correct, requires no manual maintenance, and the knowledge is useful beyond golden files.

## Decision Outcome

Every recipe has golden plans generated for all supported platforms. Golden files are stored in the repository, validated by CI on every PR, and proven executable when they change. Linux family support uses metadata-driven generation where family-aware recipes get separate files per family, while family-agnostic recipes get a single Linux file.

### Trade-offs Accepted

- Large file count (~600 golden files for all recipes x platforms)
- Storage overhead of ~50-100 MB in the repository
- Regeneration requires network access for checksum computation
- linux-arm64 is excluded from golden file generation and validation because GitHub Actions doesn't provide arm64 Linux runners in the standard tier

## Solution Architecture

### Directory Structure

Golden files are organized by category. Embedded recipes (those shipped with the tsuku binary) use a flat structure under `testdata/golden/plans/embedded/<recipe>/`. Registry recipes use first-letter subdirectories mirroring the recipe registry: `testdata/golden/plans/<letter>/<recipe>/`. Currently only embedded recipes have golden files; registry recipe coverage is being expanded.

File naming follows the pattern `{version}-{os}-{arch}.json` for family-agnostic recipes and `{version}-{os}-{family}-{arch}.json` for family-aware recipes. The presence of a family component in the filename signals that the recipe produces different plans per Linux distribution.

### Tooling

Plan generation uses `tsuku eval --recipe <path> --os <os> --arch <arch> --version <ver>` (`cmd/tsuku/eval.go`). This command generates a deterministic plan for any target platform regardless of the host OS. For family-aware recipes, `--linux-family <family>` simulates a specific distribution. The `tsuku info --recipe <path> --metadata-only --json` command (`cmd/tsuku/info.go`) exposes the `supported_platforms` array that drives generation and validation scripts.

Five shell scripts manage golden files:

- `scripts/regenerate-golden.sh` regenerates golden files for a single recipe. It queries metadata for supported platforms, applies optional `--os`, `--arch`, `--version` filters, and cleans up files for platforms no longer supported. It auto-detects whether a recipe is embedded or registry-based.
- `scripts/regenerate-all-golden.sh` iterates over all recipes and calls regenerate-golden.sh for each.
- `scripts/validate-golden.sh` validates golden files for a single recipe by regenerating to a temp directory and comparing via SHA256 hash, showing a diff on mismatch.
- `scripts/validate-all-golden.sh` runs validation for every recipe with golden files.
- `scripts/validate-golden-exclusions.sh` validates the exclusion list and checks for stale entries.

Comparison is handled entirely through shell scripts using hash-based comparison with diff output on mismatch. No Go-level comparison utility was implemented; the shell approach is sufficient and keeps the comparison logic close to CI.

### CI Workflows

Five CI workflows enforce golden file correctness:

- `validate-golden-recipes.yml` triggers when recipe TOML files change. It regenerates golden files for changed recipes and fails if they don't match committed files.
- `validate-golden-code.yml` triggers when plan generation code changes (`internal/executor/`, `internal/actions/`, etc.). It regenerates all golden files and fails on any mismatch.
- `validate-golden-execution.yml` triggers when golden files change in a PR. It runs `tsuku install --plan <file> --force` on matching platform runners to prove plans are executable. Runs on ubuntu-latest (linux-amd64) and macos-latest (darwin-arm64). darwin-amd64 requires paid runners and is excluded; linux-arm64 has no available runners.
- `generate-golden-files.yml` is a workflow_dispatch action that generates golden files on CI runners across platforms and optionally commits them back to the branch.
- `publish-golden-to-r2.yml` publishes golden files to R2 storage for caching.

Execution validation runs directly on GitHub Actions runners rather than in sandbox containers. The runners are ephemeral, the goal is to validate plans work (not test isolation), and direct execution supports all plan types including ecosystem installers that need network access.

### Version Pinning

Recipes are versionless by design, but plans are version-specific (they contain resolved URLs and checksums). Golden files pin specific versions to provide determinism. Multiple versions of the same recipe can have golden files, which proves versionlessness: a recipe change that breaks plan generation for v0.44.0 but works for v0.46.0 is a regression.

The regeneration script preserves all existing versions in a directory. When a recipe changes, all versions are regenerated. New versions are added with `--version <ver>`. There's no automated version bump workflow yet; version updates are manual.

### Family-Aware Golden Files

Some recipes produce different plans depending on the Linux distribution family (debian, rhel, arch, alpine, suse). A recipe that uses `apt_install` only runs on Debian-family systems and produces a different plan than one using `dnf_install` for RHEL. Recipes that use `{{linux_family}}` interpolation in parameters produce different output for every family.

The system determines family awareness through static recipe analysis at load time rather than runtime detection. Each step has a pre-computed `StepAnalysis` (`internal/recipe/types.go`, line 650) containing a `Constraint` (where the step can run) and a `FamilyVarying` flag (whether output differs by family). The `Step.Analysis()` method (line 336) returns this pre-computed result; it's never nil, guaranteed by the constructor.

The `Constraint` struct (`internal/recipe/types.go`, line 549) has OS, Arch, and LinuxFamily fields. Actions like `apt_install` carry implicit constraints (Debian only), while explicit `when` clauses in recipes add further constraints. When both exist, they're merged with conflict detection at load time. If an `apt_install` step has `when.linux_family = "rhel"`, that's a conflict caught during recipe loading.

Recipe-level analysis aggregates step constraints into a `RecipeFamilyPolicy` (`internal/recipe/policy.go`). The five policies are:

- **FamilyNone**: No Linux-applicable steps (darwin-only recipe)
- **FamilyAgnostic**: Has Linux steps but nothing family-specific (most recipes)
- **FamilyVarying**: At least one step uses `{{linux_family}}` interpolation
- **FamilySpecific**: All Linux steps target specific families (e.g., apt_install + dnf_install)
- **FamilyMixed**: Has both family-constrained and unconstrained Linux steps

`AnalyzeRecipe()` (line 72 in policy.go) computes this policy, and `SupportedPlatforms()` (line 150) derives the platform list. Family-agnostic recipes get generic Linux platforms without a family qualifier. Family-varying and mixed recipes expand to all 5 families. Family-specific recipes list only the targeted families.

The `WhenClause` struct (`internal/recipe/types.go`, line 238) includes `LinuxFamily` and `Arch` fields for explicit constraints on non-package-manager actions. A `Matchable` interface (line 191) provides a uniform way to match steps against platform targets with OS(), Arch(), LinuxFamily(), and Libc() accessors.

Variable interpolation scanning (`detectInterpolatedVars()`, line 665 in types.go) is action-agnostic: it walks all string fields in step parameters looking for `{{linux_family}}`, `{{os}}`, and `{{arch}}` patterns. Any action using `{{linux_family}}` in any parameter becomes family-varying.

### Cross-Platform Generation

Plan generation for a target platform different from the current runtime works for download-based recipes. Build actions (cargo_build, go_build, cmake_build) require native toolchains and can't be cross-generated. Ecosystem install actions (npm_install, pipx_install) have partial support since lockfiles may be platform-agnostic but dependency resolution can differ. System actions (require_system) are excluded entirely.

CI generates golden files on ubuntu-latest, which provides consistency for Linux plans. Darwin plans are generated on macos runners via the generate-golden-files.yml workflow.

### Plan Format

Plans use format version 3 (`PlanFormatVersion = 3` in `internal/executor/plan.go`, line 18). Version 1 was the original format with composite actions. Version 2 decomposed composite actions. Version 3 added nested dependency support. The `Platform` struct in the plan includes OS, Arch, and LinuxFamily (omitempty for backward compatibility).

## Implementation Approach

The implementation required two sets of changes: the golden file infrastructure (eval improvements, scripts, CI workflows) and the family support layer (step analysis, recipe policy, metadata exposure).

The golden file infrastructure extended `tsuku eval` with `--recipe`, `--os`, `--arch`, `--version` flags for deterministic cross-platform plan generation, and added `--linux-family` for family simulation. Shell scripts wrap these commands for single-recipe and batch operations. CI workflows detect recipe vs code changes and trigger appropriate validation.

The family support layer added pre-computed step analysis at load time. Each step gets a `StepAnalysis` during construction via `NewStep()`, which merges implicit action constraints with explicit when clauses and scans for interpolation variables. Recipe-level `AnalyzeRecipe()` aggregates step analyses into a `RecipeFamilyPolicy`, and `SupportedPlatforms()` derives the platform list. The `tsuku info --metadata-only` command exposes this metadata to scripts and workflows, making it the single source of truth for platform support.

## Security Considerations

### Download Verification

Golden files contain checksums computed from real downloads during generation. When a golden file changes, execution validation runs `tsuku install --plan --force` which verifies checksums match actual artifacts. Initial golden file generation occurs in CI, not on developer machines, to prevent checksum poisoning from compromised workstations.

### Execution Isolation

Execution validation runs directly on ephemeral GitHub Actions runners. This is appropriate because the goal is validating plan correctness, not security isolation. Direct execution supports all plan types including ecosystem installers that need network access, which sandbox mode with network isolation can't handle.

### Supply Chain Risks

Golden files act as cryptographic snapshots of expected artifacts. Checksum changes are visible in PR diffs and require reviewer approval. Version pinning prevents "latest" from silently changing content. If an upstream artifact is republished with different content, CI fails with a checksum mismatch, forcing investigation.

Network-enabled sandbox tests (`RequiresNetwork=true`) give containers full network access via `--network=host`. This is necessary for ecosystem builds but means a compromised dependency could exfiltrate data. Tsuku doesn't verify GPG/Sigstore signatures; it relies on HTTPS + SHA256 checksums.

### User Data Exposure

Golden plan tests don't access user data. All operations are confined to reading recipe files and writing JSON to the testdata directory.

## Consequences

### Positive

- Every recipe has validated golden files, eliminating silent regressions
- Recipe changes produce visible diffs in PRs showing exactly how plans change
- Changed golden files must pass execution validation before merge
- Cross-platform plan generation works from any single runner
- Family-aware recipes get separate golden files per distribution, while family-agnostic recipes avoid unnecessary duplication
- Step analysis is pre-computed at load time, so callers don't need registry access

### Negative

- ~600 golden files to maintain in version control, ~50-100 MB storage
- Regeneration requires network access for downloading artifacts and computing checksums
- Two platforms excluded from execution validation (darwin-amd64 requires paid runners, linux-arm64 has no runners)
- No automated version bump workflow yet; version updates are manual
- No Go-level comparison utility; all comparison is shell-based

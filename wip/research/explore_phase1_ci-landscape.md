# CI Landscape Analysis

## Inventory Summary

- 53 workflow files in `.github/workflows/`
- ~76 jobs triggered per typical PR (touching Go code + recipes)
- 4 genuine runner types: linux-x86_64, linux-arm64, darwin-arm64, darwin-x86_64

## Per-Workflow Job Counts (PR-Triggered)

| Workflow | Jobs | Matrix Dimensions | Consolidation Pattern |
|----------|------|--------------------|-----------------------|
| test.yml | ~18 | integration-linux: 9 tools (1 job each) | Could serialize per runner |
| build-essentials.yml | ~23 | sandbox-multifamily: 5 families x 2 tools; homebrew-linux: 4 tools | Families could containerize |
| integration-tests.yml | ~20 | checksum-pinning: 5 families; homebrew-linux: 4 families; dlopen: 3 libs x 3 variants | Families could containerize |
| test-recipe.yml | ~5 | 4 arch/OS splits, Linux families containerized in 1 job | Already optimal |
| validation workflows | ~10 | Mostly 1 job each | Already minimal |

## Key Patterns

### Good Pattern (test-recipe.yml)
- 1 linux-x86_64 job runs all 5 families via Docker containers sequentially
- 1 linux-arm64 job runs 4 families via Docker containers
- 1 darwin-arm64 job, 1 darwin-x86_64 job (native, no containers)
- Total: 5 jobs (build + 4 platform)

### Good Pattern (build-essentials.yml macOS)
- Aggregated macOS Apple Silicon: 1 job tests all tools sequentially with GHA groups
- Aggregated macOS Intel: 1 job tests all tools sequentially with GHA groups

### Bad Pattern (integration-tests.yml)
- checksum-pinning: 5 separate jobs for 5 Linux families, all on ubuntu-latest
- homebrew-linux: 4 separate jobs for 4 families, all on ubuntu-latest
- Each job does: checkout -> setup-go -> build -> run test in container
- Redundant setup across all 9 jobs

### Bad Pattern (build-essentials.yml sandbox)
- test-sandbox-multifamily: 5 families x 2 tools = 10 separate jobs on ubuntu-latest
- Each job does identical Go setup, tsuku build, then runs one containerized test

### Bad Pattern (test.yml integration-linux)
- 9 separate jobs, each installing one tool on ubuntu-latest
- Each job: checkout -> setup-go -> build -> install one tool
- Could be serialized with GHA groups in 1 runner

## Consolidation Opportunities

### Immediate (Family Containerization)
| Source | Current | After | Savings |
|--------|---------|-------|---------|
| checksum-pinning | 5 jobs | 1 job | 4 |
| homebrew-linux | 4 jobs | 1 job | 3 |
| sandbox-multifamily | 10 jobs | 2 jobs (1 per tool) | 8 |
| homebrew-linux (build-ess) | 4 jobs | 1 job | 3 |

### Medium (Test Serialization)
| Source | Current | After | Savings |
|--------|---------|-------|---------|
| integration-linux (test.yml) | 9 jobs | 1 job | 8 |
| library-dlopen-glibc | 3 jobs | 1 job | 2 |
| library-dlopen-macos | 3 jobs | 1 job | 2 |

### Total Potential Savings
- Immediate: 18 fewer jobs per PR
- Medium: 12 fewer jobs per PR
- Combined: 30 fewer jobs per PR (76 -> 46, ~40% reduction)

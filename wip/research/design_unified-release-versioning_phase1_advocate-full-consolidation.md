# Advocate: Full Consolidation

## Approach Description
Address all dimensions in a single coordinated design: merge pipelines, standardize naming, add version pinning, update recipes, and add gRPC version handshake. The implementation ships as a coordinated set of changes that land together (or in rapid succession within a single release cycle).

Key elements:
- Merge `llm-release.yml` into `release.yml` with GPU build jobs
- Standardize all artifact names to `{tool}-{os}-{arch}[-{backend}]` (no version in filename)
- Add `pinnedLlmVersion` compile-time pinning (extending dltest pattern)
- Update both recipes with `[version]` sections and new asset patterns
- Add `addon_version` field to gRPC StatusResponse proto
- Delete `llm-release.yml`

## Investigation

### Pipeline Consolidation
- `release.yml` currently has 5 jobs: release (GoReleaser), build-rust (4 platforms), build-rust-musl (2 platforms), integration-test, finalize-release
- `llm-release.yml` has 3 jobs: build-llm (6 platform matrix), integration-test, create-release + finalize-release
- Merged pipeline adds `build-llm` as a 6th job, runs in parallel after `release` job
- GPU setup (CUDA 12.4, Vulkan SDK) happens within matrix job steps
- `finalize-release` expands from 10 to 16+ expected artifacts
- Integration-test extends to validate all 3 binaries match expected version

### Naming Standardization
- `.goreleaser.yaml` `name_template`: change from `{{ .ProjectName }}-{{ .Os }}-{{ .Arch }}` (which GoReleaser also appends version to) to a clean `tsuku-{{ .Os }}-{{ .Arch }}`
- llm build output: change from `tsuku-llm-v${VERSION}-{platform}` to `tsuku-llm-{platform}`
- dltest: already uses `tsuku-dltest-{os}-{arch}` -- no change needed
- Convention: `{tool}-{os}-{arch}[-{backend}]` where backend is cuda/vulkan (Linux only, macOS Metal is implicit)
- All recipe asset patterns update simultaneously

### Version Pinning
- Add `pinnedLlmVersion` to `internal/verify/version.go` alongside existing `pinnedDltestVersion`
- `.goreleaser.yaml` ldflags: add `-X github.com/tsukumogami/tsuku/internal/verify.pinnedLlmVersion={{.Version}}`
- `internal/llm/addon/manager.go`: version check in `findInstalledBinary` or `EnsureAddon`, auto-reinstall on mismatch
- Dev mode: accept any version. Release mode: exact match required.

### gRPC Handshake
- `proto/llm.proto` StatusResponse: add `string addon_version = 6;`
- Regenerate Go proto files
- `internal/llm/local.go`: add `EnsureVersionCompatible()` method
- tsuku-llm binary: inject version at build time, return in GetStatus response
- Provides runtime visibility into version mismatches (complementary to compile-time pinning)

### Recipe Updates
- `recipes/t/tsuku-dltest.toml`: add `github_repo = "tsukumogami/tsuku"` to [version] section
- `recipes/t/tsuku-llm.toml`: add full `[version]` section, change all `repo` fields from `tsukumogami/tsuku-llm` to `tsukumogami/tsuku`, update asset patterns to remove `v{version}-` prefix

## Strengths
- **Atomic transition**: The system moves from old state to new state in one step. No intermediate inconsistency period. Users never see "version pinning exists but naming is still old."
- **Single design document**: One cohesive architecture document covers all decisions. Reviewers see the full picture and can evaluate trade-offs between dimensions (e.g., naming choice affects recipe patterns affects pipeline verification).
- **Naming and pipeline are naturally coupled**: Changing artifact names requires updating both the pipeline AND the recipes simultaneously. Doing these separately requires a compatibility shim or two coordinated releases.
- **Complete solution**: Addresses all 4 problems (lockstep, naming, resolution, constraints) plus adds runtime visibility. No follow-up work needed.
- **Fewer release cycles**: Ships in 1-2 release cycles instead of 4+.

## Weaknesses
- **Large PR or tightly-coupled PR series**: Even if split into multiple PRs, they must land in the same release cycle. Review burden is high.
- **Higher rollback cost**: If something breaks, reverting touches many files across multiple subsystems. Partial revert is harder than reverting one focused PR.
- **All-or-nothing testing**: Can't validate pipeline merge independently before adding naming changes. Must test the full set together.
- **Proto evolution adds scope**: The gRPC handshake could be deferred without losing core safety (compile-time pinning is sufficient). Including it adds proto regeneration and tsuku-llm build coordination.

## Deal-Breaker Risks
- **Cross-repo coordination**: The gRPC handshake requires changes in both tsuku (client) and tsuku-llm (server). If tsuku-llm is built from source in this repo, this is fine. If it's a separate repo, coordination becomes complex.
- **GoReleaser naming change is irreversible per release**: Once a release goes out with new naming, existing recipe versions pointing to old naming patterns break. Must coordinate recipe updates to land in the same release.
- Neither is a true deal-breaker -- both are manageable with careful execution.

## Implementation Complexity
- Files to modify: ~15-20 (pipeline, recipes, Go code, proto, goreleaser config)
- New infrastructure: Proto regeneration tooling (if not already set up)
- Estimated scope: Large (spans CI, Go, proto, TOML, and yaml)
- PRs: 1-3 tightly-coupled PRs in a single release cycle

## Summary
Full consolidation delivers the complete solution in one coordinated effort, eliminating intermediate inconsistency and ensuring all dimensions fit together. Its strength is atomic transition and a single coherent design. Its weakness is higher review burden and rollback cost. Best suited when the team can dedicate focused attention to one large change, and when the coupling between dimensions (naming affects recipes affects pipeline) makes isolated changes awkward.

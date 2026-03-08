# Advocate: Incremental Migration

## Approach Description
Phase the unified release work into 4 independently-deployable steps, each validated before the next begins:

1. **Recipe fixes** -- Add `[version]` sections to tsuku-dltest and tsuku-llm recipes pointing to `tsukumogami/tsuku` with `tag_prefix = "v"`. Update llm recipe's `repo` fields from `tsukumogami/tsuku-llm` to `tsukumogami/tsuku`. Update asset patterns to match current release artifacts.
2. **Compile-time llm pinning** -- Add `pinnedLlmVersion` to `internal/verify/version.go`, inject via `.goreleaser.yaml` ldflags, enforce in `addon/manager.go` with auto-reinstall on mismatch.
3. **Pipeline merge** -- Move `llm-release.yml` build-llm jobs into `release.yml`. Add GPU dependency steps. Extend finalize-release to verify 16+ artifacts. Delete `llm-release.yml`.
4. **Naming standardization** -- Update GoReleaser `name_template` to remove version from CLI filenames. Update llm build to remove version from artifact names. Update all recipe asset patterns. Convention: `{tool}-{os}-{arch}[-{backend}]`.

An optional Phase 5 adds `addon_version` to the gRPC StatusResponse proto for runtime version visibility.

## Investigation

### Phase 1: Recipe fixes (standalone PR)
- `recipes/t/tsuku-dltest.toml`: Already has `tag_prefix = "v"` and `repo = "tsukumogami/tsuku"` in steps. Just needs `github_repo = "tsukumogami/tsuku"` in `[version]` section to promote from InferredGitHubStrategy (priority 10) to GitHubRepoStrategy (priority 90).
- `recipes/t/tsuku-llm.toml`: Needs entire `[version]` section added, plus all step `repo` fields changed from `tsukumogami/tsuku-llm` to `tsukumogami/tsuku`. Asset patterns must also change to match whatever names the pipeline currently produces.
- This phase is blocked until llm artifacts exist under `v*` tags. If the separate `tsuku-llm-v*` tags have never been pushed, the recipe can't resolve versions from either source today. This phase works only after Phase 3 (pipeline merge) produces llm artifacts under `v*` tags.

**Reordering insight**: Recipe fixes for llm can't land before the pipeline produces artifacts under `v*` tags. The actual order might need to be: recipe fix for dltest first (works today), then pipeline merge, then recipe fix for llm, then naming.

### Phase 2: Compile-time llm pinning (standalone PR)
- Follow exact dltest pattern in `internal/verify/version.go` (add `pinnedLlmVersion` variable)
- `.goreleaser.yaml` line 24 adds another `-X` ldflags entry
- `internal/llm/addon/manager.go`: Add version check in `findInstalledBinary` or `EnsureAddon`
- Dev mode (`"dev"`) accepts any version; release mode requires exact match
- Auto-reinstall uses existing recipe installation path
- This is fully independent of pipeline and naming changes

### Phase 3: Pipeline merge (standalone PR)
- Copy `llm-release.yml` build-llm matrix job into `release.yml`
- Add GPU setup steps (CUDA toolkit, Vulkan SDK, protobuf)
- Add build-llm as a new parallel job dependent on `release` job
- Extend `integration-test` to include llm binary validation
- Extend `finalize-release` expected artifacts from 10 to 16+
- Delete `llm-release.yml`
- ~25-30 min added for longest llm build, but runs in parallel with existing jobs

### Phase 4: Naming standardization (standalone PR)
- `.goreleaser.yaml`: Change `name_template` to `tsuku-{{ .Os }}-{{ .Arch }}`
- llm build steps: Change output names from `tsuku-llm-v${VERSION}-{platform}` to `tsuku-llm-{platform}`
- Update `finalize-release` expected artifact list
- Update both recipes' asset patterns
- Update `checksums.txt` generation

## Strengths
- **Smallest blast radius per change**: Each PR touches one subsystem. A broken pipeline merge doesn't block version pinning work.
- **Easier to review**: Reviewers can focus on one concern per PR instead of understanding all dimensions simultaneously.
- **Rollback granularity**: If naming standardization causes issues, revert one PR without affecting version pinning or pipeline changes.
- **Parallel development**: Different phases can be worked on concurrently once dependencies allow. Recipe fixes and version pinning have no code overlap.
- **Progressive validation**: Each phase can be tested in a release before moving to the next. Version pinning gets validated in a real release before pipeline merge adds complexity.

## Weaknesses
- **Ordering constraints reduce independence**: Recipe fixes for llm depend on pipeline merge (llm artifacts must exist under `v*` tags first). The stated "independent" phases aren't fully independent.
- **Longer calendar time**: 4+ separate PRs, each needing review and a release cycle to validate. Could take weeks instead of days.
- **Intermediate inconsistency**: Between phases, the system is in a mixed state (e.g., version pinning exists but naming is still inconsistent). This inconsistency is harmless but adds cognitive load.
- **More total CI cost**: Each PR runs full CI independently. The naming change requires re-testing everything that was already tested.

## Deal-Breaker Risks
- **Phase ordering dependency**: If the llm recipe fix truly requires pipeline merge first, the "incremental" framing is partially misleading -- phases 1 and 3 are coupled. However, the dltest recipe fix IS independent, so partial incrementalism works.
- None that would make this approach fail entirely. The ordering constraint is a sequencing problem, not a feasibility problem.

## Implementation Complexity
- Files to modify: ~15 across all phases (same as other approaches, just spread across PRs)
- New infrastructure: No
- Estimated scope: Medium (each phase is small, but there are 4 of them)
- PRs: 4-5 separate PRs over multiple release cycles

## Summary
Incremental migration reduces risk per change by isolating each dimension into its own PR. The approach's main strength is rollback granularity and focused review. Its main weakness is that phases aren't fully independent -- llm recipe fixes depend on pipeline merge -- so the true order has more coupling than the clean 4-phase framing suggests. Best suited when the team values safety over speed, or when different contributors work on different phases.

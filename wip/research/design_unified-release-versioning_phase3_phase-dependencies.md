# Phase 3 Research: Phase Dependencies

## Questions Investigated
- Which phases truly block each other?
- Can any phases run in parallel?
- Does the llm recipe fix depend on pipeline merge?

## Findings

### Current v0.5.0 Release Assets
```
tsuku-darwin-amd64_0.5.0_darwin_amd64
tsuku-darwin-arm64_0.5.0_darwin_arm64
tsuku-linux-amd64_0.5.0_linux_amd64
tsuku-linux-arm64_0.5.0_linux_arm64
tsuku-dltest-darwin-amd64
tsuku-dltest-darwin-arm64
tsuku-dltest-linux-amd64
tsuku-dltest-linux-amd64-musl
tsuku-dltest-linux-arm64
tsuku-dltest-linux-arm64-musl
checksums.txt
```

No llm artifacts exist under `v*` tags. The llm recipe currently points to `tsukumogami/tsuku-llm` repo which has no releases either.

### Phase Analysis

**Phase 1: dltest recipe fix** -- Add `github_repo = "tsukumogami/tsuku"` to [version] section.
- Depends on: Nothing. dltest artifacts already exist under `v*` tags.
- Can land immediately.

**Phase 2: llm version pinning** -- Add `pinnedLlmVersion` to Go code.
- Depends on: Nothing in the pipeline. This is pure Go code change + goreleaser ldflags.
- The pinning variable defaults to `"dev"` so it's a no-op until a release build.
- Can land immediately, in parallel with Phase 1.

**Phase 3: pipeline merge** -- Move llm builds into release.yml.
- Depends on: Nothing from Phase 1 or 2. It's a CI workflow change.
- CREATES the prerequisite for Phase 4 (llm artifacts under `v*` tags).
- Can land in parallel with Phase 1 and 2, but best to validate separately.

**Phase 4: llm recipe fix** -- Update recipe to resolve from main repo.
- Depends on: Phase 3 (pipeline merge). Without it, there are no llm artifacts under `v*` tags. Changing the recipe to point at `tsukumogami/tsuku` would resolve versions but find no matching assets.
- BLOCKED until Phase 3 produces at least one release with llm artifacts.

**Phase 5: naming standardization** -- Remove version from artifact filenames.
- Depends on: Phase 3 (the pipeline must be consolidated first so naming changes are applied in one place).
- Depends on: Phase 4 (llm recipe asset patterns must be updated simultaneously with the pipeline naming change).
- This phase is coupled with both pipeline and recipe changes.

**Phase 6: gRPC handshake** -- Add addon_version to StatusResponse.
- Depends on: Phase 2 (llm pinning must exist so the version field has meaning).
- Can land any time after Phase 2.

### True Dependency Graph

```
Phase 1 (dltest recipe) ─────────────────────────────────────────┐
                                                                  │
Phase 2 (llm pinning) ──────────────────────────────┐             │
                                                     │             │
Phase 3 (pipeline merge) ──→ Phase 4 (llm recipe) ──┼──→ Phase 5 (naming)
                                                     │
                                                     └──→ Phase 6 (gRPC)
```

Phases 1, 2, and 3 have NO dependencies on each other. They can all be developed and merged in parallel.

Phase 4 depends only on Phase 3.
Phase 5 depends on Phases 3 and 4.
Phase 6 depends only on Phase 2.

### Parallel Opportunities
- **Batch 1**: Phases 1, 2, 3 (all independent -- three PRs in parallel)
- **Batch 2**: Phases 4 and 6 (after their respective dependencies)
- **Batch 3**: Phase 5 (after Phase 4)

Minimum 3 release cycles if strict sequential validation. As few as 2 with parallelism in Batch 1.

### Original 5-Phase Proposal vs Actual Dependencies
The original proposal grouped llm recipe fix with naming standardization in Phase 4. The dependency analysis shows these are separable: llm recipe fix depends only on pipeline merge, while naming standardization depends on both pipeline merge AND llm recipe fix. Splitting them enables earlier delivery of the llm recipe fix.

## Implications for Design
The implementation phases should be reordered to maximize parallelism:
1. Batch 1: dltest recipe fix + llm pinning + pipeline merge (3 parallel PRs)
2. Batch 2: llm recipe fix + gRPC handshake (2 parallel PRs, after batch 1)
3. Batch 3: naming standardization (1 PR, after batch 2)

This delivers version safety (the core goal) in Batch 1 and completes all changes in 3 batches instead of 5 sequential phases.

## Surprises
Phases 1, 2, and 3 are fully independent -- they can all land in the same release with no ordering constraints between them. The original sequential framing suggested more coupling than actually exists.

## Summary
Three of the six phases (dltest recipe, llm pinning, pipeline merge) have zero dependencies on each other and can land in parallel. The llm recipe fix is blocked only by pipeline merge. Naming standardization is the final step, depending on both pipeline and recipe. Maximum parallelism reduces the timeline from 5-6 sequential releases to 3 batches.

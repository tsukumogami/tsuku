# Exploration Findings: unified-release-versioning

## Round 1 Findings

### Release Pipeline (Lead 1)
- release.yml builds CLI + dltest under `v*` tags, produces 10 artifacts
- llm-release.yml builds llm independently under `tsuku-llm-v*` tags, produces 6 artifacts + checksums
- No `tsuku-llm-v*` tags have ever been pushed; the workflow has never run
- Merging requires moving llm build jobs into release.yml and expanding finalize-release to verify 16+ artifacts

### Version Resolution (Lead 2)
- dltest recipe has `[version]` with `tag_prefix = "v"` and resolves from `tsukumogami/tsuku`
- llm recipe has NO `[version]` section; falls back to InferredGitHubStrategy from `tsukumogami/tsuku-llm`
- Fix: add `[version] github_repo = "tsukumogami/tsuku"` + `tag_prefix = "v"` to both recipes
- This promotes resolution from priority 10 (inferred) to priority 90 (explicit)

### Version Constraints (Lead 3)
- No version constraint mechanism exists in recipe format
- Dependencies are version-agnostic string arrays
- Compile-time embedding via ldflags is the simplest enforcement path
- dltest already uses this pattern (`pinnedDltestVersion`); llm has nothing

### Runtime Discovery (Lead 4)
- dltest: subprocess via exec.CommandContext, compile-time pinning with auto-reinstall on mismatch
- llm: gRPC daemon over Unix socket, accepts any installed version, no pinning
- dltest pattern is the model to extend to llm

### LLM Release Consumers (Lead 5)
- No consumers of `tsuku-llm-v*` tags exist
- No tags have ever been pushed
- Recipe points to separate `tsukumogami/tsuku-llm` repo
- Safe to retire immediately; the real work is moving builds and updating recipes

## Round 2 Findings

### Artifact Naming (Lead 6)
- Current naming is inconsistent: CLI embeds version (GoReleaser default `tsuku-{os}-{arch}_{version}_{os}_{arch}`), dltest omits version, llm embeds it differently (`tsuku-llm-v{version}-{platform}`)
- Recommended: standardize on no-version filenames (`{tool}-{os}-{arch}[-{backend}]`) since `github_file` resolves within a tagged release, making version in filename redundant
- Requires: updating GoReleaser name_template, llm asset names, and recipe asset_patterns
- Backend suffix convention: asymmetric by design (macOS omits backend since Metal is implicit; Linux includes cuda/vulkan)

### LLM Version Pinning (Lead 7)
- Phase 1: compile-time `pinnedLlmVersion` via ldflags (same as dltest), enforce in `addon/manager.go`
- Phase 2: add `addon_version` field to StatusResponse proto for gRPC handshake version checking
- Both are complementary: ldflags ensures correct version installed, gRPC gives runtime visibility
- Key difference from dltest: llm is a long-running daemon, so version mismatch can happen mid-session if daemon was started by a previous tsuku version

### GPU Build Impact (Lead 8)
- CUDA toolkit adds 5-8 min per build, Vulkan SDK adds 3-5 min, macOS Metal adds nothing
- All 6 llm builds run in parallel matrix; GPU setup overlaps with cargo compilation
- Merging into release.yml adds ~10 min to critical path (from cargo compile time, not GPU setup)
- No additional runner types needed beyond existing ubuntu-22.04 and ubuntu-24.04-arm
- CUDA 12.4 build version must stay compatible with cuda-runtime recipe (12.6.77 libraries)

## Cross-Cutting Insights

### The picture is complete
Eight leads across two rounds have covered every dimension of the unified release problem:
- Pipeline mechanics (how to merge workflows)
- Version resolution (how recipes find versions)
- Version enforcement (how to ensure lockstep at build time and runtime)
- Artifact naming (what to call the files)
- Runtime discovery (how binaries find each other)
- GPU complexity (what llm builds add to the pipeline)
- Consumer impact (who would break)

### Natural implementation sequence emerges
The findings suggest a clear phased approach:
1. Recipe changes (version sections, asset patterns) -- no pipeline changes needed
2. Compile-time version pinning for llm (extend dltest pattern)
3. Pipeline consolidation (merge llm-release.yml into release.yml)
4. Artifact naming standardization (GoReleaser config, remove version from filenames)
5. gRPC version handshake (follow-up improvement for runtime diagnostics)

### Key tensions
- **Artifact naming**: Lead 6 recommends removing version from ALL filenames. Lead 8's GPU analysis shows asymmetric backend suffixes work well. These are compatible -- the convention becomes `{tool}-{os}-{arch}[-{backend}]` with no version anywhere.
- **Version in llm filenames vs no version**: If we standardize on no-version filenames, the llm recipe's `asset_pattern = "tsuku-llm-v{version}-..."` must change to `asset_pattern = "tsuku-llm-..."`. This is a coordinated change with the pipeline.
- **Proto evolution**: Adding `addon_version` to StatusResponse is safe (proto3 optional fields) but requires coordinating tsuku CLI and tsuku-llm binary releases. This is easier once they share a release tag.

### No remaining gaps
All open questions from round 1 have been answered by round 2:
- Artifact naming convention? Standardize on no-version (`{tool}-{os}-{arch}[-{backend}]`)
- How to pin llm version? Compile-time ldflags + optional gRPC handshake
- GPU build impact? ~10 min added, fully parallelizable, no new infrastructure needed
- Consumers of separate llm tags? None exist

## Decision: Crystallize

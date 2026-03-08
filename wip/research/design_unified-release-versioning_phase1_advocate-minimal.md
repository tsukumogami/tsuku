# Advocate: Minimal -- Pinning + Recipe Fix

## Approach Description
Solve only the core safety problem (version mismatch risk) with the smallest possible change surface:

1. Fix recipe version resolution -- add `[version]` sections pointing both recipes to `tsukumogami/tsuku` with `tag_prefix = "v"`
2. Add compile-time llm version pinning -- extend the dltest pattern with `pinnedLlmVersion`
3. Trigger `llm-release.yml` from `v*` tags (not just `tsuku-llm-v*`) -- so both pipelines fire on the same tag

Do NOT: merge pipelines, change artifact naming, add gRPC handshake, or delete `llm-release.yml`.

## Investigation

### Recipe Fixes
- `recipes/t/tsuku-dltest.toml`: Add `github_repo = "tsukumogami/tsuku"` to existing `[version]` section. Promotes from InferredGitHubStrategy (priority 10) to GitHubRepoStrategy (priority 90) with `tag_prefix = "v"`. No other changes needed -- asset pattern and repo in steps already point to `tsukumogami/tsuku`.
- `recipes/t/tsuku-llm.toml`: Add `[version]` section with `github_repo = "tsukumogami/tsuku"` and `tag_prefix = "v"`. Change step `repo` fields from `tsukumogami/tsuku-llm` to `tsukumogami/tsuku`.
- **Asset pattern challenge**: Current llm asset pattern is `tsuku-llm-v{version}-{platform}`. If we keep current naming AND fire llm-release.yml from `v*` tags, the version extraction logic needs updating (`VERSION="${TAG#tsuku-llm-v}"` becomes `VERSION="${TAG#v}"`). The recipe's `{version}` placeholder would resolve to e.g. `0.5.0`, producing `tsuku-llm-v0.5.0-darwin-arm64` as the expected filename. This works IF llm-release.yml also produces that filename from the `v*` tag.

### Pipeline Trigger Change
- `llm-release.yml` currently triggers on `tsuku-llm-v*` tags only
- Change to: `on: push: tags: ['v*']`
- Version extraction changes from `VERSION="${GITHUB_REF_NAME#tsuku-llm-v}"` to `VERSION="${GITHUB_REF_NAME#v}"`
- Artifact naming stays: `tsuku-llm-v${VERSION}-{platform}` (embedding the version)
- Both `release.yml` and `llm-release.yml` fire on the same `v*` tag push
- Each creates its own GitHub release draft? No -- they'd conflict on the same tag. This is the key problem.

### The Release Conflict Problem
When two workflows trigger on the same tag and both try to create a GitHub release:
- `release.yml` finalize-release publishes the release
- `llm-release.yml` create-release tries to create its own release for the same tag
- One will fail because the release already exists

**Resolution options**:
a. `llm-release.yml` uploads to existing release (use `gh release upload` instead of creating new release)
b. Merge llm into release.yml (but that's the full consolidation approach)
c. Use a fan-out workflow: a thin dispatcher triggers both build workflows as reusable workflows

Option (a) is most minimal: keep `llm-release.yml` but change it to upload artifacts to the existing release created by `release.yml`. Remove its own create-release and finalize-release jobs. This makes it a "build and upload" workflow only.

### Version Pinning
- Add `pinnedLlmVersion` to `internal/verify/version.go`
- `.goreleaser.yaml`: add ldflags `-X ...pinnedLlmVersion={{.Version}}`
- `internal/llm/addon/manager.go`: version check, auto-reinstall
- Same pattern as dltest -- well-understood, low risk

## Strengths
- **Smallest change surface**: ~8 files modified. No naming changes, no pipeline restructuring, no proto evolution.
- **Solves the actual safety problem**: Version pinning eliminates the mismatch risk. This is the core issue that motivated the exploration.
- **No breaking changes**: Existing artifact naming stays the same. No recipe users are affected.
- **Quick to ship**: 1-2 small PRs, one release cycle to validate.
- **Low review burden**: Reviewer needs to understand recipe version resolution and the dltest pinning pattern. No CI workflow expertise required.
- **Reversible**: Pinning can be disabled by setting `pinnedLlmVersion = "dev"` without any other changes.

## Weaknesses
- **Two workflows remain**: `release.yml` and `llm-release.yml` both fire on `v*` tags. Race condition on release creation must be handled. This is solvable but adds fragility.
- **Naming stays inconsistent**: GoReleaser's `tsuku-{os}-{arch}_{version}_{os}_{arch}` duplication persists. llm's `v{version}` prefix in filenames persists. This is cosmetic but creates ongoing confusion.
- **No pipeline simplification**: Two workflow files to maintain, two finalize-release jobs to keep in sync.
- **Technical debt acknowledged but not addressed**: Naming inconsistency and dual pipelines become known debt that "we'll fix later" -- but later may never come.
- **gRPC visibility gap**: Without the `addon_version` field, version mismatch errors from the daemon show as opaque gRPC failures. The user sees "connection failed" instead of "version mismatch: expected 0.5.0 but daemon is 0.4.0."

## Deal-Breaker Risks
- **Release creation race condition**: Two workflows creating releases for the same tag can conflict. This is solvable (option a: llm uploads to existing release) but requires careful coordination between workflow timing. If `release.yml` hasn't created the release yet when `llm-release.yml` tries to upload, it fails.
- This is a significant integration risk but not a deal-breaker -- it can be solved with a `needs` dependency or retry logic. However, it does mean the "keep separate pipelines" approach isn't as clean as it sounds.

## Implementation Complexity
- Files to modify: ~8 (2 recipes, 2-3 Go files, 1 goreleaser yaml, 1-2 workflow files)
- New infrastructure: No
- Estimated scope: Small
- PRs: 1-2 focused PRs

## Summary
The minimal approach solves the core safety problem (version mismatch risk) with the fewest changes. Compile-time pinning and recipe fixes are well-understood patterns. The main weakness is the release creation race condition when two workflows fire on the same tag -- this is solvable but adds fragility that pipeline merge would eliminate. Best suited when the priority is shipping version safety fast, with naming and pipeline cleanup deferred to a follow-up effort.

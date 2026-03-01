# Test Plan: Sandbox Build Cache

Generated from: docs/designs/DESIGN-sandbox-build-cache.md
Issues covered: 6
Total scenarios: 16

---

## Scenario 1: BuildFromDockerfile compiles and passes existing tests

**ID**: scenario-1
**Testable after**: #1958
**Commands**:
- `go build ./...`
- `go test ./internal/validate/... -count=1`
**Expected**: Build succeeds with no errors. All existing validate tests pass, confirming the new `BuildFromDockerfile` method is implemented on `dockerRuntime`, `podmanRuntime`, and `mockRuntime` without breaking existing `Build()` usage.
**Status**: passed

**Validation Output**: See wip/research/implement-doc_validation_issue1958.md

---

## Scenario 2: BuildFromDockerfile reads Dockerfile from context directory (not stdin)

**ID**: scenario-2
**Testable after**: #1958
**Commands**:
- `go test ./internal/validate/... -run TestBuildFromDockerfile -v -count=1`
**Expected**: Unit tests confirm that `BuildFromDockerfile` runs `docker build -t <name> <contextDir>` (or `podman build -t <name> <contextDir>`) without `-f -` or stdin piping. The mock runtime records the correct arguments.
**Status**: passed

**Validation Output**: See wip/research/implement-doc_validation_issue1958.md - Implementation verified: both podman and docker runtimes pass contextDir as argument without stdin piping.

---

## Scenario 3: FlattenDependencies returns empty slice for plans with no dependencies

**ID**: scenario-3
**Testable after**: #1959
**Commands**:
- `go test ./internal/sandbox/ -run TestFlattenDependencies_Empty -v -count=1`
**Expected**: Returns `[]FlatDep{}` (non-nil empty slice). No panic on nil or empty `Dependencies`.
**Status**: passed

---

## Scenario 4: FlattenDependencies produces correct topological order with deduplication

**ID**: scenario-4
**Testable after**: #1959
**Commands**:
- `go test ./internal/sandbox/ -run 'TestFlattenDependencies_(LeavesFirst|AlphabeticalSiblings|Deduplication|PreservesSubtree|StripsTimestamp)' -v -count=1`
**Expected**: Leaves appear before parents. Siblings at the same depth are sorted alphabetically. Duplicate tools (same tool+version via different dependency paths) are deduplicated to first occurrence. Each converted plan retains its nested dependency subtree. `GeneratedAt` is zeroed in all output plans.
**Status**: passed

---

## Scenario 5: GenerateFoundationDockerfile produces valid Dockerfile structure

**ID**: scenario-5
**Testable after**: #1959
**Commands**:
- `go test ./internal/sandbox/ -run 'TestGenerateFoundationDockerfile' -v -count=1`
**Expected**: Dockerfile starts with `FROM <packageImage>`, includes `COPY tsuku /usr/local/bin/tsuku`, sets `TSUKU_HOME` and `PATH` env vars, has interleaved COPY+RUN pairs per dependency (e.g., `COPY plans/dep-00-rust.json /tmp/plans/dep-00-rust.json` then `RUN tsuku install --plan /tmp/plans/dep-00-rust.json --force`), ends with `RUN rm -rf /usr/local/bin/tsuku /tmp/plans`. Same inputs always produce identical output.
**Status**: passed

---

## Scenario 6: FoundationImageName produces deterministic content-based tags

**ID**: scenario-6
**Testable after**: #1959
**Commands**:
- `go test ./internal/sandbox/ -run 'TestFoundationImageName' -v -count=1`
**Expected**: Output matches pattern `tsuku/sandbox-foundation:{family}-{16 hex chars}`. Same family + same Dockerfile always produces the same tag. Different Dockerfiles produce different tags.
**Status**: passed

---

## Scenario 7: Targeted mounts replace broad /workspace mount

**ID**: scenario-7
**Testable after**: #1960
**Commands**:
- `go test ./internal/sandbox/ -run 'TestSandbox.*Mount' -v -count=1`
- `go test ./internal/sandbox/... -count=1`
**Expected**: `Executor.Sandbox()` constructs four targeted mounts: plan.json (read-only), sandbox.sh (read-only), download cache (read-only), output dir (read-write). No single broad `/workspace` mount exists. The tsuku binary mount is preserved separately. All existing sandbox tests pass.
**Status**: passed

**Validation Output**: See wip/research/implement-doc_validation_issue1960.md - Both targeted mount tests and full sandbox test suite (20.8s) passed. TestSandboxTargetedMounts and TestSandboxTargetedMounts_TsukuBinaryAppendedSeparately confirmed correct mount construction.

---

## Scenario 8: Sandbox script writes verification markers to /workspace/output/

**ID**: scenario-8
**Testable after**: #1960
**Commands**:
- `go test ./internal/sandbox/ -run 'TestBuildSandboxScript' -v -count=1`
**Expected**: `buildSandboxScript()` output writes verify markers to `/workspace/output/.sandbox-verify-output` and `/workspace/output/.sandbox-verify-exit` (not `/workspace/.sandbox-verify-*`). Uses conditional `mkdir -p` guarded by `[ ! -d /workspace/tsuku/tools ]`.
**Status**: passed

**Validation Output**: See wip/research/implement-doc_validation_issue1960.md - TestBuildSandboxScript_VerifyMarkersWriteToOutput confirmed markers write to /workspace/output/. TestBuildSandboxScript_ConditionalMkdir verified conditional mkdir behavior. All 8 test variants passed.

---

## Scenario 9: readVerifyResults reads from output directory

**ID**: scenario-9
**Testable after**: #1960
**Commands**:
- `go test ./internal/sandbox/ -run 'TestReadVerifyResults' -v -count=1`
**Expected**: `readVerifyResults` reads marker files from the output subdirectory path (not directly from `workspaceDir`). Existing verify result test cases continue to pass with updated paths.
**Status**: passed

**Validation Output**: See wip/research/implement-doc_validation_issue1960.md - TestReadVerifyResults_OutputDirectory confirmed correct output directory path handling. All 8 test variants passed including edge cases (missing markers, pattern matching, exit codes).

---

## Scenario 10: BuildFoundationImage skips build when image already exists

**ID**: scenario-10
**Testable after**: #1961
**Commands**:
- `go test ./internal/sandbox/ -run 'TestBuildFoundationImage.*Cached' -v -count=1`
**Expected**: When `runtime.ImageExists()` returns true, `BuildFoundationImage` returns the cached image name without calling `BuildFromDockerfile`. The mock runtime confirms `BuildFromDockerfile` was never invoked.
**Status**: passed

**Validation Output**: See wip/research/implement-doc_validation_issue1961.md - TestBuildFoundationImage_Cached confirms imageExistsCalls==1 and buildFromDockerfileCalls==0. Complementary TestBuildFoundationImage_NotCached also passes.

---

## Scenario 11: Sandbox skips foundation image build when plan has no dependencies

**ID**: scenario-11
**Testable after**: #1961
**Commands**:
- `go test ./internal/sandbox/ -run 'TestSandbox.*NoDep' -v -count=1`
**Expected**: When `plan.Dependencies` is empty, `Executor.Sandbox()` uses the package image directly. `BuildFromDockerfile` is never called on the mock runtime.
**Status**: passed

**Validation Output**: See wip/research/implement-doc_validation_issue1961.md - TestSandbox_NoDep_SkipsFoundation and TestSandbox_NoDep_UsesPackageImageNotFoundation both pass. BuildFromDockerfile invocations == 0, Run() image is the package image (no "sandbox-foundation" prefix).

---

## Scenario 12: Sandbox builds and uses foundation image for plans with dependencies

**ID**: scenario-12
**Testable after**: #1961
**Commands**:
- `go test ./internal/sandbox/ -run 'TestSandbox.*Foundation' -v -count=1`
**Expected**: When the plan has InstallTime dependencies, `Executor.Sandbox()` calls `BuildFoundationImage`, which calls `BuildFromDockerfile` exactly once. The image name passed to `Run()` is the foundation image name (matching `tsuku/sandbox-foundation:{family}-{hash}`), not the package image.
**Status**: passed

**Validation Output**: See wip/research/implement-doc_validation_issue1961.md - Three tests pass: TestSandbox_WithDeps_BuildsFoundation (exactly 1 BuildFromDockerfile call, foundation image name matches pattern), TestSandbox_WithDeps_FoundationImageUsedAsContainerBase (Run() image is foundation, no mount shadows /workspace/tsuku), TestSandbox_WithDeps_CachedFoundationSkipsBuild (cached foundation skips build but still uses foundation for Run).

---

## Scenario 13: Foundation image integration -- sandbox skips pre-installed dependencies

**ID**: scenario-13
**Testable after**: #1961
**Environment**: manual (requires Docker/Podman + network access for toolchain download)
**Commands**:
- `go build -o tsuku-test ./cmd/tsuku`
- `export TSUKU_HOME=$(mktemp -d)`
- `./tsuku-test install --sandbox cargo-nextest`
- Inspect stdout for "Skipping" or absence of "Installing rust" on second recipe in same batch
**Expected**: When a foundation image is pre-built with Rust, the sandbox run's output shows that Rust installation was skipped (the executor's `os.Stat` skip logic fires). A second sandbox run for the same recipe reuses the foundation image without rebuilding it (no `BuildFromDockerfile` call). This validates the end-to-end flow: dependency extraction, Dockerfile generation, image building, targeted mounts, and skip logic all working together.
**Status**: passed

**Validation Output**: See wip/research/implement-doc_validation_issue1961.md - Initially failed: Foundation image Docker build failed at `RUN tsuku install --plan` step with "plan is for -, but this system is linux-amd64". Root cause: `dependencyToPlan()` did not set the Platform field. Fixed in commit 8d1e134b by threading Platform through FlattenDependencies → flattenDFS → dependencyToPlan. Unit tests verify Platform propagation (TestFlattenDependencies_PropagatesPlatform, TestBuildFoundationImage_PlanFilesContainPlatform).

---

## Scenario 14: Ecosystem classification sorts recipes correctly before batching

**ID**: scenario-14
**Testable after**: #1962
**Commands**:
- Simulate the jq classification and sorting logic with test data:
```bash
RECIPES='[
  {"name":"cargo-audit","path":"recipes/c/cargo-audit.toml","ecosystem":"rust"},
  {"name":"prettier","path":"recipes/p/prettier.toml","ecosystem":"nodejs"},
  {"name":"fzf","path":"recipes/f/fzf.toml","ecosystem":"none"},
  {"name":"b3sum","path":"recipes/b/b3sum.toml","ecosystem":"rust"},
  {"name":"esbuild","path":"recipes/e/esbuild.toml","ecosystem":"nodejs"}
]'
echo "$RECIPES" | jq -c 'sort_by(.ecosystem) | .[].ecosystem'
```
**Expected**: Output order is `nodejs, nodejs, none, rust, rust` -- same-ecosystem recipes are adjacent. When batched with size 3, batch 1 contains all nodejs recipes (and one none), batch 2 contains rust recipes. The ecosystem field classification maps: `cargo_build`/`cargo_install` to rust, `npm_install`/`npm_exec` to nodejs, `go_build`/`go_install` to go, `pipx_install` to python, `gem_install` to ruby, everything else to none.
**Status**: passed

**Validation Output**: See wip/research/implement-doc_validation_issue1962.md - All 4 test groups passed: jq sort ordering (5 recipes), batching with adjacency (8 recipes, 3 batches), action-to-ecosystem classification (13/13 mappings), and grep+sed against real TOML files (7/7 recipes). Both workflow_dispatch and PR code paths verified. macOS jobs confirmed unchanged.

---

## Scenario 15: WithCargoRegistryCacheDir option and mount construction

**ID**: scenario-15
**Testable after**: #1963
**Commands**:
- `go test ./internal/sandbox/ -run 'TestWithCargoRegistryCacheDir|TestSandbox.*CargoRegistry' -v -count=1`
**Expected**: `WithCargoRegistryCacheDir("/some/path")` sets the `cargoRegistryCacheDir` field. When set, `Executor.Sandbox()` appends a read-write mount at `/workspace/cargo-registry-cache`. `buildSandboxScript()` includes a symlink snippet (`ln -sfn /workspace/cargo-registry-cache $CARGO_HOME/registry`) before the install invocation. When not set, the mount and symlink snippet are absent.
**Status**: pending

---

## Scenario 16: End-to-end cargo registry sharing across families

**ID**: scenario-16
**Testable after**: #1963
**Environment**: manual (requires Docker/Podman + network access)
**Commands**:
- `go build -o tsuku-test ./cmd/tsuku`
- `export TSUKU_HOME=$(mktemp -d)`
- `CARGO_CACHE=$(mktemp -d)`
- `./tsuku-test install --sandbox b3sum` (with cargo registry cache set to `$CARGO_CACHE`)
- `ls -la $CARGO_CACHE/`
**Expected**: After the first family completes `cargo fetch`, the shared cargo registry cache directory contains crate index and downloaded `.crate` files. Subsequent families find crates already present and skip network fetches. The registry directory is populated with at least `cache/` and/or `src/` subdirectories.
**Status**: pending

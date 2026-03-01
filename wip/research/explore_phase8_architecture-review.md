# Architecture Review: Sandbox Build Cache (Revised Design)

Reviewer: architect-reviewer
Design: `docs/designs/DESIGN-sandbox-build-cache.md`
Relevant code: `internal/sandbox/executor.go`, `internal/sandbox/container_spec.go`, `internal/validate/runtime.go`, `internal/executor/executor.go`, `internal/executor/plan.go`

This review covers the revised design that uses per-dependency Dockerfile RUN commands with targeted mounts. The previous review was against an earlier draft that used `docker commit` and `/opt/ecosystem/`. The revised design addresses all three blocking findings from that review.

---

## Architecture Assessment

### 1. Is the architecture clear enough to implement?

Yes. The design is implementable as written, with one interface change that needs precise specification (see finding 1 below). The data flow section maps each step to a named function with a clear signature. The Dockerfile generation examples are concrete and reproducible. The targeted mount table specifies every host-to-container path, mode, and purpose.

The key architectural insight -- that `TSUKU_HOME=/workspace/tsuku` in both the foundation image build and the sandbox run means the executor's existing `os.Stat` skip logic works without modification -- is correct and well-supported by the code. `installSingleDependency()` at `internal/executor/executor.go:590-616` checks `os.Stat(finalDir)` where `finalDir` is `$TSUKU_HOME/tools/{name}-{version}/`. If the foundation image installed the dependency to that path, and the sandbox doesn't mount over `/workspace/tsuku/tools/`, the stat succeeds and the dependency is skipped. This is the core correctness claim and it holds.

### 2. Missing components or interfaces

**2a. `runtime.Build()` signature change is underspecified**

The design says "Extend `runtime.Build()` to accept a build context directory (currently uses `.` as context)." The current `Build` signature:

```go
Build(ctx context.Context, imageName, baseImage string, buildCommands []string) error
```

The current implementation pipes the Dockerfile via stdin and uses `.` as the build context:

```go
cmd := exec.CommandContext(ctx, r.path, "build", "-t", imageName, "-f", "-", ".")
cmd.Stdin = strings.NewReader(dockerfile)
```

The foundation Dockerfile uses `COPY tsuku /usr/local/bin/tsuku` and `COPY plans/ /tmp/plans/`, which require a build context directory containing those files. The current approach of piping stdin with `.` as context won't work because the build context needs to be the temp directory containing the tsuku binary and plan files.

Two options:

1. **Add a `contextDir` parameter to `Build()`**: `Build(ctx, imageName, baseImage string, buildCommands []string, contextDir string) error`. This changes the interface signature, breaking both `podmanRuntime` and `dockerRuntime` implementations plus the one callsite in `executor.go:217`.

2. **Add a new `BuildFromDockerfile()` method**: `BuildFromDockerfile(ctx, imageName, dockerfile, contextDir string) error`. This keeps `Build()` unchanged for existing callers and adds a separate method for the foundation image path that takes a raw Dockerfile string and a context directory.

Option 2 is cleaner -- the existing `Build()` callers don't need a context directory (they generate Dockerfiles from base image + commands), while foundation images generate complete Dockerfiles and need a real build context. These are different operations.

The design should specify which approach to use, including the exact interface signature. The implementer will otherwise make this decision ad hoc, and the wrong choice affects all `Runtime` interface consumers.

Severity: **Blocking** -- this is the primary interface change in the design, and it affects both runtime implementations.

**2b. Foundation image build needs network access during `docker build`**

The foundation Dockerfile runs `tsuku install --plan /tmp/plans/dep-00-rust.json --force` inside a `RUN` command. For Rust, this downloads the toolchain from the internet. `docker build` has network access by default, so this works. But the design should explicitly state this assumption, since the sandbox's security model emphasizes network restriction. The package image build (existing code) also has implicit network access during `docker build` for `apt-get install`. Consistent, but worth documenting.

Severity: **Advisory** -- works by default, but the implicit assumption should be explicit.

**2c. No `ImageRemove` / `ImagePrune` on Runtime interface**

Phase 3 mentions pruning stale foundation images. The `Runtime` interface has no image removal method. This will need to be added when Phase 3 is implemented. The design should note this as a future interface extension.

Severity: **Advisory** -- Phase 3 is out of scope for Phase 1 implementation.

### 3. Implementation phase sequencing

The sequencing is correct.

Phase 1 (foundation images + targeted mounts) is tightly coupled -- you can't use foundation images without targeted mounts (the broad mount shadows them), and targeted mounts without foundation images provide no benefit by themselves. Packaging these together is correct.

Phase 2 (cargo registry cache) is independent and additive. Could be implemented before or after Phase 1.

Phase 3 (CI adaptation) depends on Phase 1.

### 4. Simpler alternatives overlooked?

The revised design has already evaluated the relevant alternatives. The `docker commit` approach from the first draft was correctly replaced with the Dockerfile-based approach, which is simpler, reproducible, and reuses the existing `runtime.Build()` pattern. The symlink bridge and dual-TSUKU_HOME approaches were correctly rejected.

One alternative not discussed: **mounting the foundation image's filesystem as a read-only volume instead of using it as the base image**. For example, `docker create` the foundation image, then use `docker cp` or `--volumes-from` to access its filesystem. This would let you keep the broad workspace mount while adding a read-only overlay of the pre-installed tools. However, this is more complex than the targeted mount approach and introduces Docker-specific lifecycle management. The chosen approach is simpler.

### 5. Download cache mount overlap with foundation image

The design mounts the download cache at `/workspace/tsuku/cache/downloads` (read-write). The foundation image installs tools to `/workspace/tsuku/tools/`, `/workspace/tsuku/bin/`, etc. These paths don't overlap -- `cache/downloads` is a subdirectory of the TSUKU_HOME tree but doesn't shadow `tools/` or `bin/`.

However, the mount at `/workspace/tsuku/cache/downloads` does create a subtlety: it creates `/workspace/tsuku/cache/` as a mount point, which means any other content the foundation image placed under `/workspace/tsuku/cache/` (if any) would be partially shadowed. In practice, tsuku only uses `cache/downloads/` under that path, and the foundation image's `RUN` commands wouldn't populate a download cache (they install tools, not cache downloads). So this is not an issue.

The design correctly identifies this mount as read-write because "the container may need to download additional artifacts during installation that weren't pre-fetched during host-side plan generation." This is accurate for ecosystem build steps like `cargo_build` that may fetch dependencies at build time.

### 6. File mounts vs directory mounts (Docker/Podman behavior)

The targeted mount table includes two individual file mounts:

```
plan.json  -> /workspace/plan.json  (read-only)
sandbox.sh -> /workspace/sandbox.sh (read-only)
```

And two directory mounts:

```
cacheDir  -> /workspace/tsuku/cache/downloads (read-write)
outputDir -> /workspace/output                 (read-write)
```

Docker and Podman handle file mounts (bind-mounting a single file) differently from directory mounts in one important edge case: if the file is replaced on the host (e.g., by writing a new file to the same path, which atomically creates a new inode), the container still sees the old inode. This doesn't apply here because `plan.json` and `sandbox.sh` are written once before the container starts and never modified during the run.

A more relevant concern: **if the host path for a file mount doesn't exist at container start time, Docker creates it as an empty directory** (not a file). Podman's behavior differs -- it creates an empty file. The `Sandbox()` method writes both files before constructing the mount, so this shouldn't occur. But the code should ensure the files are written before the mounts are constructed -- which it does (lines 252-268 in `executor.go` write the plan and script before constructing `RunOptions`). No issue.

### 7. The no-deps fallback path

The design says: "when plan has no dependencies, fall back to current single workspace mount for backward compatibility." This creates two code paths in `Sandbox()`:

- **With deps**: foundation image + targeted mounts, verification markers at `/workspace/output/`
- **Without deps**: package image + single workspace mount, verification markers at `/workspace/`

This means `buildSandboxScript()` needs to know which path it's on to write markers to the correct location. And `readVerifyResults()` needs to know which directory to read from. The design acknowledges this: "conditionally emit `mkdir -p` only when no foundation image" and "write verification markers to `/workspace/output/` instead of `/workspace/`."

The cleaner approach is to use targeted mounts for both paths. Even without a foundation image, the targeted mounts work -- the only difference is which image is used (package image vs foundation image). The `plan.json`, `sandbox.sh`, cache, and output mounts are needed regardless. The `mkdir -p /workspace/tsuku/*` lines in the sandbox script handle the case where no foundation image pre-created the directory structure.

Using the same mount strategy for both paths eliminates the conditional logic and means `buildSandboxScript()` and `readVerifyResults()` don't need to branch. The design's own "Sandbox Script Changes" section even notes this: "When there's no foundation image, the sandbox script still needs `mkdir -p` for the TSUKU_HOME structure since there's no image layer providing it."

Severity: **Advisory** -- the design's conditional approach works, but a unified targeted-mounts-always approach is simpler. This is an implementation decision the implementer can make.

### 8. `runtime.Build()` current working directory matters

Both `podmanRuntime.Build()` and `dockerRuntime.Build()` pass `.` as the build context:

```go
cmd := exec.CommandContext(ctx, r.path, "build", "-t", imageName, "-f", "-", ".")
```

The `.` refers to the current working directory of the Go process, not a parameter. For the existing package image builds, this doesn't matter because the Dockerfile contains only `FROM` and `RUN` commands (no `COPY`). For foundation images, the `COPY` commands require the build context to contain `tsuku` and `plans/`. The design correctly identifies this: "Extend `runtime.Build()` to accept a build context directory."

If the new method doesn't change directories or accept a context path, the `COPY` commands will fail. This is handled by finding 2a above.

### 9. Tsuku binary in the build context: cleanup

The generated Dockerfile includes:

```dockerfile
COPY tsuku /usr/local/bin/tsuku
...
RUN rm -rf /usr/local/bin/tsuku /tmp/plans
```

The cleanup `RUN rm` creates a new layer that doesn't actually reclaim disk space -- Docker layers are additive. The tsuku binary (~20MB) remains in the layer cache. For local usage this is negligible. For CI with tight disk, it's a minor inefficiency. A multi-stage build would avoid this, but adds complexity for ~20MB savings.

Severity: **Advisory** -- not worth changing the design. Just noting for awareness.

---

## Security Assessment

### 1. Attack vectors

**1a. Foundation image cache poisoning**

Same analysis as the previous review: requires Docker group access or root, which already grants root-equivalent host access. Not a meaningful expansion of the attack surface. The foundation image naming uses content-hash of the Dockerfile, so the attacker would need to build a poisoned image with the exact same tag. Defense in depth could include recording the image digest after build and checking it before use, but this is not necessary given the threat model.

**1b. Plan JSON as cache key input**

The plan JSON is copied into the Docker build context and used as `COPY` input. Docker's layer caching includes the content of `COPY` sources in its cache key calculation. If two plans have identical content, they produce the same layer. If the plan content changes (different URL, different checksum), Docker rebuilds the layer. This is the correct behavior -- the plan content is the effective cache key.

One edge case: if the plan JSON contains non-deterministic fields (like `generated_at` timestamp), two functionally identical plans would have different content and produce different cache keys, defeating layer sharing. The design uses standalone plans (converted from `DependencyPlan` to `InstallationPlan`), and the conversion needs to produce deterministic output. `generated_at` should be omitted or set to a fixed value in the standalone dependency plans.

Severity: **Advisory** -- the `FlattenDependencies` function should ensure the standalone plans are deterministic (no timestamps, no non-deterministic field ordering). JSON marshaling in Go produces deterministic key ordering, so map iteration order isn't a concern for `json.Marshal`.

**1c. Env var leakage into foundation image layers**

The design correctly notes that `ExtraEnv` from `SandboxRequirements` is applied at sandbox runtime, not during foundation image construction. The foundation Dockerfile only sets `TSUKU_HOME` and `PATH`. No credential leakage into cached layers.

**1d. Foundation image built with network access**

The `RUN tsuku install --plan ...` commands in the foundation Dockerfile download artifacts from the internet. A compromised DNS or MITM could redirect downloads. Mitigation: the plan includes SHA256 checksums, and tsuku verifies them during installation. This is the same protection as non-sandbox installations. Adequate.

### 2. Mitigation sufficiency

The mitigations are adequate for all identified risks. The revised design (Dockerfile-based instead of `docker commit`) is inherently more secure than the previous draft because:

1. Foundation images are reproducible from their Dockerfile
2. No container lifecycle to manage (no detached containers that could persist)
3. Build runs under Docker's standard build security model

### 3. Read-write download cache mount

The download cache is mounted read-write. The design explains why: "the container may need to download additional artifacts during installation that weren't pre-fetched during host-side plan generation." This is correct for ecosystem build steps.

The risk is that a compromised container could write malicious content to the shared cache, affecting subsequent runs. Mitigation: tsuku verifies checksums on all downloads via the plan. A file with a wrong checksum would be rejected.

One gap: if the cache directory is shared across concurrent sandbox runs (parallel families), and one container writes a partially downloaded file that another container reads, the second container would get a checksum mismatch and fail (not silently succeed with bad data). This is a correctness issue, not a security issue.

---

## Structural Fit

### Follows existing patterns

- **Image naming**: `tsuku/sandbox-foundation:{family}-{hash16}` follows `tsuku/sandbox-cache:{family}-{hash16}` from `ContainerImageName()` in `container_spec.go:368-422`. Same hash width, same family prefix convention.
- **Build pattern**: Check `ImageExists()`, build if missing. Same as `executor.go:204-226`.
- **Executor option**: `WithCargoRegistryCacheDir()` would follow `WithDownloadCacheDir()` at `executor.go:84-89`.
- **File placement**: `internal/sandbox/foundation.go` is correctly scoped to the sandbox package.
- **No dispatch bypass**: Foundation images use `runtime.Build()` (or a new method on the same interface), not direct shell-outs. The Runtime interface remains the single dispatch point.
- **No state contract violation**: No new fields added to `InstallationPlan` or `DependencyPlan`. The design reads from `plan.Dependencies` (existing field) without modification.

### No parallel pattern introduction

The design extends the existing three-level image hierarchy (base -> package -> foundation) rather than creating a parallel caching mechanism. Foundation images use the same Docker layer cache that package images use. The targeted mount structure is a refinement of the existing mount approach, not a separate system.

### Dependency direction

`internal/sandbox/foundation.go` would import `internal/executor` (for `InstallationPlan`, `DependencyPlan`) and `internal/validate` (for `Runtime`). The sandbox package already imports both of these. No new dependency edges, no circular dependencies.

---

## Summary of Findings

| # | Finding | Severity | Section |
|---|---------|----------|---------|
| 1 | `runtime.Build()` needs to accept a build context directory for `COPY` commands. The exact interface change (new parameter vs new method) should be specified in the design. Both runtime implementations and the existing callsite are affected. | Blocking | Architecture 2a |
| 2 | Foundation image build relies on default network access during `docker build`. Design should state this explicitly since the sandbox security model emphasizes network restriction. | Advisory | Architecture 2b |
| 3 | `generated_at` or other non-deterministic fields in standalone dependency plans would defeat Docker's layer cache deduplication. `FlattenDependencies` should produce deterministic plan JSON. | Advisory | Security 1b |
| 4 | The no-deps fallback to the current broad workspace mount creates two code paths in `buildSandboxScript()` and `readVerifyResults()`. Using targeted mounts for both paths (with `mkdir -p` in the script for the no-foundation case) would eliminate the conditional. | Advisory | Architecture 7 |
| 5 | `RUN rm -rf /usr/local/bin/tsuku /tmp/plans` in the final layer doesn't reclaim disk space in Docker's layer model. The binary persists in earlier layers. Minor disk waste (~20MB). | Advisory | Architecture 9 |
| 6 | Phase 3 (image pruning) will need an `ImageRemove` method on the `Runtime` interface. Not needed for Phase 1 but worth noting. | Advisory | Architecture 2c |

### Compared to previous review

The revised design resolves all three previously blocking findings:

1. **`Run()` always passes `--rm`**: No longer relevant -- foundation images use `runtime.Build()` via Dockerfile, not `docker commit`.
2. **`ExtractEcosystemDeps` data source**: Replaced by `FlattenDependencies()` that reads from `plan.Dependencies` directly.
3. **Multi-stage Dockerfile alternative**: The revised design adopted this approach (per-dep RUN commands in a generated Dockerfile).

The remaining blocking finding (item 1 above) is a new concern specific to the revised design's use of `COPY` in generated Dockerfiles.

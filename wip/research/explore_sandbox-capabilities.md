# Sandbox Capabilities Analysis

Research output from analyzing the sandbox system vs CI container usage.

## 1. What does `--sandbox` do end-to-end?

The `--sandbox` flag triggers `runSandboxInstall()` in `cmd/tsuku/install_sandbox.go`, which orchestrates:

1. **Plan acquisition** - One of three modes:
   - `tsuku install <tool> --sandbox` - generates plan from registry recipe
   - `tsuku install --recipe <path> --sandbox` - generates plan from local recipe file
   - `tsuku install --plan <path> --sandbox` - loads pre-generated plan JSON

2. **Requirement computation** - `ComputeSandboxRequirements()` analyzes the plan to determine:
   - Whether network access is needed (by querying each action's `RequiresNetwork()`)
   - Which container image to use (`debian:bookworm-slim` for binary, `ubuntu:22.04` for source builds)
   - Resource limits (2GB/2CPU/2min for binary, 4GB/4CPU/15min for source builds)
   - When `--target-family` is set, selects family-specific base images from `familyToBaseImage`

3. **System dependency extraction** - `ExtractSystemRequirements()` pulls package and repository declarations from plan steps (e.g., `apt_install`, `apt_ppa` actions).

4. **Container image preparation** - `DeriveContainerSpec()` creates a container spec with build commands:
   - Maps package managers to Linux families (apt->debian, dnf->rhel, pacman->arch, apk->alpine, zypper->suse)
   - Generates `RUN` commands for repo setup (with GPG verification) and package installation
   - Adds infrastructure packages (ca-certificates, curl for network; build-essential/gcc for builds)
   - Generates a deterministic image name via SHA256 hash for caching

5. **Container image build** - If the cached image doesn't exist, builds it from the base + build commands.

6. **Sandbox script generation** - `buildSandboxScript()` creates a `/bin/sh` script that:
   - Creates `$TSUKU_HOME` directory structure (`recipes/`, `bin/`, `tools/`)
   - Adds `$TSUKU_HOME/bin` to `PATH` for dependency binaries
   - Runs `tsuku install --plan /workspace/plan.json --force`

7. **Container execution** - Runs the container with:
   - Workspace directory mounted at `/workspace` (read-write)
   - Download cache mounted at `/workspace/tsuku/cache/downloads` (read-only)
   - Tsuku binary mounted at `/usr/local/bin/tsuku` (read-only)
   - Environment: `TSUKU_SANDBOX=1`, `TSUKU_HOME=/workspace/tsuku`, `DEBIAN_FRONTEND=noninteractive`
   - Network mode: `none` (default) or `host` (when network required)
   - Resource limits applied via container runtime

8. **Result evaluation** - Pass/fail based on container exit code 0.


## 2. What output/artifacts does sandbox produce?

The sandbox produces:

- **Pass/fail status** - `SandboxResult.Passed` (bool)
- **Exit code** - integer exit code from the container
- **Stdout/Stderr capture** - full container output stored in `SandboxResult.Stdout` and `SandboxResult.Stderr`
- **Error detail** - if the container runtime itself fails, `SandboxResult.Error` is set

What sandbox does NOT produce:
- No structured JSON results file
- No installed binaries preserved after the test (workspace is `defer os.RemoveAll`'d)
- No verification scripts run (no `verify-tool.sh`, no `verify-binary.sh`)
- No per-platform result aggregation
- No GitHub step summary output
- No retry logic for transient failures


## 3. Can sandbox test across multiple families in a single invocation?

**No.** The sandbox accepts a single `platform.Target` and produces a single `SandboxResult`. To test across families, you must invoke sandbox multiple times, each with a different `--target-family` flag.

The CI workflow `build-essentials.yml` demonstrates this pattern:
```yaml
strategy:
  matrix:
    family: [debian, rhel, arch, alpine, suse]
    tool: [cmake, ninja]
steps:
  - run: ./tsuku eval ${{ matrix.tool }} --os linux --linux-family ${{ matrix.family }} --install-deps > plan.json
  - run: ./tsuku install --plan plan.json --sandbox --force
```

Each family is a separate job. Sandbox has no built-in matrix/loop mechanism.


## 4. What happens when sandbox detects system dependencies?

When the plan contains system dependency actions (e.g., `apt_install`, `apt_ppa`):

1. `ExtractSystemRequirements()` pulls packages and repository configs from plan steps
2. `augmentWithInfrastructurePackages()` adds needed infrastructure packages (ca-certificates, curl, build-essential)
3. `DeriveContainerSpec()` generates:
   - A family-appropriate base image
   - Build commands for repository setup (GPG key download + verification for apt repos, PPA addition)
   - Package installation commands
4. A custom container image is built and cached by hash
5. The sandbox script runs inside this custom image

Key: System dependencies are installed at **container build time** (in the image), not at **script runtime**. This means subsequent runs with the same dependencies reuse the cached image.


## 5. Does sandbox support custom verification?

**No.** The sandbox script is hardcoded to run exactly one command:

```sh
tsuku install --plan /workspace/plan.json --force
```

There is no mechanism to:
- Run verification scripts (`verify-tool.sh`, `verify-binary.sh`)
- Execute the installed binary to confirm it works
- Run custom post-install checks
- Validate binary quality (relocation, system deps)

The pass/fail is purely based on whether `tsuku install --plan` exits 0.


## 6. Key limitations preventing sandbox from replacing CI docker calls

### 6.1 No verification beyond installation

CI workflows run multiple verification steps after installation:
- `verify-tool.sh` (functional verification - runs the tool)
- `verify-binary.sh` (binary quality - checks relocation and dynamic linking)
- Custom verification commands (e.g., `just --version`)

Sandbox only checks that `tsuku install --plan` succeeds. A recipe could install a broken binary and sandbox would report success.

### 6.2 No environment variable passthrough

CI workflows pass `GITHUB_TOKEN` and `TSUKU_REGISTRY_URL` into containers:
```yaml
docker run --rm -e GITHUB_TOKEN -e TSUKU_REGISTRY_URL ...
```

Sandbox hardcodes a fixed set of env vars (`TSUKU_SANDBOX=1`, `TSUKU_HOME`, `HOME`, `DEBIAN_FRONTEND`, `PATH`). There's no way to pass `GITHUB_TOKEN` for rate-limited API calls or `TSUKU_REGISTRY_URL` for PR-branch testing.

### 6.3 No retry logic for transient failures

CI workflows (`recipe-validation-core.yml`, `test-recipe-changes.yml`) implement retry with exponential backoff for network errors (exit code 5):
```bash
for attempt in 0 1 2; do
  ...
  elif [ "$EXIT_CODE" = "5" ] && [ "$attempt" -lt 2 ]; then
    echo "Network error, retrying..."
    sleep $((2 ** (attempt + 1)))
    continue
```

Sandbox has no retry mechanism. A transient network error fails the test immediately.

### 6.4 No structured result reporting

CI workflows produce:
- JSON results files (`validation-results-*.json`) with recipe, platform, status, exit_code, attempts
- GitHub step summaries with markdown tables
- Per-platform aggregated results

Sandbox returns a Go struct with pass/fail, stdout, stderr. No structured output for CI consumption.

### 6.5 No source-code/recipe-file mounting

CI workflows mount the full checkout (`-v "$PWD:/workspace"`) into containers, giving access to:
- Recipe files (for `--recipe` installs)
- Test scripts (`test/scripts/verify-*.sh`)
- Registry recipes (local `TSUKU_REGISTRY_URL=${{ github.workspace }}`)

Sandbox only mounts the workspace temp dir, download cache, and the tsuku binary. The recipe is serialized as a plan JSON -- there's no way to use `--recipe` mode inside the sandbox or access the repo's test infrastructure.

### 6.6 Single-tool-per-invocation

CI workflows like `test-recipe-changes.yml` test multiple recipes in a single container run (batch mode with shared download cache). Sandbox tests exactly one plan per container run.

### 6.7 No macOS support

Sandbox relies on Linux container images. CI workflows also test on `macos-14` (arm64) and `macos-15-intel` (x86_64) runners natively. Sandbox cannot replace macOS testing.

### 6.8 Static binary not used

CI workflows build with `CGO_ENABLED=0` for a static binary that works across all Linux families. Sandbox uses `findTsukuBinary()` which finds the current executable, which may be dynamically linked against glibc. The `DefaultSandboxImage` comment acknowledges this: "Uses Debian because the tsuku binary is dynamically linked against glibc."

### 6.9 No timeout customization per recipe

CI workflows use `timeout 300` per recipe. Sandbox uses the resource limit timeout (2 min default, 15 min for source builds) which applies to the entire container, not per-recipe. Some recipes may need different timeouts.

### 6.10 No jq/build-essential pre-installation for all families

CI workflows install `jq`, `build-essential`/equivalents, and other utilities for every container run. Sandbox only installs infrastructure packages when the plan requires them. A recipe that doesn't declare system deps but expects common utilities (like `tar` or `gzip`) could fail in sandbox but pass in CI.


## 7. How does sandbox handle exit codes and failure reporting?

Exit code handling in `executor.go`:

```go
result, err := runtime.Run(ctx, opts)
if err != nil {
    return &SandboxResult{
        Passed:   false,
        ExitCode: -1,
        Stdout:   result.Stdout,
        Stderr:   result.Stderr,
        Error:    err,
    }, nil  // Note: returns nil error, wrapping failure in result
}

passed := result.ExitCode == 0
return &SandboxResult{
    Passed:   passed,
    ExitCode: result.ExitCode,
    Stdout:   result.Stdout,
    Stderr:   result.Stderr,
}, nil
```

Key behaviors:
- Runtime infrastructure errors (can't detect runtime, can't build image) return as Go errors
- Container execution errors (runtime.Run fails) return as `SandboxResult` with `ExitCode: -1` and `Error` set, but the function returns `nil` error
- Non-zero container exit codes are reported as `Passed: false` with the actual exit code
- The CLI layer (`install_sandbox.go`) maps these to `ExitInstallFailed` exit code

There is no distinction between:
- Recipe-level failures (e.g., download URL 404)
- Network errors (exit code 5 in tsuku convention)
- Timeout kills
- OOM kills


## Comparison: CI workflow patterns vs sandbox

| Capability | CI Docker Calls | Sandbox |
|---|---|---|
| Installation testing | Yes | Yes |
| Functional verification (run tool) | Yes (`verify-tool.sh`) | No |
| Binary quality checks | Yes (`verify-binary.sh`) | No |
| Multi-family per run | Yes (matrix strategy) | No (one family per invocation) |
| Multi-recipe per run | Yes (batch loops) | No (one recipe per invocation) |
| Environment passthrough | Yes (GITHUB_TOKEN, TSUKU_REGISTRY_URL) | No |
| Retry on transient failure | Yes (3 attempts, exponential backoff) | No |
| Structured results | Yes (JSON files) | No |
| GitHub step summary | Yes (markdown tables) | No |
| macOS testing | Yes (native runners) | No |
| ARM64 testing | Yes (arm64 runners) | No (only host arch in containers) |
| Custom timeout per recipe | Yes (`timeout 300`) | No (single container-level timeout) |
| Image caching | No (pulls fresh each time) | Yes (hash-based caching) |
| System dep auto-detection | N/A (hardcoded per family) | Yes (extracts from plan) |
| Container security (rootless warning) | No | Yes |
| Download cache reuse | Partial (via `runner.temp`) | Yes (mount read-only) |
| Local recipe testing | No (CI only tests committed recipes) | Yes (`--recipe` flag) |


## Key capability gaps (summary)

The three most significant gaps that prevent sandbox from replacing CI docker usage:

1. **No post-install verification** -- Sandbox proves a recipe installs but never confirms the installed tool actually works. CI runs `verify-tool.sh` and `verify-binary.sh` to catch broken binaries, missing libraries, and relocation issues.

2. **No environment variable passthrough** -- Without `GITHUB_TOKEN`, sandbox hits GitHub API rate limits during version resolution and download. Without `TSUKU_REGISTRY_URL`, sandbox can't test recipes from PR branches. These are required for any CI-like usage.

3. **No structured result aggregation** -- CI workflows produce JSON results and markdown summaries across multiple platforms and recipes. Sandbox returns a single pass/fail Go struct. Any CI workflow using sandbox would need to rebuild all the batching, retry, and reporting logic externally.

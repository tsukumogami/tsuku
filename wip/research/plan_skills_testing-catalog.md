# tsuku Recipe Testing Workflow Catalog

## Overview

This document catalogs tsuku's complete recipe testing workflow for instructing recipe authors on how to test their recipes. The workflow spans five major phases: validation (validate command), plan generation (eval command), containerized testing (sandbox), regression testing (golden files), and continuous integration (CI).

---

## Testing Workflow Steps: validate → eval → sandbox → golden → CI

### Phase 1: Validate (Recipe Syntax & Metadata)

**Purpose**: Ensure recipe files are syntactically correct and contain required fields before plan generation.

**Command**: `tsuku validate <recipe-file>`

**Flags**:
- `--json`: Output validation result as JSON
- `--strict`: Treat warnings as errors
- `--check-libc-coverage`: Validate glibc/musl coverage for library recipes

**Checks Performed**:
- TOML syntax validation
- Required fields: `metadata.name`, `steps`, `verify.command`
- Action type and parameter validation
- Security checks (URL schemes, path traversal)
- Dependency shadowing detection
- Hardcoded version detection in action parameters
- Libc coverage analysis (libraries without musl support are flagged)

**Output**:
```
Valid recipe: <recipe-name>
```

**Exit Codes**:
- 0: Recipe is valid
- 1 (ExitGeneral): Recipe is invalid
- 2 (ExitUsage): Invalid command usage

---

### Phase 2: Eval (Deterministic Plan Generation)

**Purpose**: Generate a complete, self-contained installation plan with resolved URLs, checksums, and dependencies. Plans enable reproducible testing and serve as golden files.

**Command**: `tsuku eval <tool>[@version]`

**Flags**:
- `--os <os>`: Target OS (linux, darwin). Default: current platform
- `--arch <arch>`: Target architecture (amd64, arm64). Default: current platform
- `--linux-family <family>`: Linux distribution family (debian, rhel, arch, alpine, suse)
- `--install-deps`: Auto-install eval-time dependencies
- `--recipe <path>`: Evaluate a local recipe file (for testing)
- `--version <version>`: Version to evaluate (with `--recipe`)
- `--pin-from <golden-file>`: Extract version constraints from existing golden file for deterministic output
- `--require-embedded`: Require action dependencies from embedded registry only

**Platform Validation**:
- Valid OS values: `linux`, `darwin`
- Valid arch values: `amd64`, `arm64`
- Valid Linux families: `debian`, `rhel`, `arch`, `alpine`, `suse`

**Plan Output**:
JSON format containing:
- `format_version`: Plan format identifier
- `tool`: Tool name
- `version`: Resolved version
- `platform`: Target OS, arch, Linux family
- `generated_at`: Timestamp
- `steps`: Resolved action steps with URLs, checksums, parameters
- `dependencies`: Nested dependency plans (preserving full subtree)
- `verify`: Verification command

**Examples**:
```bash
# Generate plan for current platform
tsuku eval kubectl

# Generate for specific platform
tsuku eval cmake --os linux --linux-family rhel --arch amd64

# Constrained evaluation using golden file
tsuku eval black@26.1a1 --pin-from testdata/golden/plans/b/black/v26.1a1-darwin-amd64.json

# Test local recipe
tsuku eval --recipe ./my-recipe.toml --version v1.2.0 --os darwin --arch arm64
```

**Exit Codes**:
- 0: Plan generated successfully
- 1 (ExitGeneral): Plan generation failed
- 3 (ExitRecipeNotFound): Recipe not found
- 4 (ExitVersionNotFound): Version not found
- 5 (ExitNetwork): Network error during resolution
- 8 (ExitDependencyFailed): Dependency resolution failed
- 9 (ExitDeterministicFailed): Deterministic generation failed (--deterministic-only used)

---

### Phase 3: Sandbox (Containerized Testing)

**Purpose**: Test installation in isolated containers to verify cross-platform compatibility without affecting the host system.

**Command**: `tsuku install <tool|--plan <file>|--recipe <file>> --sandbox [--env VAR=value]...`

**Requirements**:
- Container runtime (Docker or Podman)
- Host platform architecture (tests native binaries only)

**Behavior**:
1. Detects available container runtime (Docker or Podman)
2. Extracts system requirements (packages, repos) from the plan
3. Selects base container image for the target Linux family
4. Builds a "foundation image" with dependencies pre-installed (for build recipes)
5. Runs the installation in the container
6. Captures installation output and exit code
7. Runs verification command in the same container
8. Returns structured result (passed, exit code, stdout, stderr)

**Supported Linux Families** (from `container-images.json`):
- **debian**: `debian:bookworm-slim` (build-essential for compilation)
- **rhel**: `fedora:41` (gcc, gcc-c++, make)
- **arch**: `archlinux:base` (base-devel)
- **alpine**: `alpine:3.21` (build-base, musl libc)
- **suse**: `opensuse/leap:15.6` (gcc, gcc-c++, make)

**Flags**:
- `--sandbox`: Enable sandbox mode
- `--force`: Skip user confirmation and existing installations
- `--json`: Output result as JSON
- `--env VAR=value`: Pass environment variables into container
- `--target-family <family>`: Override target family for plan generation (when using `--recipe`)

**Output Format** (JSON with `--json`):
```json
{
  "passed": true,
  "skipped": false,
  "install_exit_code": 0,
  "verify_exit_code": 0,
  "duration_ms": 12345,
  "stdout": "...",
  "stderr": "",
  "error": null
}
```

**Exit Codes**:
- 0: Installation and verification succeeded
- 1 (ExitGeneral): Installation or verification failed
- 12 (ExitNotInteractive): No container runtime available

**Examples**:
```bash
# Test a tool in default container (Debian)
tsuku install kubectl --sandbox --force

# Test against specific Linux family
tsuku install cmake --sandbox --force --env GITHUB_TOKEN="$GITHUB_TOKEN"

# Test from a plan JSON
tsuku eval rg | tsuku install --plan - --sandbox --force

# Test from a local recipe
tsuku install --recipe ./my-recipe.toml --sandbox --force
```

**Caching**:
- Foundation images cached per Dockerfile hash (format: `tsuku/sandbox-foundation:{family}-{hash16}`)
- Download cache mounted from `$TSUKU_HOME/downloads` (read-only)
- Cargo registry cache mounted to `/workspace/cargo-registry-cache` (shared across families)

---

### Phase 4: Golden Files (Regression Testing)

**Purpose**: Store pre-generated installation plans as regression tests. When recipe or tsuku code changes, CI verifies output matches expected golden files.

**Golden File Location**:
- Embedded recipes: `testdata/golden/plans/embedded/<recipe>/<version>-<os>[-<family>]-<arch>.json`
- Registry recipes: `testdata/golden/plans/<first-letter>/<recipe>/<version>-<os>[-<family>]-<arch>.json`

**Family-Aware Files**:
- Family-aware recipes (using `apt_install`, `dnf_install`, etc.) generate 5 files per version per platform:
  - `v1.0.0-linux-debian-amd64.json`
  - `v1.0.0-linux-rhel-amd64.json`
  - `v1.0.0-linux-arch-amd64.json`
  - `v1.0.0-linux-alpine-amd64.json`
  - `v1.0.0-linux-suse-amd64.json`
- Family-agnostic recipes generate a single Linux file:
  - `v1.0.0-linux-amd64.json`

**Generation**:
```bash
# Manual generation (one platform at a time)
./tsuku eval cmake@4.2.3 --os linux --arch amd64 > testdata/golden/plans/embedded/cmake/v4.2.3-linux-debian-amd64.json

# Automated multi-platform generation via GitHub Actions
# Trigger: Actions UI → generate-golden-files.yml → Enter recipe name, check "commit_back"
```

**Validation Workflow**:
1. CI generates plans for a recipe on all platforms
2. Compares generated plans with stored golden files
3. If mismatch detected, shows diff
4. User reviews changes, accepts if intentional
5. Golden files are committed

**Constrained Evaluation** (for deterministic output):
```bash
# Extract constraints from golden file
tsuku eval black@26.1a1 --pin-from testdata/golden/plans/b/black/v26.1a1-darwin-amd64.json

# This ensures pip, go, cargo, npm, gem, cpan versions match golden file exactly
```

**Exclusions**:
File: `testdata/golden/exclusions.json`
- Recipes that cannot be generated (network-only, platform-specific failures)
- Skipped during CI validation

---

## Commands and Flags: Exact Syntax for Each Step

### Validate
```bash
tsuku validate <recipe-file>
tsuku validate <recipe-file> --json
tsuku validate <recipe-file> --strict
tsuku validate <recipe-file> --check-libc-coverage
```

### Eval
```bash
# Current platform
tsuku eval <tool>
tsuku eval <tool>@<version>

# Cross-platform
tsuku eval <tool> --os linux --arch amd64
tsuku eval <tool> --os linux --linux-family rhel
tsuku eval <tool> --os darwin --arch arm64

# Local recipe
tsuku eval --recipe <path>
tsuku eval --recipe <path> --version <version>
tsuku eval --recipe <path> --os darwin --arch arm64

# With dependencies
tsuku eval <tool> --install-deps

# Constrained evaluation
tsuku eval <tool>@<version> --pin-from <golden-file>

# Deterministic-only (fail if LLM fallback needed)
tsuku eval <tool> --require-embedded

# Output
tsuku eval <tool> | jq .
tsuku eval <tool> > plan.json
```

### Sandbox Install
```bash
# Install a tool
tsuku install <tool> --sandbox --force

# Install with recipe
tsuku install --recipe <path> --sandbox --force

# Install from plan
tsuku install --plan plan.json --sandbox --force
cat plan.json | tsuku install --plan - --sandbox --force

# With environment
tsuku install <tool> --sandbox --force --env GITHUB_TOKEN="$GITHUB_TOKEN"

# Cross-family testing (from plan)
for family in debian rhel arch alpine suse; do
  tsuku eval <tool> --os linux --linux-family "$family" | \
    tsuku install --plan - --sandbox --force
done

# JSON output for CI
tsuku install <tool> --sandbox --force --json > result.json
```

### Golden File Generation
```bash
# Manual (Linux only, generates one file)
tsuku eval <tool> --os linux --arch amd64 > testdata/golden/plans/.../v1.0.0-linux-amd64.json

# Automated via regenerate-golden.sh script
./scripts/regenerate-golden.sh <recipe> --os linux --arch amd64
./scripts/regenerate-golden.sh <recipe> --os darwin --arch arm64

# Via GitHub Actions UI
# 1. Actions → generate-golden-files
# 2. Enter recipe name
# 3. Check "commit_back"
# 4. Run workflow
```

### CI Testing Commands
```bash
# Unit tests
go test ./...
go test -race ./...
go test -short ./...

# Functional tests
make test-functional
make test-functional-critical

# Linting
go test -v -run 'Test(GolangCILint|GoFmt|GoModTidy|GoVet|Govulncheck)' .

# Specific test scripts
./test/scripts/test-checksum-pinning.sh [family]
./test/scripts/test-homebrew-recipe.sh <tool> [family]
./test/scripts/test-library-integrity.sh <library> [family]
./test/scripts/test-library-dlopen.sh <library> [family]
./test/scripts/test-tls-cacerts.sh [tsuku-binary]
./test/scripts/verify-tool.sh <tool>
```

---

## Exit Codes: All Documented Exit Codes with Meanings

| Code | Name | Meaning | When It Occurs |
|------|------|---------|---|
| 0 | ExitSuccess | Operation succeeded | All operations completed without errors |
| 1 | ExitGeneral | General error | Recipe validation failed, install failed, verify failed, generic error |
| 2 | ExitUsage | Invalid usage | Missing required args, invalid flag combination, wrong number of arguments |
| 3 | ExitRecipeNotFound | Recipe not found | Recipe does not exist in embedded registry or registry recipes |
| 4 | ExitVersionNotFound | Version not found | Requested version does not exist for the tool |
| 5 | ExitNetwork | Network error | Download failed, network connectivity issue |
| 6 | ExitInstallFailed | Installation failed | All tools in project install failed |
| 7 | ExitVerifyFailed | Verification failed | Verification command exited non-zero |
| 8 | ExitDependencyFailed | Dependency resolution failed | Required dependency not available, dependency install failed |
| 9 | ExitDeterministicFailed | Deterministic generation failed | LLM fallback needed but `--deterministic-only` flag used |
| 10 | ExitAmbiguous | Multiple sources found | Multiple ecosystem sources detected, user must specify `--from` |
| 11 | ExitIndexNotBuilt | Binary index not built | Run `tsuku update-registry` first |
| 12 | ExitNotInteractive | Not interactive | Confirm mode used without TTY, set `TSUKU_AUTO_INSTALL_MODE` or use `--mode` |
| 13 | ExitUserDeclined | User declined | User rejected interactive prompt |
| 14 | ExitForbidden | Operation forbidden | Security restriction (e.g., running as root) |
| 15 | ExitPartialFailure | Partial failure | Some tools installed, others failed (project install only) |
| 130 | ExitCancelled | Cancelled | User pressed Ctrl+C |

**Note for Project Install Exit Codes**:
- Exit code 6 (ExitInstallFailed): All tools failed
- Exit code 15 (ExitPartialFailure): Some tools succeeded, some failed

---

## Container Testing: Sandbox, Families, Docker-dev.sh

### Supported Container Runtimes

Detection order:
1. `docker` (with rootless check)
2. `podman` (preferred for security)

Security warnings:
- Docker with docker group membership: warns about root-equivalent access
- Recommends rootless Docker mode

### Linux Distribution Families

Base images (from `container-images.json`, with pinned hashes):

| Family | Image | Build Tools | Install Command |
|--------|-------|-------------|---|
| debian | debian:bookworm-slim | build-essential | apt-get install |
| rhel | fedora:41 | gcc, gcc-c++, make | dnf install |
| arch | archlinux:base | base-devel | pacman -S |
| alpine | alpine:3.21 | build-base | apk add |
| suse | opensuse/leap:15.6 | gcc, gcc-c++, make | zypper install |

**Family-Specific Detection**:
- In eval: `--linux-family` flag or auto-detect from host
- In sandbox: extracted from generated plan

### Foundation Images

**Purpose**: Pre-build dependencies as cached Docker layers for faster test runs.

**Naming**: `tsuku/sandbox-foundation:{family}-{hash16}`
- `hash16`: First 16 hex characters of SHA-256 hash of Dockerfile content
- Deterministic: same dependencies always produce same hash

**Building**:
1. Extracts dependency list from plan
2. Generates Dockerfile with COPY+RUN pairs per dependency
3. Checks if image already exists (cached)
4. Builds image if needed, skips if cached

**Dockerfile Structure**:
```dockerfile
FROM <base-image>
COPY tsuku /usr/local/bin/tsuku
ENV TSUKU_HOME=/workspace/tsuku
ENV PATH=/workspace/tsuku/tools/current:/workspace/tsuku/bin:$PATH

# Per-dependency caching layers
COPY plans/dep-00-rust.json /tmp/plans/dep-00-rust.json
RUN tsuku install --plan /tmp/plans/dep-00-rust.json --force

COPY plans/dep-01-cargo.json /tmp/plans/dep-01-cargo.json
RUN tsuku install --plan /tmp/plans/dep-01-cargo.json --force

RUN rm -rf /usr/local/bin/tsuku /tmp/plans
```

### Sandbox Execution Flow

1. **Runtime Detection**: Detect Docker/Podman
2. **Extract Requirements**: Parse plan for system packages and repositories
3. **Select Base Image**: From container-images.json based on Linux family
4. **Build Foundation**: Compile dependencies into cached Docker image
5. **Mount Files**:
   - Read-only: tsuku binary, plan JSON, download cache
   - Read-write: TSUKU_HOME (workspace), cargo registry cache
6. **Run Container**: Execute install with mounted environment
7. **Verify**: Run verification command in same container
8. **Capture Output**: stdout, stderr, exit code

### Caching Strategy

**Download Cache**:
- Location: `$TSUKU_HOME/downloads`
- Mounted read-only into container at `/workspace/downloads`
- Populated during eval phase
- Reused across multiple sandbox tests

**Cargo Registry Cache**:
- Shared cache mounted to `/workspace/cargo-registry-cache`
- Allows cargo fetch results to be shared across Linux families
- Critical for multi-family testing (avoids re-downloading crates)

**Foundation Image Cache**:
- Persisted Docker images named `tsuku/sandbox-foundation:{family}-{hash}`
- Skips rebuild if Dockerfile (and thus dependencies) unchanged

### Docker-dev.sh

**Status**: Does not exist in the repository.

Instead, development uses:
```bash
make build          # Builds tsuku with .tsuku-dev home directory
eval $(./tsuku shellenv)  # Configure shell for dev binary
./tsuku doctor      # Verify environment setup
```

---

## Golden File System: Generation, Validation, Usage

### File Organization

**Embedded Recipes** (built-in):
```
testdata/golden/plans/embedded/<recipe>/<version>-<os>[-<family>]-<arch>.json
```

Examples:
- `testdata/golden/plans/embedded/cmake/v4.2.3-linux-debian-amd64.json`
- `testdata/golden/plans/embedded/cmake/v4.2.3-linux-rhel-amd64.json`
- `testdata/golden/plans/embedded/cmake/v4.2.3-darwin-arm64.json`

**Registry Recipes** (in separate registry):
```
testdata/golden/plans/<first-letter>/<recipe>/<version>-<os>[-<family>]-<arch>.json
```

Examples:
- `testdata/golden/plans/f/fzf/v0.50.0-linux-amd64.json`
- `testdata/golden/plans/f/fzf/v0.50.0-darwin-amd64.json`

### Generation Workflow

#### Manual Generation

```bash
# Generate for one platform
tsuku eval cmake@4.2.3 --os linux --arch amd64 > \
  testdata/golden/plans/embedded/cmake/v4.2.3-linux-amd64.json

# Generate for multiple families (separate commands, one file per family)
for family in debian rhel arch alpine suse; do
  tsuku eval cmake@4.2.3 --os linux --linux-family "$family" --arch amd64 > \
    testdata/golden/plans/embedded/cmake/v4.2.3-linux-${family}-amd64.json
done
```

#### Automated Generation via regenerate-golden.sh

```bash
./scripts/regenerate-golden.sh <recipe> --os linux --arch amd64
./scripts/regenerate-golden.sh <recipe> --os darwin --arch arm64
```

This script:
1. Queries recipe metadata to determine if family-aware
2. If family-aware: generates 5 Linux files (one per family)
3. If family-agnostic: generates single Linux file
4. Generates macOS files for both amd64 and arm64

#### Automated Generation via GitHub Actions

**Workflow**: `.github/workflows/generate-golden-files.yml`

Usage:
1. Click "Actions" → "Generate Golden Files"
2. Enter recipe name
3. (Optional) Check "Commit back to branch"
4. Click "Run workflow"

Results:
- Runs on 3 platforms (Linux x86_64, macOS arm64, macOS amd64) in parallel
- Generates files for all platforms simultaneously
- Optionally commits files back to the current branch
- Handles retry logic (up to 3 attempts for push with exponential backoff)

### Plan JSON Structure

```json
{
  "format_version": "2",
  "tool": "cmake",
  "version": "4.2.3",
  "platform": {
    "os": "linux",
    "arch": "amd64",
    "linux_family": "debian"
  },
  "generated_at": "2024-04-01T12:34:56Z",
  "steps": [
    {
      "action": "download_file",
      "params": {
        "url": "https://github.com/Kitware/CMake/releases/download/v4.2.3/cmake-4.2.3-linux-x86_64.tar.gz",
        "checksum": "sha256:abc123..."
      }
    },
    {
      "action": "extract",
      "params": {"format": "tar.gz"}
    }
  ],
  "dependencies": [
    {
      "tool": "openssl",
      "version": "3.0.0",
      "steps": [...],
      "dependencies": []
    }
  ],
  "verify": {
    "command": "cmake --version"
  }
}
```

### Validation (CI Integration)

**Workflow**: `.github/workflows/batch-generate.yml` and others

Process:
1. Changes to recipes or tsuku code trigger plan generation
2. CI generates plans for affected recipes on all platforms
3. Plans compared with stored golden files
4. If mismatch: shows diff, test fails
5. User reviews changes (intentional or bug)
6. If intentional: accept and update golden files

**Exclusions**:
File: `testdata/golden/exclusions.json`
```json
{
  "exclusions": [
    {"recipe": "tool-with-network-issues"},
    {"recipe": "platform-specific-only"}
  ]
}
```

### Constrained Evaluation (for Testing)

**Purpose**: Generate plans deterministically to match golden files exactly.

**How It Works**:
1. Extract constraints from golden file: `ExtractConstraints(goldenPath)`
2. Constraints include ecosystem-specific version locks:
   - pip: full requirements string with package==version
   - go: go.sum file
   - cargo: Cargo.lock-equivalent info
   - npm: package-lock.json
   - gem: Gemfile.lock
   - cpan: cpan requirements
3. Pass constraints to eval: `tsuku eval <tool> --pin-from <golden>`
4. Eval uses constraints instead of live dependency resolution
5. Output matches golden file exactly

**Example**:
```bash
# Generate, then test determinism
tsuku eval black@26.1a1 --pin-from testdata/golden/plans/b/black/v26.1a1-darwin-amd64.json > test.json
diff test.json testdata/golden/plans/b/black/v26.1a1-darwin-amd64.json
# Should output no diff
```

---

## CI Patterns: Which Workflows Run What

### Primary Test Workflows

#### `.github/workflows/test.yml` (Tests)
**Triggers**: Push to main, pull requests, nightly schedule

**Jobs**:
1. **unit-tests**: Go unit tests (race detector on push, coverage)
2. **lint-tests**: GolangCI lint, GoFmt, GoModTidy, GoVet, Govulncheck
3. **functional-tests**: Functional test suite (`make test-functional`)
4. **rust-test**: Rust tsuku-dltest binary compilation (Linux + macOS)
5. **llm-integration**: LLM integration tests (when code changes)

**Exit Handling**: Any failure fails the job

---

#### `.github/workflows/build-essentials.yml` (Build Essentials)
**Triggers**: Weekly schedule, push to main, pull requests

**Coverage**: Tools that require system dependencies (compilers, libraries)

**Jobs**:
1. **test-linux**: Sandbox tests for build tools
   - Tools: pkg-config, cmake, gdbm, pngcrush, zig, ninja
   - Recipes: libsixel-source, sqlite-source, git-source
   - TLS test: ca-certificates + curl integration
2. **test-sandbox-cmake**: Multi-family sandbox testing (debian, rhel, arch, alpine, suse)
3. **test-sandbox-ninja**: Multi-family sandbox testing

**Test Pattern**:
```bash
./tsuku install --sandbox --force <tool> --json > result.json
# Parse JSON: check .passed == true, .install_exit_code == 0
```

---

#### `.github/workflows/integration-tests.yml` (Integration Tests)
**Triggers**: Push to main, pull requests

**Jobs** (all use containerized test scripts):
1. **checksum-pinning**: Test checksum verification across families
   - Script: `./test/scripts/test-checksum-pinning.sh [family]`
   - Families: debian, rhel, arch, alpine, suse
   - Validates: binary_checksums stored and verified correctly

2. **homebrew-linux**: Test homebrew recipe building
   - Script: `./test/scripts/test-homebrew-recipe.sh <tool> [family]`
   - Tool: pkg-config
   - Families: debian, rhel, arch, suse (alpine excluded: uses apk_install)

3. **library-integrity**: Test library checksum validation
   - Script: `./test/scripts/test-library-integrity.sh <lib> [family]`
   - Libraries: zlib, libyaml
   - Validates: fresh install passes, modified file detected

4. **library-dlopen-glibc**: Test library dlopen via tsuku-dltest (glibc)
   - Script: `./test/scripts/test-library-dlopen.sh <lib> [family]`
   - Libraries: zlib, libyaml, gcc-libs
   - Validates: libraries load successfully via dlopen

5. **library-dlopen-musl**: Test library dlopen in Alpine container (musl libc)
   - Container: `golang:1.23-alpine`
   - Validates: musl libc compatibility

---

#### `.github/workflows/generate-golden-files.yml` (Golden File Generation)
**Triggers**: Manual workflow dispatch, called from other workflows

**Inputs**:
- `recipe`: Recipe name to generate for
- `commit_back`: Auto-commit results to branch (boolean)

**Jobs**:
1. **generate**: Runs on 3 platforms in parallel
   - ubuntu-latest (Linux x86_64, amd64)
   - macos-14 (macOS arm64)
   - macos-15-intel (macOS amd64)

2. **commit** (if commit_back=true): Merges artifacts and commits

---

#### `.github/workflows/batch-generate.yml` (Batch Generation & Validation)
**Triggers**: On recipe changes, builds in batches

**Pattern**: Groups recipes into batches (config: `.github/ci-batch-config.json`)
- test-recipe jobs: `max: 20` recipes per job, at least 5 batches
- validate-golden-recipes: `max: 20` recipes per job
- rust recipes: `max: 3` per job (slower)

**Purpose**: Parallelize CI while avoiding excessive job count

---

#### `.github/workflows/container-tests.yml` (Container Integration Tests)
**Triggers**: Changes to sandbox code or container infrastructure

**Jobs**:
- **sandbox-tests**: `go test -v -timeout 20m ./internal/sandbox/...`

---

### Test Scripts (in `test/scripts/`)

| Script | Purpose | Parameters | Families |
|--------|---------|------------|----------|
| **test-checksum-pinning.sh** | Verify binary checksum storage and tamper detection | `[family]` | All 5 |
| **test-homebrew-recipe.sh** | Test homebrew recipe builds (patchelf/install_name_tool) | `<tool> [family]` | debian, rhel, arch, suse |
| **test-library-integrity.sh** | Test library checksum computation and verification | `<library> [family]` | All 5 |
| **test-library-dlopen.sh** | Test libraries load via dlopen (using tsuku-dltest) | `<library> [family]` | All 5 |
| **test-tls-cacerts.sh** | Test TLS integration (ca-certs + curl) | `[tsuku-binary]` | Host platform |
| **verify-tool.sh** | Run tool-specific functional tests | `<tool-name>` | Host platform |

---

### Batch Configuration (`.github/ci-batch-config.json`)

```json
{
  "batch_sizes": {
    "test-recipe": {
      "linux": {"default": {"max": 20, "min_batches": 5}, "rust": 3},
      "darwin": 20
    },
    "validate-golden-recipes": {
      "default": 20
    }
  }
}
```

**Tuning**:
- Increase `max` to run fewer, larger batches (faster but higher memory)
- Increase `min_batches` to ensure minimum parallelism
- Rust recipes capped at 3 (compilation is resource-intensive)

---

## Common Failure Patterns and Debugging

### Pattern 1: Network Errors During Plan Generation

**Symptom**: `eval` command times out or fails with "network error"

**Causes**:
- DNS resolution failure
- GitHub API rate limiting (use `GITHUB_TOKEN`)
- Mirror/registry unavailable

**Debug**:
```bash
# Check network connectivity
curl -I https://api.github.com

# Set GitHub token (required for version resolution)
export GITHUB_TOKEN="ghp_..."
tsuku eval <tool>

# Increase verbosity
tsuku eval <tool> --verbose
```

**Fix**:
- Provide valid `GITHUB_TOKEN` environment variable
- Check firewall/proxy settings
- Wait for rate limit to reset (1 hour)

---

### Pattern 2: Sandbox Runtime Not Available

**Symptom**: Sandbox test skipped with "Container runtime not available"

**Causes**:
- Docker or Podman not installed
- Runtime not in PATH
- Runtime permission issues

**Debug**:
```bash
docker --version    # Check Docker
podman --version    # Check Podman
tsuku install <tool> --sandbox --force  # See warning message
```

**Fix**:
- Install Docker: https://docs.docker.com/install
- Install Podman: https://podman.io/
- For Docker group membership: `sudo usermod -aG docker $USER` (security risk, prefer rootless)
- Configure rootless Docker: `dockerd-rootless-setuptool.sh install`

---

### Pattern 3: Golden File Mismatch

**Symptom**: CI fails "golden file validation" with diff shown

**Causes**:
- Recipe changed (intentional)
- tsuku code changed (intentional)
- Deterministic generation failed (bug)
- Platform-specific drift (time-based URLs)

**Debug**:
```bash
# Regenerate locally and compare
tsuku eval <tool>@<version> --os linux --arch amd64 > local.json
diff local.json testdata/golden/plans/.../<version>-linux-amd64.json

# Check what changed
git diff testdata/golden/plans/

# Use constrained evaluation to verify determinism
tsuku eval <tool>@<version> --pin-from testdata/golden/plans/.../...json > test.json
diff test.json testdata/golden/plans/.../<version>-...json
```

**Fix**:
- If intentional: Regenerate via Actions UI with `commit_back=true`
- If unintended: Find code change and revert

---

### Pattern 4: Checksum Mismatch During Installation

**Symptom**: Install fails with "checksum mismatch" or "file tampered"

**Causes**:
- Download corrupted
- File modified post-install
- Checksum stored incorrectly
- Race condition in multi-threaded download

**Debug**:
```bash
# Check state.json for stored checksums
cat ~/.tsuku/state.json | jq '.installed.<tool>.versions.<version>.binary_checksums'

# Compute checksum of installed binary
sha256sum ~/.tsuku/tools/<tool>-<version>/bin/<binary>

# Run verify to detect tampering
tsuku verify <tool>
```

**Fix**:
- Clear and reinstall: `tsuku remove <tool> --force && tsuku install <tool> --force`
- Check disk space: `df -h`
- Check file permissions: `chmod -R 755 ~/.tsuku/tools/<tool>-<version>`

---

### Pattern 5: Dependency Resolution Failed

**Symptom**: Install fails with exit code 8 (ExitDependencyFailed)

**Causes**:
- Dependency recipe not found
- Dependency version not available
- Circular dependency (rare)
- Dependency recipe broken

**Debug**:
```bash
# Check if dependency exists
tsuku eval <dependency-tool>

# Try installing dependency directly
tsuku install <dependency-tool> --force

# List what would be installed
tsuku eval <main-tool> --install-deps | jq '.dependencies'
```

**Fix**:
- Ensure dependency recipe exists in embedded or registry recipes
- Check dependency version is available
- Update main recipe's dependency version constraint
- Fix broken dependency recipe

---

### Pattern 6: Sandbox Installation Hangs

**Symptom**: Sandbox test times out (default: 10 minutes)

**Causes**:
- Slow network (download >10 minutes)
- Compilation very slow (especially Rust)
- Disk I/O bottleneck
- Infinite loop in build script

**Debug**:
```bash
# Check container build logs
docker logs <container-id>

# Run with verbose output
tsuku install <tool> --sandbox --force --verbose

# Check system resources
docker stats  # Watch memory/CPU usage
```

**Fix**:
- Increase timeout (if known to be slow): `timeout 1800 ./test/scripts/...`
- Optimize recipe (cache dependencies, parallelize build)
- Increase disk/memory for Docker: Settings → Resources
- Check Dockerfile for infinite loops

---

### Pattern 7: Verification Failed

**Symptom**: Install succeeds but verification fails (exit code 7 or exit code 1 with verify error)

**Causes**:
- Verification command doesn't exist in PATH
- Verification command fails (tool broken)
- Dependencies missing from container
- Binary incompatible with system

**Debug**:
```bash
# Check verification command syntax
tsuku validate <recipe> | grep -A2 verify

# Test verification manually in container
docker run --rm -it <base-image> /bin/bash
# Inside container: install tool, then run verify command

# Check what's on PATH
tsuku install <tool> --sandbox --force --verbose | grep PATH
```

**Fix**:
- Add missing dependencies to recipe's system requirements
- Fix verification command (must be in PATH after install)
- Ensure tool installs to a standard location
- Check tool is statically linked (or dependencies bundled)

---

### Pattern 8: Plan Generation Nondeterminism

**Symptom**: Running eval twice on same tool produces different plans

**Causes**:
- Dynamic URLs (version-specific mirrors)
- Time-based version resolution
- Ecosystem package manager nondeterminism
- LLM-generated dependency chains

**Debug**:
```bash
# Generate twice and compare
tsuku eval <tool> > plan1.json
tsuku eval <tool> > plan2.json
diff plan1.json plan2.json  # Should be empty

# Use deterministic-only flag
tsuku eval <tool> --require-embedded
```

**Fix**:
- Use golden file + `--pin-from` for deterministic output
- Specify exact version: `tsuku eval <tool>@<version>`
- Fix dynamic URLs in recipe (use version substitution)
- Implement constraint extraction for newly-supported ecosystems

---

### Pattern 9: Family-Specific Failures

**Symptom**: Test passes on Debian but fails on Alpine (or other family)

**Causes**:
- Different package managers (apt, dnf, apk, zypper, etc.)
- Different system dependencies
- libc differences (glibc vs musl)
- Package availability varies by family

**Debug**:
```bash
# Test locally across families
for family in debian rhel arch alpine suse; do
  echo "Testing $family..."
  tsuku install <tool> --sandbox --force \
    --env GITHUB_TOKEN="$GITHUB_TOKEN" || echo "FAILED: $family"
done

# Inspect container for missing packages
docker run --rm -it <alpine-image> apk search <package>
```

**Fix**:
- Add family-specific system requirements to recipe
- Use `apt_install`, `dnf_install`, etc. (instead of generic commands)
- Test against all 5 families before submitting PR
- Update container-images.json if base images need new packages

---

### Pattern 10: Sandbox Foundation Image Bloat

**Symptom**: Docker disk usage grows over time, foundation images accumulate

**Causes**:
- Old foundation images cached but unused
- Large dependency trees
- Many versions of same tool tested

**Debug**:
```bash
docker images | grep sandbox-foundation  # List all cached images
docker image inspect <image-id> | jq '.Size'  # Check size
```

**Fix**:
```bash
# Clean up old images
docker image prune -a

# Disable caching (for testing only)
docker image rm tsuku/sandbox-foundation:*

# Manual cleanup
docker rmi $(docker images --filter reference='tsuku/sandbox*' -q)
```

---

## Recipe Testing Best Practices

### Before Submitting a PR

1. **Validate**: `tsuku validate recipes/my-recipe.toml`
2. **Test locally**: `tsuku install my-tool --force` (or use --sandbox if dependencies)
3. **Test all families**: Loop through all 5 Linux families with sandbox
4. **Verify**: `tsuku verify my-tool`
5. **Clean up**: Remove `.tsuku-dev/` state between tests

### Local Testing Workflow

```bash
# Step 1: Build tsuku for development
make build

# Step 2: Validate recipe
./tsuku validate recipes/my-recipe.toml --check-libc-coverage

# Step 3: Generate plan (cross-platform)
./tsuku eval my-tool --os linux --arch amd64 > /tmp/plan-linux.json
./tsuku eval my-tool --os darwin --arch arm64 > /tmp/plan-macos.json

# Step 4: Test in sandbox (all families if applicable)
for family in debian rhel arch alpine suse; do
  echo "Testing $family..."
  ./tsuku eval my-tool --os linux --linux-family "$family" --arch amd64 | \
    ./tsuku install --plan - --sandbox --force --json || echo "FAILED"
done

# Step 5: Verify tool works
./tsuku verify my-tool

# Step 6: Clean up
rm -rf .tsuku-dev/  # Remove dev state
```

### CI Will Additionally Test

1. All platforms (x86_64, arm64) on Linux and macOS
2. Golden file validation (diff against expected)
3. Cross-platform consistency
4. Checksum pinning
5. Build essential dependencies
6. TLS certificate chain

---

## Summary

This catalog covers tsuku's complete recipe testing workflow from validation through CI. Recipe authors should:

1. **Validate** locally before each change
2. **Test cross-platform** using sandbox (all 5 Linux families for distribution-aware recipes)
3. **Understand exit codes** to debug failures efficiently
4. **Use golden files** as regression tests
5. **Watch CI** for full multi-platform validation

The testing pipeline ensures recipes are deterministic, reproducible, and compatible across all supported platforms and distributions.


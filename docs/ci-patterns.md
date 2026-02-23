# CI Workflow Patterns

This document describes the two standard patterns for structuring CI test jobs in tsuku's GitHub Actions workflows. Both patterns exist to reduce job count -- each CI job has fixed overhead (runner provisioning, checkout, Go setup) that adds up fast when you have dozens of independent tests.

## When to Use Each Pattern

**Container loop** -- Use when testing the same operation across Linux distribution families (debian, rhel, arch, alpine, suse). All variants run on the same `ubuntu-latest` runner using tsuku's sandbox mode, so there's no reason to spin up separate jobs.

**GHA group serialization** -- Use when running multiple independent tool tests on the same runner type. Each tool gets its own `$TSUKU_HOME` for isolation, but they share a download cache and the runner's Go toolchain.

If you're testing different runner types (`ubuntu-latest` vs `macos-latest` vs a container image), those are genuinely different environments and should be separate jobs.

## The Four Runner Types

Current workflows use these runner environments:

| Runner | Platform | Notes |
|--------|----------|-------|
| `ubuntu-latest` | linux-x86_64 (glibc) | Default for most tests |
| `macos-latest` | darwin-arm64 | Apple Silicon |
| `macos-14` | darwin-arm64 | Pinned Apple Silicon |
| `macos-15-intel` | darwin-x86_64 | Intel Mac |
| Container: `golang:1.23-alpine` | linux-x86_64 (musl) | For musl-specific tests |

Each genuinely different runner type warrants its own job. Within a single runner type, use the patterns below to batch tests.

## Pattern 1: Container Loop

Iterates over Linux distribution families, running the same test command in sandbox mode. Used when validating that recipes produce correct installation plans across families.

### Structure

```yaml
- name: Test across Linux families
  run: |
    FAMILIES=(debian rhel arch alpine suse)
    FAILED=()
    for family in "${FAMILIES[@]}"; do
      echo "::group::Test description on $family"
      timeout 300 bash -c '
        ./tsuku eval <tool> --os linux --linux-family '"$family"' --install-deps > plan.json
        ./tsuku install --plan plan.json --sandbox --force
      '
      EXIT_CODE=$?
      if [ "$EXIT_CODE" != "0" ]; then FAILED+=("$family"); fi
      echo "::endgroup::"
    done
    if [ ${#FAILED[@]} -gt 0 ]; then
      echo "::error::Failed families: ${FAILED[*]}"
      exit 1
    fi
```

### Key elements

- **Image array**: `FAMILIES=(debian rhel arch alpine suse)` defines the iteration targets
- **`timeout` wrapper**: Prevents a hung build from blocking the runner for the full job timeout
- **Exit code capture**: Store the result instead of failing immediately, so all families get tested
- **Failure array**: `FAILED+=("$family")` collects failures for a single error report at the end
- **`::group::`/`::endgroup::`**: Collapses each iteration in the GitHub Actions log viewer

### Real example

From `build-essentials.yml` (test-sandbox-cmake):

```yaml
- name: Test sandbox across Linux families
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
    TSUKU_REGISTRY_URL: https://raw.githubusercontent.com/${{ github.repository }}/${{ github.head_ref || github.ref_name }}
  run: |
    FAMILIES=(debian rhel arch alpine suse)
    FAILED=()
    for family in "${FAMILIES[@]}"; do
      echo "::group::Sandbox cmake on $family"
      timeout 300 bash -c '
        ./tsuku eval cmake --os linux --linux-family '"$family"' --install-deps > plan.json
        ./tsuku install --plan plan.json --sandbox --force
      '
      EXIT_CODE=$?
      if [ "$EXIT_CODE" != "0" ]; then FAILED+=("$family"); fi
      echo "::endgroup::"
    done
    if [ ${#FAILED[@]} -gt 0 ]; then
      echo "::error::Failed families: ${FAILED[*]}"
      exit 1
    fi
```

From `integration-tests.yml` (checksum-pinning):

```yaml
- name: Run checksum pinning tests across families
  run: |
    FAMILIES=(debian rhel arch alpine suse)
    FAILED=()
    for family in "${FAMILIES[@]}"; do
      echo "::group::Checksum pinning on $family"
      if ! timeout 300 ./test/scripts/test-checksum-pinning.sh "$family"; then
        FAILED+=("$family")
      fi
      echo "::endgroup::"
    done
    if [ ${#FAILED[@]} -gt 0 ]; then
      echo "::error::Failed families: ${FAILED[*]}"
      exit 1
    fi
    echo "All checksum pinning tests passed"
```

## Pattern 2: GHA Group Serialization

Runs multiple tool-level tests sequentially within a single job. Each test gets its own isolated `$TSUKU_HOME` but shares a download cache directory to avoid re-downloading common archives.

### Structure

```yaml
- name: Run all tests
  run: |
    TOOLS=(tool-a tool-b tool-c)
    CACHE_DIR="${{ runner.temp }}/tsuku-cache/downloads"
    mkdir -p "$CACHE_DIR"
    FAILED=()

    for tool in "${TOOLS[@]}"; do
      echo "::group::Testing $tool"

      # Fresh TSUKU_HOME per tool with shared download cache
      export TSUKU_HOME="${{ runner.temp }}/tsuku-$tool"
      mkdir -p "$TSUKU_HOME/cache"
      ln -s "$CACHE_DIR" "$TSUKU_HOME/cache/downloads"
      export PATH="$TSUKU_HOME/bin:$PATH"

      if ! timeout 300 bash -c '
        ./tsuku install --force "'"$tool"'" && \
        ./test/scripts/verify-tool.sh "'"$tool"'"
      '; then
        FAILED+=("$tool")
      fi
      echo "::endgroup::"
    done

    if [ ${#FAILED[@]} -gt 0 ]; then
      echo "::error::Failed tools: ${FAILED[*]}"
      exit 1
    fi
    echo "All tests passed"
```

### Key elements

- **Per-test `$TSUKU_HOME`**: Each tool installs into its own directory. This prevents one tool's state from affecting another.
- **Shared download cache**: `ln -s "$CACHE_DIR" "$TSUKU_HOME/cache/downloads"` lets tools share downloaded archives. If tool-b depends on zlib and tool-a already downloaded it, tool-b won't re-download.
- **`::group::`/`::endgroup::`**: Same log grouping as the container loop pattern.
- **Failure collection**: Same `FAILED+=()` approach -- run everything, report at the end.

### Real example

From `build-essentials.yml` (test-homebrew-linux):

```yaml
- name: Test all homebrew tools
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
    TSUKU_REGISTRY_URL: https://raw.githubusercontent.com/${{ github.repository }}/${{ github.head_ref || github.ref_name }}
  run: |
    # make excluded - see #1581
    # pngcrush tests dependency chain: pngcrush -> libpng -> zlib
    TOOLS=(pkg-config cmake gdbm pngcrush)
    CACHE_DIR="${{ runner.temp }}/tsuku-cache/downloads"
    mkdir -p "$CACHE_DIR"
    FAILED=()

    for tool in "${TOOLS[@]}"; do
      echo "::group::Testing $tool"

      # Fresh TSUKU_HOME per tool with shared download cache
      export TSUKU_HOME="${{ runner.temp }}/tsuku-$tool"
      mkdir -p "$TSUKU_HOME/cache"
      ln -s "$CACHE_DIR" "$TSUKU_HOME/cache/downloads"
      export PATH="$TSUKU_HOME/bin:$PATH"

      if ! timeout 300 bash -c '
        ./tsuku install --force "'"$tool"'" && \
        ./test/scripts/verify-tool.sh "'"$tool"'" && \
        ./test/scripts/verify-binary.sh "'"$tool"'"
      '; then
        FAILED+=("$tool")
      fi
      echo "::endgroup::"
    done

    if [ ${#FAILED[@]} -gt 0 ]; then
      echo "::error::Failed tools: ${FAILED[*]}"
      exit 1
    fi
    echo "All homebrew tools passed"
```

### JSON matrix variant

When test parameters come from a JSON matrix file (like `test-matrix.json`), the loop reads from JSON instead of a bash array. From `test.yml` (integration-linux):

```yaml
- name: Run all Linux integration tests
  run: |
    FAILED=()
    TESTS='${{ needs.matrix.outputs.linux }}'
    CACHE_DIR="${{ runner.temp }}/tsuku-cache/downloads"
    mkdir -p "$CACHE_DIR"

    while IFS= read -r item; do
      tool=$(echo "$item" | jq -r '.tool')
      desc=$(echo "$item" | jq -r '.desc')
      recipe=$(echo "$item" | jq -r '.recipe // empty')

      echo "::group::Testing $tool ($desc)"

      export TSUKU_HOME="${{ runner.temp }}/tsuku-$tool"
      mkdir -p "$TSUKU_HOME/cache"
      ln -s "$CACHE_DIR" "$TSUKU_HOME/cache/downloads"

      if [ -n "$recipe" ]; then
        if ! ./tsuku install --force --recipe "$recipe"; then
          FAILED+=("$tool")
        fi
      else
        if ! ./tsuku install --force "$tool"; then
          FAILED+=("$tool")
        fi
      fi
      echo "::endgroup::"
    done < <(echo "$TESTS" | jq -c '.[]')

    if [ ${#FAILED[@]} -gt 0 ]; then
      echo "::error::Failed tools: ${FAILED[*]}"
      exit 1
    fi
```

## Job Count Impact

Before consolidation, each tool test and each family variant ran as a separate GHA job. The worst-case PR (touching Go code, recipes, and test scripts) created ~87 jobs. After applying these patterns:

| PR type | Before | After |
|---------|--------|-------|
| Worst-case (Go + recipes + scripts) | ~87 jobs | ~46 jobs |
| Typical Go-only | ~29 jobs | ~13 jobs |

The reduction comes from batching N tool tests into 1 job per runner type, and batching M family tests into 1 job per tool. Fixed per-job overhead (runner spin-up, checkout, Go setup) is paid once instead of N or M times.

## Adding a New Test

### New tool on an existing runner

Add the tool to the relevant serialized loop. For example, to add a new homebrew tool test on Linux, add it to the `TOOLS` array in `build-essentials.yml`'s `test-homebrew-linux` job:

```yaml
TOOLS=(pkg-config cmake gdbm pngcrush your-new-tool)
```

### New family variant for an existing test

Add the family to the relevant `FAMILIES` array. Most container loop jobs already test all five families (debian, rhel, arch, alpine, suse).

### Genuinely new runner type

If you need a runner that doesn't match any existing job (for example, a Windows runner or a specific container image), create a new job. But batch any tests that will run on that runner into a single job using the GHA group serialization pattern.

### What NOT to do

Don't add a new `strategy.matrix` entry to fan out tests that could run sequentially on the same runner. The matrix strategy is appropriate when tests need genuinely different environments (different OS, different architecture). It's not appropriate for "test tool A" and "test tool B" on the same `ubuntu-latest` runner.

# CI Platform Filtering by Recipe `platforms` Metadata

## 1. Current Workflow Analysis

The workflow at `.github/workflows/test-changed-recipes.yml` has three jobs:

1. **matrix** (ubuntu-latest): Detects changed recipes via `git diff`, builds two outputs:
   - `recipes`: JSON array of `{tool, path, linux_only}` objects for all changed recipes
   - `macos_recipes`: JSON array of tool names for recipes that aren't Linux-only

2. **test-linux**: Matrix job -- one runner per recipe. Runs on `ubuntu-latest` (x86_64 only). Builds tsuku from source, runs `tsuku install --force <tool>`.

3. **test-macos**: Single job running on `macos-latest` (arm64). Iterates over all macOS-compatible recipes sequentially in a loop.

### Current platform filtering

The workflow already does basic OS filtering using shell `grep`:
- Checks `supported_os = ["linux"]` in the TOML to detect Linux-only recipes
- Checks `os_mapping` for legacy recipes that lack darwin mappings
- Sets `linux_only=true` and excludes from `macos_recipes`

This filtering is **coarse-grained**: it only distinguishes Linux vs macOS at the OS level. It does not filter by architecture (e.g., x86_64-only vs arm64) or by libc (glibc vs musl). There's no linux-arm64 runner either.

### Runner platform mapping

| Runner | Platform tuple |
|--------|---------------|
| `ubuntu-latest` | `linux-glibc-x86_64` |
| `macos-latest` | `darwin-arm64` |

The workflow currently tests 2 of the 5 batch pipeline platforms. Missing: `linux-glibc-arm64`, `linux-musl-x86_64`, `darwin-x86_64`.

## 2. CLI Metadata Capabilities

### `tsuku info --json --metadata-only`

Already exists and outputs `supported_platforms` as a computed array:

```json
{
  "supported_platforms": [
    {"os": "darwin", "arch": "amd64"},
    {"os": "darwin", "arch": "arm64"},
    {"os": "linux", "arch": "amd64", "linux_family": "debian"}
  ]
}
```

The `--metadata-only` flag skips installation state and dependency resolution, making it fast for CI queries. It also supports `--recipe <path>` to load a local file.

### `tsuku info --json --recipe <path> --metadata-only`

This is the exact command CI would need. It loads the recipe from a file path, computes supported platforms, and outputs JSON without touching `$TSUKU_HOME` state.

### The `platforms` metadata field

The `platforms` field described in the prompt (`platforms = ["linux-glibc-x86_64", ...]`) does **not yet exist** in the recipe TOML schema. The `Metadata` struct in `internal/recipe/types.go` has `SupportedOS`, `SupportedArch`, `SupportedLibc`, and `UnsupportedPlatforms` but no `platforms` field.

The batch pipeline presumably writes `platforms` as a convenience field that flattens all constraints into explicit platform tuples. Two approaches for CI:

1. **Read the `platforms` field directly** (simple grep/tomlq) once it's added to the schema
2. **Use `tsuku info --json --metadata-only`** which already computes `supported_platforms` from the existing constraint fields

## 3. Industry Patterns

**Homebrew**: Uses `bottle do` blocks with per-OS/arch SHA declarations. CI skips bottle builds for platforms not listed. The `depends_on` DSL includes `:linux` and `:macos` filters.

**Conda**: Uses `# [linux]`, `# [osx]`, `# [aarch64]` selectors as YAML comments. The CI build matrix reads these selectors to skip irrelevant platform builds.

**GitHub Actions matrix filtering**: Two standard patterns:
- **Pre-filter**: Build the matrix JSON in a setup job, only including valid entries. This avoids spinning up unnecessary runners.
- **Post-filter with `if`**: Include all entries but skip steps with `if: contains(matrix.platforms, 'linux')`. This still starts runners but exits early.

Pre-filtering is strictly better for cost/time since it avoids runner allocation entirely.

## 4. Options Analysis

### Option A: Shell-based TOML parsing in workflow

Extract the `platforms` field with grep/sed or `yq`/`tomlq`.

```bash
# If platforms field exists, parse it
platforms=$(grep '^platforms' "$path" | sed 's/.*\[//;s/\].*//' | tr -d '"' | tr ',' '\n')
if echo "$platforms" | grep -q "linux-glibc-x86_64"; then
  # include in linux matrix
fi
```

**Pros:**
- No CLI changes needed
- Zero build step -- runs in seconds
- Works with the proposed `platforms` field directly

**Cons:**
- Fragile: TOML parsing with grep is error-prone (multiline values, comments, etc.)
- Duplicates platform logic already in the CLI
- Requires `platforms` field to be added to the schema first
- Falls apart if `platforms` is absent (universal recipe) -- needs fallback logic

**Complexity:** Low implementation, medium maintenance burden.

### Option B: `tsuku info --json --metadata-only` in workflow

Build tsuku in the matrix job, then query each changed recipe:

```bash
for path in $CHANGED; do
  tool=$(basename "$path" .toml)
  platforms=$(./tsuku info --json --recipe "$path" --metadata-only | jq -r '.supported_platforms[] | "\(.os)-\(.arch)"')
  # Filter based on runner platform
done
```

**Pros:**
- Uses the authoritative platform computation (same logic as `tsuku install`)
- Already works today -- no schema changes needed
- Handles all constraint types (supported_os, supported_arch, unsupported_platforms, libc)
- Single source of truth; adding new constraint fields automatically works in CI

**Cons:**
- Requires building tsuku in the matrix detection job (adds ~15-30s)
- The matrix job already runs on ubuntu-latest with Go available, so this isn't a new dependency
- Slightly more complex than a grep

**Complexity:** Low. The infrastructure already exists.

### Option C: Per-platform skip conditions

Keep the current matrix but add an early-exit step per job:

```yaml
- name: Check platform support
  id: check
  run: |
    platforms=$(./tsuku info --json --recipe "${{ matrix.recipe.path }}" --metadata-only | jq ...)
    if ! echo "$platforms" | grep -q "$RUNNER_PLATFORM"; then
      echo "skip=true" >> $GITHUB_OUTPUT
    fi

- name: Install
  if: steps.check.outputs.skip != 'true'
  run: ./tsuku install --force ${{ matrix.recipe.tool }}
```

**Pros:**
- Minimal workflow restructuring
- Easy to understand

**Cons:**
- Still allocates runners for skipped recipes (costs time and money)
- macOS runners are expensive; starting one just to skip is wasteful
- Each skipped Linux job still takes ~1 min for checkout + Go setup + build

**Complexity:** Very low, but wasteful.

### Option D: Separate matrices per platform

Split into `test-linux-x86_64`, `test-linux-arm64`, `test-macos-arm64` jobs, each with their own filtered recipe list.

**Pros:**
- Clean separation
- Easy to add new platforms later
- Each job only contains recipes it will actually test

**Cons:**
- Significant workflow restructuring
- The matrix job output multiplies (one list per platform)
- Currently there's no linux-arm64 runner in use, so this is premature
- Duplicates job definitions (checkout, Go setup, build) across jobs

**Complexity:** High restructuring for marginal benefit over Option B.

## 5. Recommendation

**Option B (`tsuku info --json --metadata-only`) is the clear winner.**

Rationale:

1. **Already works today.** No schema changes, no new CLI features. The `info` command with `--json --recipe --metadata-only` flags outputs `supported_platforms` computed from all existing constraint fields.

2. **Single source of truth.** The platform resolution logic lives in Go code (`SupportedPlatforms()` in `internal/recipe/policy.go`). Shell-based TOML parsing would duplicate this logic and inevitably diverge.

3. **Cost is minimal.** The matrix job already has Go installed. Building tsuku takes ~10 seconds with Go module caching. Querying metadata per recipe is instantaneous.

4. **Forward-compatible.** When the `platforms` field is added to the schema, the `SupportedPlatforms()` function can incorporate it, and CI needs no changes.

### Implementation sketch

In the `matrix` job's "Get changed recipes" step, after the existing loop that builds `JSON` and `MACOS_JSON`:

1. Add a Go build step before recipe detection (or move it earlier)
2. Replace the `grep`-based `linux_only` detection with:
   ```bash
   platforms=$(./tsuku info --json --recipe "$path" --metadata-only 2>/dev/null | jq -r '[.supported_platforms[] | "\(.os)-\(.arch)"] | join(",")')
   ```
3. Check `echo "$platforms" | grep -q "darwin"` for macOS inclusion
4. Check `echo "$platforms" | grep -q "linux"` for Linux inclusion
5. Store the platforms string in the matrix entry for potential future per-arch filtering

This replaces ~10 lines of fragile grep with one CLI call per recipe, and removes the divergence risk between CI filtering and runtime platform checks.

### When the `platforms` field ships

Once the batch pipeline writes `platforms = [...]` into recipe TOML and the Go struct supports it, no CI changes are needed -- `SupportedPlatforms()` will use the field, and the `info --json` output will reflect it automatically.

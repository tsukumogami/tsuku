# Issue 1864 Plan

## Approach

Create `.github/workflows/test-recipe.yml` as a `workflow_dispatch` + `pull_request` workflow that cross-compiles tsuku in a `build` job, uploads binaries as artifacts, then runs four parallel jobs (Linux x86_64, Linux arm64, macOS arm64, macOS x86_64). Linux jobs use Docker containers to cover all five Linux families. Failures are soft (`continue-on-error: true`) — the summary tells contributors which `when` filters to add. Library recipes are NOT skipped (this is intentional divergence from `test-changed-recipes.yml`).

## Reference Patterns

### Action SHA Pins (from existing workflows)

```yaml
actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd  # v6.0.2
actions/setup-go@7a3fe6cf4cb3a834922a1244abfce67bcef6a0c5  # v6.2.0
actions/upload-artifact@b7c566a772e6b6bfb58ed0dc250532a479d7789f  # v6.0.0
actions/download-artifact@37930b1c2abaa49bbe596cd826c3c89aef350131  # v7.0.0
actions/cache@b7e8d49f17405cc70c1c120101943203c98d3a4b  # v5.0.3
```

### Cross-Compilation (from batch-generate.yml)

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64  go build -o tsuku-linux-amd64  ./cmd/tsuku
CGO_ENABLED=0 GOOS=linux GOARCH=arm64  go build -o tsuku-linux-arm64  ./cmd/tsuku
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o tsuku-darwin-arm64 ./cmd/tsuku
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o tsuku-darwin-amd64 ./cmd/tsuku
```

### Artifact Names (from batch-generate.yml)

- Binary artifact: `tsuku-binaries` — contains `tsuku-linux-amd64`, `tsuku-linux-arm64`, `tsuku-darwin-arm64`, `tsuku-darwin-amd64`
- `retention-days: 1`

### Docker Image Mapping (from batch-generate.yml + validate-golden-execution.yml)

| Family | Image |
|--------|-------|
| debian | `debian:bookworm-slim` |
| rhel   | `fedora:41` |
| arch   | `archlinux:base` (x86_64 only — no arm64 image) |
| alpine | `alpine:3.21` |
| suse   | `opensuse/tumbleweed` |

### Per-Family Package Install (from batch-generate.yml)

```bash
case '$family' in
  alpine) apk add --no-cache curl bash ca-certificates jq ;;
  debian) apt-get update && apt-get install -y --no-install-recommends curl ca-certificates jq ;;
  rhel)   dnf install -y --setopt=install_weak_deps=False curl ca-certificates jq ;;
  arch)   pacman -Sy --noconfirm curl ca-certificates jq ;;
  suse)   zypper -n install curl ca-certificates jq ;;
esac
```

### Docker Invocation Pattern (from batch-generate.yml)

```bash
docker run --rm \
  -v "$PWD:/workspace" \
  -w /workspace \
  "$image" \
  sh -c "
    <install deps>
    timeout 300 ./tsuku-linux-amd64 install --force --recipe '$recipe_path' > /workspace/.tsuku-output.json 2>&1
    echo \$? > /workspace/.tsuku-exit-code
  " 2>&1 || true

EXIT_CODE=$(cat .tsuku-exit-code 2>/dev/null || echo 1)
rm -f .tsuku-exit-code .tsuku-output.json
```

### TSUKU_HOME Isolation (from test-changed-recipes.yml + validate-golden-execution.yml)

```bash
# Linux (per-run or per-recipe)
export TSUKU_HOME="${{ runner.temp }}/tsuku-$tool"
mkdir -p "$TSUKU_HOME"

# Inside Docker (validate-golden-execution.yml pattern)
export TSUKU_HOME="/tmp/tsuku-$recipe_name"
mkdir -p "$TSUKU_HOME"
```

### macOS Pattern (from batch-generate.yml)

- `macos-14` for arm64 runner
- `macos-15-intel` for x86_64 runner
- Requires `brew install coreutils` for `gtimeout`
- Binary: `./tsuku-darwin-arm64` or `./tsuku-darwin-amd64`
- Command: `gtimeout 300 ./tsuku-darwin-arm64 install --force --recipe "$recipe_path"`

### PR Recipe Detection (from test-changed-recipes.yml)

```bash
# fetch-depth: 0 required on checkout
CHANGED=$(git diff --name-only --diff-filter=d origin/main...HEAD -- 'recipes/**/*.toml')
# Recipe path resolution from name:
FIRST="${recipe:0:1}"
recipe_path="recipes/$FIRST/$recipe.toml"
```

Unlike `test-changed-recipes.yml`, do NOT skip `type = "library"` recipes.

### Install Command

```bash
./tsuku install --force --recipe "$recipe_path"
```

(No `--json` flag needed; this workflow just needs pass/fail, not structured output.)

### Runner Selection

| Job | Runner |
|-----|--------|
| build | ubuntu-latest |
| linux-x86_64 | ubuntu-latest |
| linux-arm64 | ubuntu-24.04-arm |
| darwin-arm64 | macos-14 |
| darwin-x86_64 | macos-15-intel |

### Job Summary Pattern (from batch-generate.yml)

```bash
echo "### Results" >> "$GITHUB_STEP_SUMMARY"
echo "| Family | Status |" >> "$GITHUB_STEP_SUMMARY"
echo "|--------|--------|" >> "$GITHUB_STEP_SUMMARY"
```

## File Changes

- `.github/workflows/test-recipe.yml` (NEW)

## Implementation Steps

1. Define triggers: `workflow_dispatch` with `recipe` string input; `pull_request` on `recipes/**/*.toml` and `.github/workflows/test-recipe.yml`.

2. Create `build` job on `ubuntu-latest`:
   - `actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd`
   - `actions/setup-go@7a3fe6cf4cb3a834922a1244abfce67bcef6a0c5` with `go-version-file: go.mod`
   - Cross-compile four binaries with `CGO_ENABLED=0`.
   - Upload artifact `tsuku-binaries` (`retention-days: 1`) containing all four binaries.
   - Detect recipe from input or PR diff; output `recipe_name` and `recipe_path` for downstream jobs.
   - For PR trigger: `git diff --name-only --diff-filter=d origin/main...HEAD -- 'recipes/**/*.toml'`. If no recipe found, set a `has_recipe=false` output and let downstream jobs skip cleanly.

3. Create `test-linux-x86_64` job (`ubuntu-latest`, `needs: build`, `if: needs.build.outputs.has_recipe == 'true'`):
   - Download `tsuku-binaries` artifact.
   - `chmod +x tsuku-linux-amd64`
   - Loop over five families (debian, rhel, arch, alpine, suse) with their Docker images.
   - For each family: `docker run` to install deps then run `tsuku install --force --recipe`.
   - Capture per-family pass/fail; write job summary table.
   - Use `TSUKU_HOME=/tmp/tsuku-$recipe_name` inside the container.

4. Create `test-linux-arm64` job (`ubuntu-24.04-arm`, `needs: build`):
   - Same structure as x86_64 but uses `tsuku-linux-arm64` and omits `archlinux:base` (no arm64 image).
   - Four families: debian, rhel, suse, alpine.

5. Create `test-darwin-arm64` job (`macos-14`, `needs: build`):
   - Download `tsuku-binaries`.
   - `brew install coreutils` for `gtimeout`.
   - `chmod +x tsuku-darwin-arm64`
   - `export TSUKU_HOME="${{ runner.temp }}/tsuku-$recipe_name"`
   - `gtimeout 300 ./tsuku-darwin-arm64 install --force --recipe "$recipe_path"`
   - Write job summary.

6. Create `test-darwin-x86_64` job (`macos-15-intel`, `needs: build`):
   - Same as arm64 job but uses `tsuku-darwin-amd64`.

7. For all test jobs, add to job summary: "Platform failures indicate which `when` filters to add to the recipe. They are not required fixes before merge." Use `continue-on-error: true` on any inner loops, but let the job itself fail if any family fails, so the PR shows red — reviewers can then decide.

8. Add inline comments throughout explaining the matrix structure, why library recipes are included, and the `when` filter guidance.

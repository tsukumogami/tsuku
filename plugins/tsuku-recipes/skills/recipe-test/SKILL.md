---
name: recipe-test
description: |
  Full recipe testing workflow: validation, plan evaluation, sandbox
  testing, golden file regression, and failure debugging. Use when
  testing new recipes, debugging install failures, or validating
  changes before a PR.
---

## Phase 1: Validate

Check syntax, required fields, and security rules.

```bash
tsuku validate <recipe.toml>
tsuku validate <recipe.toml> --strict              # warnings become errors
tsuku validate <recipe.toml> --check-libc-coverage  # flag missing musl support
```

Exit codes: 0 = valid, 1 = invalid, 2 = bad usage.

## Phase 2: Evaluate

Generate a resolved installation plan (JSON) to inspect before installing.

```bash
tsuku eval --recipe <recipe.toml>
tsuku eval --recipe <recipe.toml> --version <version>
tsuku eval --recipe <recipe.toml> --os darwin --arch arm64
```

For registry or embedded recipes:

```bash
tsuku eval <tool>@<version>
tsuku eval <tool> --os linux --linux-family alpine --arch amd64
tsuku eval <tool> --install-deps     # auto-install eval-time deps
tsuku eval <tool> --require-embedded # fail if LLM fallback needed
```

Pipe to `jq` for inspection: `tsuku eval <tool> | jq .`

Exit codes: 0 = success, 3 = recipe not found, 4 = version not found, 5 = network error, 8 = dependency failed.

## Phase 3: Sandbox Install

Test in an isolated container without touching the host.

```bash
tsuku install --recipe <recipe.toml> --sandbox --force
tsuku install <tool> --sandbox --force                          # embedded/registry
tsuku install <tool> --sandbox --force --env GITHUB_TOKEN="$GITHUB_TOKEN"
```

From a plan:

```bash
tsuku eval <tool> | tsuku install --plan - --sandbox --force
```

### Cross-Family Testing

Supported families: debian, rhel, alpine, arch, suse.

```bash
for family in debian rhel arch alpine suse; do
  echo "Testing $family..."
  tsuku eval <tool> --os linux --linux-family "$family" --arch amd64 | \
    tsuku install --plan - --sandbox --force --json || echo "FAILED: $family"
done
```

Requires Docker or Podman. JSON output (`--json`) returns `passed`, exit codes, stdout, stderr, and duration.

## Phase 4: Golden File Validation

Golden files are stored plans used as regression tests. CI compares freshly generated plans against these files.

**Paths:**
- Embedded: `testdata/golden/plans/embedded/<recipe>/<version>-<os>[-<family>]-<arch>.json`
- Registry: `testdata/golden/plans/<first-letter>/<recipe>/<version>-<os>[-<family>]-<arch>.json`

**Regenerate for a recipe:**

```bash
./scripts/regenerate-golden.sh <recipe> --os linux --arch amd64
./scripts/regenerate-golden.sh <recipe> --os darwin --arch arm64
```

Or use the GitHub Actions workflow: Actions > Generate Golden Files > enter recipe name > check "commit_back" > Run.

**Verify determinism with constrained eval:**

```bash
tsuku eval <tool>@<version> --pin-from testdata/golden/plans/.../<file>.json > test.json
diff test.json testdata/golden/plans/.../<file>.json
```

When to regenerate: after intentional recipe or code changes that affect plan output. If CI shows a golden file diff you didn't expect, investigate before accepting.

## Test Infrastructure

| Tool | Purpose |
|------|---------|
| `make build-test` | Build `tsuku-test` binary with `$TSUKU_HOME` set to `.tsuku-test` |
| `tsuku doctor` | Check environment health |
| `TSUKU_HOME=/tmp/test-home` | Isolate testing from real installs |
| `TSUKU_REGISTRY_URL=<url>` | Point at a local or branch registry |

**Local testing setup:**

```bash
make build-test
export TSUKU_HOME=$(mktemp -d)
./tsuku-test validate recipes/my-recipe.toml
./tsuku-test install my-tool --sandbox --force
```

## Common Failure Patterns

**Exit code 3 -- recipe not found.** Recipe doesn't exist in embedded or registry recipes. Check the name matches a file in `recipes/` or an embedded recipe.

**Exit code 4 -- version not found.** Version provider can't resolve the requested version. Check the `[version]` source in the recipe, verify the tag exists upstream.

**Exit code 5 -- network error.** GitHub API rate limiting is the most common cause. Set `GITHUB_TOKEN` and retry.

**Exit code 8 -- dependency failed.** A required dependency isn't available or its recipe is broken. Test the dependency in isolation: `tsuku eval <dep>`.

**Sandbox won't start (exit code 12).** Docker/Podman not running or not in PATH. Run `docker --version` or `podman --version` to confirm.

**Verification fails after successful install.** The verify command can't find the binary. Check that the recipe's `verify.command` matches the installed binary name and that it lands on PATH.

**"patchelf not found" warning.** A dependency is missing and RPATH won't be fixed. The recipe needs patchelf as a dependency, or the tool needs static linking.

**Golden file mismatch in CI.** Regenerate locally and diff. Use `--pin-from` to test determinism. If the change is intentional, regenerate via the Actions workflow with `commit_back` enabled.

**Family-specific failures (passes on Debian, fails on Alpine).** Usually a glibc/musl difference or missing package. Test all five families before submitting. Check that family-specific install actions (`apt_install`, `apk_install`, etc.) cover every target.

## Further Reading

See CONTRIBUTING.md for full CI patterns, test scripts in `test/scripts/`, and the recipe authoring guide.

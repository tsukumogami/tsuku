## Goal

Run the same verification logic during `tsuku install` that `tsuku verify` uses, failing the installation if verification fails.

## Context

Currently `tsuku install` succeeds after downloading, extracting, and symlinking binaries without verifying the installation actually works. Users only discover broken installations when running `tsuku verify` manually. For example, a recipe with a broken verify command like `kubectl --version` (which exits with error) passes batch CI because verification is never run.

Both tools and libraries should be verified before reporting success. The verification paths must be shared with `tsuku verify` to avoid code duplication and ensure both commands validate the same things.

**Implementation notes:**
- Current verification functions in `cmd/tsuku/verify.go` call `exitWithCode()` directly; refactor to return errors for reuse
- For tools: run the same verification as `tsuku verify` (verify command execution, integrity verification, dependency validation, PATH checks)
- For libraries: run Tiers 1-3 (header validation, dependency checking, dlopen); skip Tier 4 (integrity) which is opt-in via `--integrity` flag
- Entry points: `installWithDependencies()` at `install_deps.go:543`, `installLibrary()` at `install_lib.go:165`

## Acceptance Criteria

- [ ] `tsuku install <tool>` runs the same verification as `tsuku verify` before printing "Installation successful!"
- [ ] `tsuku install <library>` runs library verification (Tiers 1-3) before printing success
- [ ] Installation fails with appropriate error if verification fails
- [ ] `tsuku verify` and post-install verification share the same code paths (no duplication)

## Validation

```bash
#!/usr/bin/env bash
set -euo pipefail

# Test: install a tool and verify it shows verification output
tsuku install jq --force 2>&1 | grep -q "Verif" || echo "Should show verification output"

# Test: verify command failure causes install failure
cat <<'RECIPE' > /tmp/broken-verify.toml
[metadata]
name = "broken-verify-test"
description = "Test recipe with broken verify"

[version]
source = "static"
static_version = "1.0.0"

[[steps]]
action = "run_command"
command = "echo 'hello' > hello.txt"

[[steps]]
action = "install_binaries"
binaries = []

[verify]
command = "false"
RECIPE

# This should fail due to verification
if tsuku install --recipe /tmp/broken-verify.toml broken-verify-test 2>&1; then
  echo "FAIL: install should have failed due to broken verify command"
  exit 1
fi
echo "PASS: install failed as expected"
```

## Dependencies

None

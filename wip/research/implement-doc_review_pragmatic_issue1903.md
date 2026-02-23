# Review: #1903 (ci: add Renovate config and drift-check CI job)

**Focus**: pragmatic (simplicity, YAGNI, KISS)

## Files Changed

- `renovate.json` (new, 13 lines)
- `.github/workflows/drift-check.yml` (new, 162 lines)

## Findings

### Advisory 1: Duplicated find-grep-filter loop in drift-check.yml

`.github/workflows/drift-check.yml:110-142` -- The three `while IFS= read -r file` loops (workflows, Go, shell) are identical except for the `find` command. Could be refactored into a function or a single `find` with multiple `-name` patterns:

```bash
find . \( -path '.github/workflows' -name '*.yml' -o -name '*.go' -o -name '*.sh' \) \
  -not -path './vendor/*' -type f
```

Not blocking because it's a single-use CI script with bounded scope. The duplication won't compound.

### Advisory 2: Comment-matching exceptions are fragile

`.github/workflows/drift-check.yml:68-74` -- The exceptions for Go and YAML comment lines use regex patterns like `':\s*//'` and `'//.*debian:'`. These will miss some edge cases (e.g., a string literal that contains `//` before the image name). However, the failure mode is a false negative (missing a real hardcoded reference), not a false positive (blocking a valid PR), so the risk is low. The allowlist approach with explicit file:pattern exceptions (lines 80-90) is more reliable and should be preferred for known cases.

### No Blocking Findings

Both files are straightforward implementations of the design doc's Phase 3 spec. The Renovate config is minimal (13 lines), and the drift-check job does exactly two things: verify embedded copy freshness and grep for hardcoded references. No dead code, no speculative generality, no unnecessary abstractions.

The exception list in the drift-check job corresponds to real hits in the codebase (`DefaultValidationImage` in `internal/validate/executor.go`, `FROM debian:` in `scripts/test-zig-cc.sh`). Each exception has an explanatory comment.

The `opensuse/tumbleweed` entry in `container-images.json` won't match the Renovate regex (no colon-separated version tag), which is the correct behavior for a rolling release. This is documented in the implementation's key decisions.

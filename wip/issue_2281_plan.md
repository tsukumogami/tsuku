# Implementation Plan — Issue #2281

## Scope

Curate 5 already-handcrafted security-scanner recipes:

- `recipes/t/trivy.toml` — `github_archive`, `aquasecurity/trivy`
- `recipes/g/grype.toml` — `github_archive`, `anchore/grype`
- `recipes/c/cosign.toml` — `sigstore/cosign`
- `recipes/s/syft.toml` — `github_archive`, `anchore/syft`
- `recipes/t/tflint.toml` — `github_archive`, `terraform-linters/tflint`

## Approach

Per recipe, parallelizable across tools:

1. Read the current recipe and note the asset pattern, `arch_mapping`, `os_mapping`, and platform coverage.
2. Fetch the latest upstream release via WebFetch or `gh api repos/<owner>/<repo>/releases/latest` and enumerate every asset filename.
3. Cross-reference recipe asset patterns against actual published filenames for each of linux/amd64, linux/arm64, darwin/amd64, darwin/arm64. Flag any mismatches.
4. If platform support is incomplete upstream (e.g., no Intel Mac binary published), document with an inline comment and use `unsupported_platforms` or `supported_libc` as appropriate.
5. Add `curated = true` to `[metadata]`.
6. Run `tsuku validate --strict --check-libc-coverage recipes/<path>` — must return 0.
7. Run `tsuku eval --recipe <path> --os <os> --arch <arch>` for each target platform to sanity-check URL resolution.
8. Update `docs/curated-tools-priority-list.md`: no change needed — these are already marked "handcrafted", and they already pass validation.

## Parallelization

The 5 tools are independent. I'll validate each with a background agent and collect fix recommendations; then I'll apply the fixes in one commit per tool (or one combined commit if all trivially pass).

## Risks

- Upstream asset naming may have changed since last recipe update — same class of issue that eza/ripgrep hit in #2267. Parallel validation agents will catch these.
- Checksum coverage: current recipes may not use `checksum_url`. Best-effort check; add if upstream publishes `_checksums.txt` at a predictable URL.
- tflint has binaries under `terraform-linters/tflint` with `_linux_amd64.zip` (zip archive); confirm `github_archive` handles zip correctly.

## Testing

- `tsuku validate --strict --check-libc-coverage` for each recipe (required, CI enforces).
- `tsuku eval` spot checks across all 4 platforms.
- Nightly curated workflow picks up all `curated = true` recipes automatically — no test-matrix.json edit required.

## Commit Strategy

One commit per tool if fixes are substantive; one combined commit if all 5 trivially pass. Either way, PR body summarizes the per-tool outcome.

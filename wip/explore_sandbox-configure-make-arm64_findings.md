# Explore Findings: sandbox-configure-make-arm64

## Summary

The issue is well-understood with confirmed root cause. No design work is needed. Three targeted fixes cover all acceptance criteria.

## Confirmed Facts

**Bottle gap (root cause):**
- `pkgconf` — 0 `arm64_linux` entries in GHCR
- `make` — 0 `arm64_linux` entries in GHCR
- `zig` — not affected; uses direct arch_mapping download, no homebrew step

**Recipe system:**
- `WhenClause.Arch` exists (`internal/recipe/types.go:285`) and is evaluated in `Matches()` — arch-gated steps are already supported, just unused so far
- Pattern for arch-gated recipe steps: `when = { os = ["linux"], arch = "arm64", linux_family = "debian" }`

**JSON mismatch:**
- `installError.ExitCode` is `json:"exit_code"` — the field exists but is named differently from `sandboxJSONOutput`'s `install_exit_code`
- CI's `jq -r '.install_exit_code'` returns null on error responses because the field is `exit_code`, not `install_exit_code`

**No fallback in HombrewAction:**
- `getBlobSHA` returns a plain `fmt.Errorf` when no bottle entry matches the platform tag
- No retry, no system-package fallback — error propagates immediately up through plan generation

## Fix Shape

Three independent changes, none requiring design:

**Fix 1 — JSON field name (cmd/tsuku/install.go):**
Rename `exit_code` to `install_exit_code` in `installError` struct (or add the field alongside for backward compat). Makes error and success JSON shapes consistent for `.install_exit_code` consumers.

**Fix 2 — CI log upload (.github/workflows/test-recipe.yml):**
Upload `.log-${recipe}-${family}.txt` as workflow artifacts when the job fails. One-line change to add an `upload-artifact` step gated on `failure()`.

**Fix 3 — Dep recipe fallbacks (pkg-config.toml, make.toml):**
Split each `when = { os = ["linux"], libc = ["glibc"] }` homebrew step into:
- `when = { os = ["linux"], libc = ["glibc"], arch = "amd64" }` — keep homebrew
- `when = { os = ["linux"], libc = ["glibc"], arch = "arm64", linux_family = "debian" }` — `apt_install pkgconf` / `apt_install make`
- `when = { os = ["linux"], libc = ["glibc"], arch = "arm64", linux_family = "rhel" }` — `dnf_install pkgconf-pkg-config` / `dnf_install make`
- `when = { os = ["linux"], libc = ["glibc"], arch = "arm64", linux_family = "suse" }` — `zypper_install pkg-config` / `zypper_install make`

After Fix 3, PR #2373 can revert the `cca8089d` workaround and use `configure_make` uniformly for arm64+glibc.

## Suggested Issue Breakdown

**Issue A (simple):** Fix JSON shape and upload CI failure logs — `cmd/tsuku/install.go` + `.github/workflows/test-recipe.yml`

**Issue B (medium):** Add arm64+glibc system package fallbacks to `pkg-config.toml` and `make.toml` — enables all `configure_make` recipes on arm64

## Decision: Crystallize

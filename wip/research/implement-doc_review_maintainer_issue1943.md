# Maintainer Review: Issue #1943 -- `--env` flag for environment variable passthrough

**0 blocking, 2 advisory**

The implementation is clean and well-structured. The two-layer design (CLI resolves KEY-only to KEY=VALUE, then executor filters protected keys) keeps responsibilities separated. The `StringArrayVar` choice over `StringSliceVar` is the right call for env values that may contain commas. Tests are thorough and accurately named.

---

## Advisory Findings

### 1. `--env` silently ignored without `--sandbox`

`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/cmd/tsuku/install.go:229` -- The `--env` flag is registered on the install command unconditionally, but only consumed inside the `installSandbox` branch (line 58). A user who runs `tsuku install kubectl --env GITHUB_TOKEN=abc` (forgetting `--sandbox`) gets no error and no warning -- the flag is silently eaten.

The next developer debugging a CI pipeline that passes `--env` without `--sandbox` will spend time figuring out why the env var isn't reaching the container. They won't suspect the flag is being ignored because cobra accepted it without complaint.

Consider either: (a) validating that `--env` requires `--sandbox` at the top of the `Run` function (same pattern as the `--dry-run` + `--sandbox` check on line 66), or (b) adding a comment at the flag registration explaining it's sandbox-only. Option (a) is better since it fails fast. **Advisory** because the help text says "sandbox container" which is a hint, but hints in help text don't help during debugging.

### 2. `filterExtraEnv` godoc says KEY-only entries are resolved by the caller, but the function handles them anyway

`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/internal/sandbox/executor.go:543-544` -- The comment says "the caller resolves these to KEY= before calling this function" but the function explicitly handles KEY-only entries (no `=` separator) by treating the whole string as the key for protection checking. The test `TestFilterExtraEnv_KeyOnlyFormat` at `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/internal/sandbox/executor_test.go:784` exercises this path. The test comment even calls it out: "filterExtraEnv itself should handle KEY-only gracefully."

This isn't contradictory, but the godoc parenthetical creates a small mental model conflict: it first says the caller handles it, then implies the function handles it too. The next person reading the godoc might wonder whether the KEY-only handling is dead code or intentional defense-in-depth. Suggest simplifying the parenthetical to: "Entries without an `=` separator are treated as KEY-only (the whole string is compared against protected keys)." **Advisory** because the test documents the actual behavior clearly.

---

## Notes (not findings)

- Good use of `protectedEnvKeys` as a `map[string]bool` package variable rather than inlining the list in `filterExtraEnv`. Makes the protected set visible and easy to extend.
- `resolveEnvFlags` in `install_sandbox.go` has no unit tests, but the function is four lines of straightforward string manipulation with `os.Getenv`. The `filterExtraEnv` tests cover the downstream filtering. Acceptable given the simplicity.
- The `ExtraEnv` field comment on `SandboxRequirements` (requirements.go:68-72) lists the exact protected keys. This is helpful now but will go stale if `protectedEnvKeys` changes. Low risk since the map and the comment are in adjacent files and the drift would be caught in code review, but worth noting.

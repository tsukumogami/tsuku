## Summary

When a recipe uses `libc = ["musl"]` in when clauses, the planner does not produce family-specific plans. It generates generic `linux-amd64` plans instead of per-family plans like `linux-alpine-amd64`.

This happens because the `Constraint` struct has no `Libc` field, so `MergeWhenClause()` silently drops libc information. `AnalyzeRecipe()` then sees these steps as unconstrained Linux steps and assigns `FamilyAgnostic` policy.

## Expected behavior

A recipe with `when = { os = ["linux"], libc = ["glibc"] }` on some steps and `when = { os = ["linux"], libc = ["musl"] }` on others should produce family-specific plans (at minimum, alpine for musl and debian/rhel/arch/suse for glibc).

## Actual behavior

Both libc-scoped steps produce identical `Constraint{OS: "linux"}` with no family info. The planner determines `FamilyAgnostic` policy and only generates `linux/amd64` and `linux/arm64` platforms.

## Root cause

`Constraint` struct in `internal/recipe/types.go` (line ~596):

```go
type Constraint struct {
    OS          string
    Arch        string
    LinuxFamily string
    GPU         string
    // Missing: Libc string
}
```

`MergeWhenClause()` handles `LinuxFamily`, `OS`, `Arch`, and `GPU` but has no code path for `Libc`.

## Affected recipes

9 registry recipes currently use `libc = ["musl"]` when clauses:
- abseil, apr, cairo, fontconfig, gettext, giflib, gmp, jpeg-turbo, pcre2

None have golden files yet, so no incorrect golden files exist today.

## Workaround

Use `linux_family = "alpine"` instead of `libc = ["musl"]` (as done in the embedded Rust recipe). This is equivalent today since Alpine is the only musl family, but less semantically correct.

## Files involved

| File | Role |
|------|------|
| `internal/recipe/types.go` | `Constraint` struct missing `Libc` field; `MergeWhenClause` drops libc |
| `internal/recipe/policy.go` | `AnalyzeRecipe` only checks `LinuxFamily`, not libc |

## Fix approach

Add a `Libc` field to `Constraint`, propagate it in `MergeWhenClause`, and teach `AnalyzeRecipe` that libc-scoped steps need family-specific plan generation.

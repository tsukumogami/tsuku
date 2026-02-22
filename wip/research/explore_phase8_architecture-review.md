# Architecture Review: DESIGN-embedded-recipe-musl-coverage

**Reviewer**: architect-reviewer
**Date**: 2026-02-22
**Document**: `docs/designs/DESIGN-embedded-recipe-musl-coverage.md`

## Verdict

The design is structurally sound. It follows the established recipe pattern, doesn't introduce new abstractions, and changes are contained to the correct layers. Three issues need resolution before implementation: a heuristic gap in the static analysis that misses half the broken recipes, a field reference to a nonexistent struct member, and unclear verify command behavior on the musl path.

---

## 1. Architecture Clarity

**Adequate for implementation, with one structural error in the pseudocode.**

The three-layer decomposition (recipe TOML, coverage analysis, CI triggers) is clean and maps directly to files. The recipe changes follow the pattern established by cmake.toml, openssl.toml, and gcc-libs.toml -- no new patterns introduced.

However, the pseudocode in Component 2 references `step.OsMapping` as a struct field:

```go
if step.OsMapping == nil || hasLibcWhenClause(step) {
    continue
}
for _, mapped := range step.OsMapping {
```

The `Step` struct (`internal/recipe/types.go:345`) has no `OsMapping` field. `os_mapping` lives inside `step.Params` as `map[string]interface{}`. The correct access pattern, used throughout the codebase (e.g., `internal/recipe/platform.go:377`, `internal/actions/download.go:126`), is:

```go
osMapping, ok := step.Params["os_mapping"].(map[string]interface{})
```

This is a pseudocode error, not a design flaw -- the implementer can look at existing code and figure it out. But since the design is the implementation specification, getting the interface right matters.

---

## 2. Missing Components / Interfaces

### 2a. Heuristic Gap: Only Catches 3 of 6 Broken Recipes (Blocking)

The static analysis heuristic checks `os_mapping` values for glibc indicators ("gnu", "ubuntu", "debian", "fedora"). Checking the actual recipes:

| Recipe | os_mapping linux value | Contains glibc indicator? |
|--------|----------------------|--------------------------|
| rust.toml | `"unknown-linux-gnu"` | Yes ("gnu") |
| python-standalone.toml | `"unknown-linux-gnu"` | Yes ("gnu") |
| ruby.toml | `"ubuntu-22.04"` | Yes ("ubuntu") |
| nodejs.toml | `"linux"` | No |
| perl.toml | `"linux"` | No |
| patchelf.toml | no os_mapping | No |

**The heuristic misses nodejs, perl, and patchelf.** These three recipes produce glibc binaries despite having generic os_mapping values (nodejs, perl) or no os_mapping at all (patchelf uses `homebrew`).

The design says the static analysis "catches this class of bug at unit test time" and "future embedded recipes with the same problem" get flagged. But the heuristic has a 50% miss rate on the current set of broken recipes. Someone adding a new embedded recipe that downloads a glibc binary with `os_mapping = { linux = "linux" }` would pass the check.

**Recommendation**: The heuristic should be reframed or supplemented. Options:

1. **Flip the logic**: Instead of detecting glibc indicators in os_mapping, flag any embedded recipe that has steps matching Linux (via unconditional or os=linux when clauses) but has no step with `when.libc = ["musl"]`. This catches all six recipes and any future recipe that forgets the musl path, regardless of os_mapping content. This is closer to what `AnalyzeRecipeCoverage` already does -- it checks `HasMusl` -- but currently treats unconditional steps as musl-compatible. The fix would be: if a recipe has download/download_archive/homebrew/github_archive steps for Linux but no apk_install step, flag it.

2. **Narrow the scope**: Keep the os_mapping heuristic as one signal but add a second check: any embedded recipe with type != "library" that has `HasMusl = true` only because of unconditional download/extract steps should trigger a warning. Unconditional download steps are the real problem -- they claim musl compatibility but produce binaries that can't run on musl.

Option 1 is simpler and catches all cases. Option 2 is more precise but more complex.

### 2b. patchelf.toml Has a Different Problem Pattern

patchelf.toml doesn't use `os_mapping` at all. It uses an unconditional `homebrew` action:

```toml
[[steps]]
action = "homebrew"
formula = "patchelf"
```

On musl/Alpine, Homebrew isn't available. This is a different failure mode than the other five recipes (glibc binary download). The design groups all six together under the same fix pattern, which is correct for the recipe-level fix (add `when` + `apk_install`), but the static analysis needs to cover this case too.

### 2c. Verify Command Path Ambiguity

The design notes that verify commands referencing `{install_dir}/bin/cargo` won't work on the musl path since `apk_install` installs to system paths. It says "verify commands should use the bare executable name." But the design doesn't specify which recipes need verify command changes or what the new commands should be.

Looking at the actual recipes:
- `rust.toml`: `command = "{install_dir}/bin/cargo --version"` -- needs change for musl
- `python-standalone.toml`: `command = "python3 --version"` -- already bare, works
- `nodejs.toml`: `command = "{install_dir}/bin/node --version"` -- needs change for musl
- `perl.toml`: `command = "{install_dir}/bin/perl -v"` -- needs change for musl
- `ruby.toml`: `command = "{install_dir}/bin/ruby --version"` -- needs change for musl
- `patchelf.toml`: `command = "{install_dir}/bin/patchelf --version"` -- needs change for musl

Five of six recipes use `{install_dir}` in verify commands. The design should specify how verify works on the musl path. Does `apk_install` set `{install_dir}` to something? Does the verify step get a `when` clause too? Or does the verify step need conditional logic?

---

## 3. Phase Sequencing

**Correct.** Phase 1 (recipes) before Phase 2 (static analysis) before Phase 3 (CI) is the right order:

- The recipe fixes must land first so the static analysis changes don't immediately flag the recipes you're trying to fix.
- The CI trigger changes can go in any phase but are lowest risk, so last makes sense.

One small note: Phase 2 should include updating the existing test `TestTransitiveDepsHavePlatformCoverage` to verify it now catches the bug. The design mentions "update if needed" but this is the primary validation that the fix works. It should be explicit.

---

## 4. Simpler Alternatives

### 4a. The Design Already Chose the Simplest Recipe-Level Fix

`apk_install` with Alpine packages is the right call. The alternatives analysis is well-reasoned. No simpler option exists for the recipe layer.

### 4b. The Static Analysis Could Be Simpler

As noted in 2a, the os_mapping heuristic is complex (string matching on mapping values) and incomplete. A simpler check exists: "does this embedded recipe have Linux steps but no musl-specific step?" This is a structural check on step when clauses, which `AnalyzeRecipeCoverage` already mostly does. The gap is that unconditional steps are treated as musl-compatible when they aren't.

The simplest fix to AnalyzeRecipeCoverage would be: when evaluating an unconditional step that uses `download`, `download_archive`, `github_archive`, or `homebrew` actions, don't count it as musl-compatible. These actions produce binaries from external sources that are likely glibc-linked. Only `apk_install` and `run_command` (and similar) should count as musl-compatible when unconditional.

This avoids string-matching on os_mapping values entirely.

---

## 5. Testing and Validation Strategy

**The design has a testing gap.** It describes what the static analysis should catch but doesn't specify how to verify the recipe fixes actually work.

### What's Present

- Static analysis check in `AnalyzeRecipeCoverage` catches future regressions (unit test level)
- CI trigger change enables PR-time Alpine testing (integration test level)
- The weekly `recipe-validation-core.yml` provides ongoing Alpine testing (periodic validation)

### What's Missing

**No explicit test plan for the six recipe changes.** The design should specify:

1. **How to validate the musl path works**: Run `tsuku install <recipe>` in an Alpine container for each of the six recipes. This is the direct validation that the fix works. The CI trigger change enables this for future PRs, but for the initial PR, someone needs to either:
   - Manually trigger `test-recipe.yml` with each recipe name, or
   - Verify the PR triggers `test-recipe-changes.yml` (it will, since `internal/recipe/recipes/**/*.toml` is in the trigger paths)

2. **How to validate the glibc path isn't broken**: The `when` clause additions to existing steps could introduce regressions if applied incorrectly. The same CI run tests all five container families including debian and fedora, so glibc is covered -- but this should be stated explicitly.

3. **Drift prevention completeness**: The static analysis catch is the primary drift prevention mechanism. If it only catches 50% of the pattern (as noted in 2a), drift prevention is incomplete. The design should specify the false-negative rate of the chosen heuristic and accept it or fix it.

4. **Verify command testing on both paths**: The design notes the verify command issue but doesn't specify how to test that verify works on both glibc and musl after the changes.

### Recommendation

Add a "Validation Plan" section with:
- Pre-merge: PR must pass `test-recipe-changes.yml` on Alpine (already triggered by path)
- Pre-merge: Unit test `TestTransitiveDepsHavePlatformCoverage` must pass with the new heuristic
- Post-merge: Confirm next weekly `recipe-validation-core.yml` run shows all six recipes passing on Alpine
- Drift: Explain the coverage boundary of the static check -- what it catches and what it doesn't

---

## Summary of Findings

| # | Finding | Severity | Section |
|---|---------|----------|---------|
| 1 | `step.OsMapping` doesn't exist; os_mapping is in `step.Params["os_mapping"]` | Advisory | 2, pseudocode |
| 2 | Heuristic only catches 3/6 broken recipes (misses nodejs, perl, patchelf) | Blocking | 2a |
| 3 | patchelf uses unconditional `homebrew`, not `os_mapping` -- different failure mode not caught by proposed heuristic | Blocking | 2b |
| 4 | 5/6 recipes use `{install_dir}` in verify commands; musl path behavior unspecified | Advisory | 2c |
| 5 | No explicit validation plan for the recipe fixes | Advisory | 5 |
| 6 | Phase sequencing is correct | -- | 3 |
| 7 | Recipe-level fix pattern is the simplest available | -- | 4a |
| 8 | Simpler static analysis alternative exists: action-type awareness instead of os_mapping string matching | Advisory | 4b |

### Blocking Items

**Finding 2+3**: The static analysis heuristic must be redesigned to catch all six broken recipes and prevent the full class of future regressions. The os_mapping string-matching approach has a fundamental gap: it only works when the os_mapping value contains a distro-specific string. Generic values like `"linux"` and recipes without os_mapping (patchelf) are invisible to it. The simpler alternative -- treat unconditional download/homebrew steps as not musl-compatible -- catches all cases without string matching.

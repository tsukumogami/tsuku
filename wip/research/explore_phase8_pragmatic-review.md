# Pragmatic Review: DESIGN-embedded-recipe-musl-coverage

## Summary

The design addresses a real bug (six embedded recipes broken on Alpine/musl) with a proven fix pattern (apk_install fallbacks). The recipe fixes and CI trigger change are straightforward. The static analysis enhancement has a design flaw that will miss most of the affected recipes and could become a maintenance burden.

## Findings

### 1. os_mapping detection is fragile and misses 4 of 6 recipes -- Blocking

`docs/designs/DESIGN-embedded-recipe-musl-coverage.md:106` -- The proposed check inspects `os_mapping` values for glibc indicators (`gnu`, `ubuntu`, `debian`, `fedora`). But look at the actual recipes:

| Recipe | os_mapping linux value | Contains glibc indicator? |
|--------|----------------------|--------------------------|
| rust | `unknown-linux-gnu` | Yes (`gnu`) |
| python-standalone | `unknown-linux-gnu` | Yes (`gnu`) |
| ruby | `ubuntu-22.04` | Yes (`ubuntu`) |
| nodejs | `linux` | **No** |
| perl | `linux` | **No** |
| patchelf | (uses `homebrew`, no os_mapping) | **No os_mapping at all** |

The heuristic catches 3 of 6 broken recipes. nodejs, perl, and patchelf would pass the new check and remain undetected by static analysis.

Worse, `go.toml` and `zig.toml` also have `os_mapping = { linux = "linux" }` with no `when` clause -- are they also broken on musl? The design doesn't mention them. Go downloads statically-linked binaries so it probably works, and Zig likely does too. But the proposed detection heuristic can't distinguish "os_mapping value `linux` that produces a glibc binary" from "os_mapping value `linux` that produces a static binary." The mapping value tells you what goes into the URL template, not what libc the resulting binary links against.

**Fix:** The os_mapping value heuristic is the wrong signal. A simpler and complete approach: flag any embedded recipe that has no `when` clause with `libc` on any step and no `apk_install` step. This catches all six recipes without maintaining a list of glibc string patterns. Recipes like `go.toml` and `zig.toml` that work fine on musl would need either a `when` clause or a brief `# musl: static binary, works without apk_install` annotation (or an explicit `supported_libc` field). That forces recipe authors to make an active decision about musl rather than hoping a substring match catches problems.

### 2. patchelf recipe is not "as simple as described" -- Advisory

`internal/recipe/recipes/patchelf.toml` -- This recipe uses `homebrew` with no `when` clause, no `os_mapping`. The design's recipe fix pattern (add `when = { os = ["linux"], libc = ["glibc"] }` to existing glibc-specific steps) doesn't apply cleanly here because `homebrew` isn't glibc-specific by mechanism -- it just happens to install a glibc-linked binary on Linux. The fix needs to wrap the existing `homebrew` + `install_binaries` steps with a `when = { os = ["linux"], libc = ["glibc"] }` guard, add the `apk_install` musl path, and add a `when = { os = ["darwin"] }` for macOS. But the current recipe has no macOS/Linux distinction at all -- homebrew handles both. Splitting it into three paths is a bigger change than "add a when clause."

Not blocking because the fix is still straightforward, but the design understates the delta.

### 3. nodejs has hidden complications -- Advisory

`internal/recipe/recipes/nodejs.toml` -- The design says all six recipes follow the same pattern. But nodejs has `link_dependencies` and a `run_command` wrapper script with `when = { os = ["linux"] }`. These steps run on both glibc and musl. On the musl path (after apk_install), there's no `gcc-libs` dependency and no `node.real` wrapper to create. The existing `when = { os = ["linux"] }` steps need to become `when = { os = ["linux"], libc = ["glibc"] }`, or the wrapper script will run after `apk add nodejs` and break (there's no `bin/node` in the install_dir to rename to `bin/node.real`).

Similarly, the `runtime_dependencies = ["gcc-libs"]` in metadata is glibc-specific. On musl, `gcc-libs` would also need to be installed differently or skipped.

### 4. ruby wrapper script complexity on musl path -- Advisory

`internal/recipe/recipes/ruby.toml` -- Ruby has a 70-line wrapper script creating `ruby.real`, wrapping gem/irb/erb/bundle/bundler/rake/rdoc/ri, setting RUBYLIB/GEM_HOME/GEM_PATH, and handling DYLD_LIBRARY_PATH vs LD_LIBRARY_PATH. All of this exists for the glibc pre-built binary path. On musl with `apk add ruby`, none of this applies -- system ruby doesn't need wrappers. But the `link_dependencies` (libyaml) and `run_command` steps have no `when` clause, so they'd run on musl too unless guarded. The design doesn't mention this.

### 5. verify commands need per-recipe attention -- Advisory

`docs/designs/DESIGN-embedded-recipe-musl-coverage.md:149` -- The design notes that verify commands referencing `{install_dir}/bin/cargo` won't work on the musl path since `apk_install` puts binaries in system paths. This is correct, but the design doesn't propose a solution. Looking at the recipes:

- rust: `{install_dir}/bin/cargo --version` -- broken on musl
- python-standalone: `python3 --version` with `mode = "output"` -- likely works
- nodejs: `{install_dir}/bin/node --version` -- broken on musl
- ruby: `{install_dir}/bin/ruby --version` -- broken on musl
- perl: `{install_dir}/bin/perl -v` -- broken on musl
- patchelf: `{install_dir}/bin/patchelf --version` -- broken on musl

Five of six verify commands reference `{install_dir}`. The design acknowledges the issue but doesn't include it in the implementation plan. This is scope that needs doing but isn't estimated.

### 6. CI trigger change is already done -- scope that doesn't need doing

`docs/designs/DESIGN-embedded-recipe-musl-coverage.md:126` -- The design says `test-recipe-changes.yml` "already includes `internal/recipe/recipes/**/*.toml` in its trigger paths" and proposes to "verify this works correctly." This was confirmed: line 7 of `test-recipe-changes.yml` has `'internal/recipe/recipes/**/*.toml'`. The `test-recipe.yml` trigger only covers `recipes/**/*.toml` (registry recipes), which makes sense because it's a cross-platform validation workflow that tests in Docker containers on Alpine -- exactly what catches this class of bug. But `test-recipe.yml` also only detects changed recipes from `recipes/**/*.toml` in its detection step (line 85), not embedded recipes.

The "verify this works" task is real, but "add the path" is a no-op for `test-recipe-changes.yml`. For `test-recipe.yml`, it's more work than described because the detection logic also needs updating, not just the trigger path.

### 7. Security section is proportionate -- No finding

The security section is appropriate for the change. No over-analysis.

### 8. "Considered Options" are proportionate -- No finding

The alternatives are real and briefly dismissed. No speculative generality.

## Recommendations

1. **Redesign the static analysis check.** The os_mapping substring matching is the wrong abstraction. Instead: flag any embedded recipe lacking either (a) a `when` clause with `libc` on at least one step, or (b) an explicit `supported_libc` metadata field. This is simpler code, catches all cases, and doesn't need a growing list of glibc indicator strings.

2. **Acknowledge per-recipe complexity in the implementation plan.** nodejs, ruby, and patchelf aren't "add a when clause" -- they need existing unconditional steps guarded and verify commands reconsidered. The design should either detail these or say "implementation will handle per-recipe details" rather than implying they're all trivial.

3. **Decide on verify command strategy before implementation.** Five recipes have `{install_dir}` verify commands that break on the musl path. Options: conditional verify commands (if supported), bare executable verify, or skip verify on musl. This needs a decision, not a "note to self."

4. **Check go.toml and zig.toml.** Both download pre-built binaries with no libc guard. Are they static? If so, the revised static analysis should have a way to mark them as musl-safe without requiring an apk_install path.

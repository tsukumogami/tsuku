# Architecture Review: DESIGN-gem-exec-wrappers

## Summary

The design is sound and well-scoped. The proposed fix correctly identifies the root cause, chooses the right solution level (single function, single file), and reuses an established pattern. There are two structural issues worth noting -- one blocking, one advisory -- and one question about scope that needs a decision before implementation begins.

---

## Is the architecture clear enough to implement?

Yes, with one exception.

The data flow is precise: `findBundler()` -> `filepath.Dir(bundlerPath)` -> ruby bin dir used in wrapper template. The "rename to `.gem`, write wrapper" sequence mirrors exactly what `gem_install.go` lines 209-247 do, so the implementer has working reference code in the same file.

The one gap: the design says "Replace lines 470-492 in `executeLockDataMode()`" but does not state what to do with the `binDir` variable resolved by `findBundlerBinDir()` at line 451. The wrapper at step 4a renames `bin/<exe>` (in `rootBinDir`), but the source executable found during the verification loop (lines 457-468) may be at a different path -- `ruby/<version>/bin/<exe>` inside `installDir`, not in `rootBinDir`. The implementation needs to either:
- Rename from `binDir` (the deep path) to `rootBinDir/<exe>.gem`, or
- Copy from `binDir` to `rootBinDir/<exe>.gem`

The design implies rename but the paths are different directories. This needs to be spelled out explicitly.

---

## Missing components or interfaces

None structurally. The fix is genuinely self-contained within `executeLockDataMode()`. No new interfaces, no new parameters, no new actions.

The design correctly identifies that `install_binaries` will see the wrapper at `bin/<exe>` and the secondary path bug resolves automatically.

---

## Structural issues

### Issue 1 (Blocking): Template duplication without shared code is a divergence risk

The design designates extracting the wrapper template to `gem_common.go` as "optional, but reduces duplication." Given the actual code:

- `gem_install.go` already has the wrapper template at lines 218-234
- `gem_exec.go` will have an identical template after this fix
- `install_gem_direct.go` is a third gem action that currently creates bare symlinks (lines 97-113)

Template drift between `gem_install` and `gem_exec` is the exact problem being fixed -- the two paths produced different results. Making the fix optional means the next developer adding a fourth gem action path will copy one of the two templates without knowing they are supposed to be identical, and the bug recurs.

The template is the correctness-critical piece. Marking it optional understates the risk. Extracting to `gem_common.go` should be **required**, not optional.

This is a parallel pattern introduction: two copies of the same template with no mechanism to keep them synchronized.

### Issue 2 (Advisory): `findBundler()` has non-deterministic selection when multiple ruby versions are installed

`findBundler()` uses `filepath.Glob()` and returns `rubyDirs[0]` -- the first lexicographic match when multiple `ruby-*` directories exist. The ruby bin dir embedded in the wrapper script at install time is therefore version-specific and hardcoded.

This is fine as long as tsuku maintains one ruby installation per tool, but it means the wrapper script breaks if ruby is updated in place (the embedded path in the wrapper will point to the old ruby). The design acknowledges relocatability for the install directory itself but not for ruby upgrades.

This is contained -- `gem_install.go` has the same issue with `ResolveGem()` -- and not introduced by this change, so it's advisory.

---

## Are there simpler alternatives that were overlooked?

No. The design's alternatives section is thorough. The environment-variable-in-recipe approach (83 files) and the absolute-symlink approach (doesn't fix `GEM_HOME`) are correctly rejected. The bash wrapper is the right answer.

One micro-simplification the design doesn't mention: the wrapper could call `exec "$SCRIPT_DIR/<exe>.gem" "$@"` (delegating to the script's own shebang) instead of `exec ruby "$SCRIPT_DIR/<exe>.gem" "$@"`. This would avoid hardcoding `ruby` as the interpreter. However, the whole point of the wrapper is to override the shebang's ruby with tsuku's ruby via `PATH`, so explicitly calling `ruby` is intentional and correct. Not a gap.

---

## Is "extract shared wrapper template" appropriate as optional?

No. It should be required.

The duplication is not cosmetic -- it's the same logic that produced the original divergence. Two copies of a security-and-correctness-critical template with no enforcement to keep them synchronized will drift. The template is 15 lines and the extraction is a single-day task. There is no cost argument for keeping it optional.

Recommendation: reframe the implementation as two steps:
1. Extract wrapper template to `internal/actions/gem_common.go` as an exported or unexported function/constant
2. Replace symlink creation in `executeLockDataMode()` using the shared template

This also surfaces `install_gem_direct.go` as a third site that should be audited for consistency (it currently creates bare symlinks without wrappers).

---

## Implementation gaps to address

1. Clarify the rename/copy path: the source executable in `binDir` (deep bundler path) vs. `rootBinDir` (the `bin/` exposed to users). The rename must move the file from its deep location to `rootBinDir/<exe>.gem`, not rename within `rootBinDir`.

2. The existing `os.Remove(dstPath)` at line 481 removes a symlink. After the fix, the file at `dstPath` after a failed or partial previous install could be a wrapper script (regular file), which `os.Remove` also handles -- but the error behavior differs. The rollback logic in `gem_install.go` (restore original on write failure) should be mirrored.

3. Tests: the current `TestGemExecAction_LockDataMode_WithMockBundler` test (lines 1003-1076) verifies `Gemfile` and `Gemfile.lock` creation but does not verify that `bin/<exe>` is a wrapper script rather than a symlink. A new test case should assert the wrapper content contains `GEM_HOME`, `GEM_PATH`, and the ruby `PATH` line.

# Decision 1: Where does the homebrew dylib-chaining fix live?

## Question

Where in the codebase does the dylib-chaining fix live so that homebrew-bottle tool recipes can resolve sibling dylibs from tsuku-installed library deps on both macOS and Linux?

- **(a)** New composite action `homebrew_chained` that wraps `homebrew` + post-install chaining.
- **(b)** Strengthen the existing `homebrew` action so it chains deps automatically when present.
- **(c)** New recipe-callable action `chain_deps_into_rpath` that recipes invoke after `homebrew`.

## Context recap

The exploration established these facts (see `wip/research/explore_tsuku-homebrew-dylib-chaining_r1_lead-fix-rpath-analysis.md` and `wip/research/explore_tsuku-homebrew-dylib-chaining_r1_lead-install-patterns.md`):

1. The dylib-chaining mechanism already exists for `Type == "library"` recipes on darwin, in `internal/actions/homebrew_relocate.go:574-674` (`fixLibraryDylibRpaths`). It walks `WorkDir/lib` for `.dylib` files and adds `LC_RPATH` entries pointing at `$LibsDir/<dep>-<version>/lib` for each `ctx.Dependencies.InstallTime` entry, then re-codesigns.
2. Three independent gates fence this off from tool recipes:
   - **Gate G1** (line 103): `runtime.GOOS == "darwin" && Type == "library"` — only library recipes trigger.
   - **Gate G2** (line 609): reads `ctx.Dependencies.InstallTime` only — `RuntimeDependencies` does not flow there (`install_deps.go:386-408` only iterates `Metadata.Dependencies`; `plan_generator.go:702-714` only iterates `deps.InstallTime`).
   - **Gate G3** (line 578): `runtime.GOOS != "darwin"` short-circuits — Linux has no equivalent helper.
3. `fixElfRpath` (lines 334-401, Linux tool path) sets a single `$ORIGIN` rpath; never iterates `ctx.Dependencies`.
4. `fixMachoRpath` (lines 433-572, macOS tool path) rewrites `LC_LOAD_DYLIB` entries to `@rpath/foo.dylib` but only adds a single `@loader_path` rpath. It never adds dep-lib rpaths, so the rewritten loader entries can find the bottle's own libs but never sibling-installed deps.
5. `set_rpath` (`internal/actions/set_rpath.go`) is a recipe-callable primitive that already supports composing dep-lib chains via `{libs.foo.libdir}` or `{libs_dir}/foo-{deps.foo.version}/lib` interpolation. Its Linux backend uses `patchelf --force-rpath --set-rpath`; its macOS backend issues per-path `install_name_tool -add_rpath` calls.
6. Across 1,168 recipes that invoke `homebrew`, exactly **0** also invoke `set_rpath`. The "manual chain" workaround pattern has not converged — recipe authors are not authoring it. The only chain in the registry (curl) lives on the Linux source-build branch, not the homebrew branch.
7. Two recipes (git, wget) successfully chain deps into homebrew bottles today via step-level `dependencies = [...]` plus the existing `fixMachoRpath` `LC_LOAD_DYLIB` rewrites — but only because their dep recipes (pcre2, gettext) were hand-tuned to expose the precise `.dylib` filenames the bottle's `@rpath` looks for. This is N×M coordination cost: every new dep needs both ends patched.

The blast radius is >15 affected recipes across the registry (the exploration's stop-signal for routing to tsuku-core rather than recipe-side workarounds).

---

## Option (a): New composite action `homebrew_chained`

### What it implements

A new `HomebrewChainedAction` in `internal/actions/homebrew_chained.go`. Its `Decompose` method emits the same primitive steps as `HomebrewAction.Decompose` (`download_file`, `extract`, `homebrew_relocate`) plus one new tail step: a `chain_dylib_rpaths` (or named-equivalent) step that runs the dep walk currently locked inside `fixLibraryDylibRpaths` and an analogous Linux helper.

Concrete code sketch:

- New file: `internal/actions/homebrew_chained.go` defines `HomebrewChainedAction{}` with the same `Preflight`/`Execute`/`Decompose` shape as `HomebrewAction`. `Decompose` returns the three homebrew primitive steps followed by a `chain_dylib_rpaths` step that takes no recipe-author parameters but reads `ctx.Dependencies` to compose paths.
- New file: `internal/actions/chain_dylib_rpaths.go` factors the body of `fixLibraryDylibRpaths` into a runnable action. The new action drops the `Type == "library"` gate, reads from both `ctx.Dependencies.InstallTime` and `.Runtime`, and adds a Linux branch that composes `$ORIGIN/../lib:<dep1lib>:<dep2lib>:...` and shells out to patchelf (the same call shape `setRpathLinux` uses).
- `internal/actions/registry.go` registers both new actions.
- `internal/actions/homebrew_relocate.go:103` is left untouched. Library recipes keep using the existing in-action chaining.

The recipe author writes `action = "homebrew_chained"` instead of `action = "homebrew"`. Existing `homebrew` recipes are unchanged.

### Backward compatibility

Excellent. The existing `homebrew` action is untouched, so all 1,168 homebrew-using recipes — including the 4 Pattern C library recipes (pcre2, libnghttp3, libevent, utf8proc) and the 2 Pattern E recipes (git, wget) that already chain via step-level `dependencies` — keep working bit-identically.

The `Type == "library"` gate at line 103 stays in place, so library recipes' dylib chaining is bit-identical.

### Recipe-author surface

- **New TOML surface**: a second action name. Authors must learn that `homebrew` and `homebrew_chained` exist and choose between them.
- **Upgrade path for existing recipes**: opt-in, per-recipe rename. Curl (darwin), tmux (darwin), git, wget, and any future bottle recipe with chained deps switches `action = "homebrew"` to `action = "homebrew_chained"`. The migration is mechanical but must be done deliberately for each recipe — there is no automatic uptake.
- **Two-action drift risk**: the two action implementations share download/extract/relocate logic. A future change to homebrew (e.g., revision handling at `homebrew.go:150-193`) must be applied to both, or `homebrew_chained` falls behind. Could be mitigated by having `HomebrewChainedAction.Decompose` call `HomebrewAction.Decompose` and append the chain step.

### Test surface

- New unit tests: `homebrew_chained_test.go` with the same Decompose-fixture pattern as `homebrew_test.go`; the dep-walk test that asserts both InstallTime and Runtime deps produce rpath entries; the Linux patchelf branch test.
- New action-registry test: assert `homebrew_chained` is registered.
- No changes to existing `homebrew_test.go` or `homebrew_relocate_test.go` — the existing action is untouched.
- Integration test: a new sandbox fixture (a tool recipe declaring `homebrew_chained` + library deps) that confirms end-to-end resolution. Approximately 1 new sandbox golden file.
- Recipe validator: needs to know the new action name and its parameter schema. `internal/recipe/validator.go` action whitelist plus action-specific param checks.

### Failure modes

- **New bug surface**: divergence between `HomebrewAction` and `HomebrewChainedAction` over time. Bug fixes to homebrew that don't get mirrored produce subtle behavioral differences across the two paths.
- **Authoring confusion**: 95 curated recipes vs 10 curated homebrew users; a new author writes `action = "homebrew"` for a tool that needs chaining and silently gets the broken Pattern E behavior. The fix is in the registry but the recipe doesn't pick it up.
- **Bug class made harder**: existing library recipes can't accidentally regress — they don't go through the new code path.

### Reversal cost

Low-to-medium. The new action and the recipes that use it are additive. Walking back means either (i) deleting the new action and reverting the few recipes that used it, or (ii) folding the chain step into the original `homebrew` action and removing the wrapper. Recipe-side churn is bounded by the number of recipes that opted in.

---

## Option (b): Strengthen the existing `homebrew` action

### What it implements

The chaining behavior becomes part of the `homebrew_relocate` action, which is already a primitive step inside the `homebrew` action's decomposition.

Concrete code sketch:

- `internal/actions/homebrew_relocate.go:103`: replace the gate with a less restrictive condition. Two viable shapes:
  - Lift the gate entirely so the function name changes to `fixDylibRpaths` and runs whenever `ctx.Dependencies` has any entries.
  - Keep a gate but widen it to `Type == "library" || hasChainableDeps(ctx)`.
- The body of `fixLibraryDylibRpaths` (lines 574-674) becomes a generic dep walker. It still walks `WorkDir/lib` for `.dylib` files but also walks `WorkDir/bin` for any binary that ended up with rewritten `LC_LOAD_DYLIB` entries (so tool binaries get rpath entries too, not just dylibs).
- New Linux helper `fixDylibRpathsLinux` (or extend `fixElfRpath`) iterates `ctx.Dependencies.{InstallTime,Runtime}` and composes `$ORIGIN/../lib:<dep1lib>:<dep2lib>:...`, calling `patchelf --force-rpath --set-rpath` for each ELF binary that contains Homebrew placeholders. The same call shape `setRpathLinux` uses.
- Wiring fix for `RuntimeDependencies` reaching `ctx.Dependencies` — modify `cmd/tsuku/install_deps.go:386-408` to also iterate `r.Metadata.RuntimeDependencies` and `cmd/tsuku/install_lib.go:75-100` similarly. Or extend `actions/resolver.go` so `Runtime` deps populate a separate-but-readable map. (The exact wiring choice is decision 2, but option (b) requires *some* fix here.)
- `homebrew_relocate.go` reads from both `ctx.Dependencies.InstallTime` and `ctx.Dependencies.Runtime` for chaining.
- For tool deps that live under `$ToolsDir` (not `$LibsDir`), construct fallback paths or skip silently. The dynamic loader skips missing rpath entries, so dead entries are tolerable but bloat the binary.

### Backward compatibility

This is the riskiest dimension. The change is subtle:

- **Pattern A recipes (docker, pyenv, tmux on linux, openjdk)**: no `dependencies`, no `runtime_dependencies` flowing to ctx. Dep walk no-ops. **Zero behavior change.**
- **Pattern C recipes (pcre2, libnghttp3, libevent, utf8proc)**: already trigger `fixLibraryDylibRpaths` via the `Type == "library"` gate. If the gate is replaced with a generic condition, they continue to trigger. The dep walk behavior is identical (same code, just renamed). **Bit-identical behavior** if the refactor is faithful.
- **Pattern E recipes (git, wget)**: declare step-level `dependencies = [...]` which already flows into `ctx.Dependencies.InstallTime`. Today they get the existing `fixMachoRpath` `LC_LOAD_DYLIB` rewrite (which works). With the strengthened action, they also get new rpath entries pointing at the dep `lib/` dirs. **Behavior change**: an additional rpath entry per dep. The rewritten `LC_LOAD_DYLIB` entries already resolve via the existing patching, so the new entries are functionally redundant. The redundancy is harmless on macOS (extra rpath entries are tolerated and ignored if not needed), but it does add codesign cycles and bloats `LC_RPATH`.
- **Linux Pattern A recipes**: gain new rpath entries via the new Linux helper. For recipes with no chained deps (most), no-op. For tmux on Linux (no deps declared anyway), no-op.

The wiring change (`RuntimeDependencies` → `ctx.Dependencies`) has its own blast radius: it changes what `plan.Dependencies` / `DependencyPlan` carries, which can invalidate `plan_cache_test.go` golden fixtures and may require a plan-cache version bump.

### Recipe-author surface

- **No new TOML surface**. Authors keep writing `action = "homebrew"`. They opt in to chaining by adding `runtime_dependencies` (or step `dependencies`) — same fields they already use today, just with new effects.
- **Upgrade path**: zero. Every existing recipe with declared deps automatically gains chaining. No recipe rewrites are required to fix curl-darwin, tmux-darwin, or any future bottle.
- **Surprise risk**: a recipe author who currently relies on the absence of chaining behavior (none surveyed, but theoretically possible) might see a behavior change. The exploration found no such case.

### Test surface

- Existing `homebrew_relocate_test.go` covers `Dependencies()`, `extractBottlePrefixes`, `findPatchelf*`. None of these are affected.
- New tests: assert the gate-replacement runs `fixDylibRpaths` for tool recipes when deps are non-empty; assert it no-ops for tool recipes with no deps; assert the Linux helper composes the right rpath chain; assert `RuntimeDependencies` flow into `ctx.Dependencies` via the wiring fix.
- `executor_test.go::TestSetResolvedDeps` (line 1372) probably needs a `RuntimeDependencies` variant.
- `plan_generator_test.go` tests need to assert runtime deps appear in `DependencyPlan`. Plan-cache golden fixtures may need regeneration — `plan_cache_test.go`.
- `validator_runtime_deps_test.go` and `validator_runtime_deps_integration_test.go` cover the multi-satisfier alias rule. Untouched by the wiring fix.
- Integration: end-to-end test for a chained homebrew tool — same fixture as option (a), but exercising the unchanged `action = "homebrew"`.

The plan-cache golden regeneration is the most expensive piece. It's a one-time cost paid in CI.

### Failure modes

- **New bug class**: a recipe with `dependencies` listed for a non-dylib reason (e.g., a build-time dep that doesn't ship any libs) might gain dead rpath entries that bloat the binary and waste a codesign cycle. Verifying this doesn't break existing Pattern E recipes (git, wget) requires careful integration testing.
- **Wiring change spillover**: if the `RuntimeDependencies` → `ctx.Dependencies` wiring affects other action consumers of `ctx.Dependencies` (e.g., `install_binaries`, `setup_build_env`), the change has wider-than-anticipated semantics. The action audit is necessary.
- **Bug class made harder**: recipe authors no longer need to remember to add a chaining step. Pattern E (the wget/git pattern) becomes the default for any recipe with declared deps — the most common authoring shape automatically gets the right behavior.

### Reversal cost

Medium-to-high. Once shipped, ~10 curated recipes (and an unbounded number of future recipes) silently rely on the strengthened behavior. Walking back means re-introducing the gate AND adding explicit chaining steps to every recipe that started depending on the new behavior. The plan-cache schema change is itself reversible but cache-invalidating.

The wiring change to `RuntimeDependencies` propagation is the longest-tailed piece. If it turns out to be wrong, undoing it touches the executor, plan generator, install command, and library-install command.

---

## Option (c): New recipe-callable action `chain_deps_into_rpath`

### What it implements

A new primitive action that recipes invoke explicitly after `homebrew`. The action wraps the rpath composition that `set_rpath` already supports, but with a higher-level interface: instead of writing `rpath = "$ORIGIN/../lib:{libs_dir}/openssl-{deps.openssl.version}/lib:..."`, the author writes `chain_deps_into_rpath` with a `binaries = [...]` list and the action computes the chain from `ctx.Dependencies` automatically.

Concrete code sketch:

- New file: `internal/actions/chain_deps_into_rpath.go` defines `ChainDepsIntoRpathAction`. Its `Execute` does roughly:
  1. Read `binaries` from params (defaulting to all binaries in `WorkDir/bin` and all `.dylib` files in `WorkDir/lib`).
  2. Build the dep-lib path list from `ctx.Dependencies.InstallTime` (and possibly `Runtime`, see decision 2). For each entry, compose `$LibsDir/<name>-<version>/lib`.
  3. On Linux: compose `$ORIGIN/../lib:<dep1>:<dep2>:...` and call patchelf via the same code path as `setRpathLinux`.
  4. On macOS: call `install_name_tool -add_rpath` per dep path on each binary, then re-codesign — same code path as `fixLibraryDylibRpaths` does for libraries.
- Register in `internal/actions/registry.go`.
- `homebrew_relocate.go:103` is left untouched. Library recipes keep using the existing in-action chaining.

The recipe author appends:

```toml
[[steps]]
action = "homebrew"
formula = "wget"
dependencies = ["openssl", "gettext", "libidn2", "libunistring"]

[[steps]]
action = "chain_deps_into_rpath"
when = { os = ["darwin"] }
binaries = ["bin/wget"]
```

### Backward compatibility

Excellent — same as option (a). The existing `homebrew` action and `homebrew_relocate` action are untouched. All 1,168 homebrew recipes keep working.

The `Type == "library"` gate stays. Library recipes' dylib chaining is bit-identical.

### Recipe-author surface

- **New TOML surface**: one new action. Authors learn one new step name. The action's parameter schema is small — `binaries`, optional `include_runtime_deps`, optional `extra_paths`.
- **Upgrade path**: opt-in, per-recipe append. Curl (darwin re-enabled), tmux (darwin re-enabled), git, wget, etc. each append a `[[steps]] action = "chain_deps_into_rpath"` block.
- **Step-ordering semantics**: the action must run after `homebrew` (because it operates on relocated binaries), so authors must place it correctly. The exploration shows the curl source-build pattern already does this for `set_rpath` — same discipline applies.
- **Functional overlap with set_rpath**: the new action is essentially `set_rpath` with the dep-chain pre-computed. Recipe-author guidance must explain when to use which: `set_rpath` for full rpath control, `chain_deps_into_rpath` for the homebrew-chained common case.

### Test surface

- New unit tests: `chain_deps_into_rpath_test.go` covering both ELF and Mach-O code paths, both InstallTime and Runtime dep selection, missing-dep error handling, recipe-as-dep-not-library handling.
- No changes to existing tests — `homebrew_test.go`, `homebrew_relocate_test.go`, `set_rpath_test.go` all stay.
- Validator: action whitelist plus param schema. Same surface as option (a).
- Integration: same fixture as the other options, exercising the explicit two-step pattern.

### Failure modes

- **New bug surface**: small. The action is mostly a thin wrapper over existing primitives (`patchelf` + `install_name_tool`).
- **Authoring forgetfulness**: same as option (a). A recipe author who doesn't know to add the new step gets the broken Pattern E behavior. The fix is in the registry but doesn't auto-apply.
- **Confusion with `set_rpath`**: two actions in similar territory — recipe authors must learn the distinction. Documentation burden.
- **Bug class made harder**: explicit chaining step makes the chain visible in the recipe; reviewers can audit it. (Compare with option (b) where the chain is implicit and only visible in the deps list.)

### Reversal cost

Low. The action and its uses are additive. Walk-back: delete the action, remove the few recipes that called it. Recipe churn is bounded by the number of opt-ins.

---

## Comparison

| Dimension | (a) homebrew_chained | (b) strengthen homebrew | (c) chain_deps_into_rpath |
|-----------|----------------------|--------------------------|----------------------------|
| Backward compat risk | Very low (additive) | Medium (changes existing path) | Very low (additive) |
| Recipe author burden | New action name to learn; rename existing recipes | None | New action name; new step in existing recipes |
| Auto-applies to existing recipes | No (rename required) | **Yes** | No (append required) |
| Auto-applies to future recipes | No | **Yes** | No |
| Code duplication risk | High (two homebrew action paths) | None | Low (small wrapper over patchelf/install_name_tool) |
| Test surface | Medium | Medium-high (plan-cache regen) | Low |
| Reversal cost | Low | Medium-high | Low |
| Wiring change for runtime deps required | If recipes use `runtime_dependencies` to drive it, yes | Yes | If recipes use `runtime_dependencies` to drive it, yes |
| Linux + macOS unification | Built-in (Decompose emits per-platform step) | Built-in (action handles both) | Built-in (action handles both) |
| Visible in recipe TOML | Action name change | Hidden (deps list is the input) | Explicit step |

A subtlety: the wiring change for `RuntimeDependencies` → `ctx.Dependencies` is independent of which option we pick, IF the design wants `runtime_dependencies` to be the input. Decision 2 settles that; this decision settles the *shape* of the entry point, not what fills it.

---

## Recommendation

**Choose option (b): strengthen the existing `homebrew` action.**

### Confidence: medium-high

### Rationale

Three load-bearing facts pointed me here:

1. **The exploration's count of converged workarounds is zero.** Across 1,168 homebrew-using recipes, exactly 0 chain `homebrew + set_rpath`. Two recipes (git, wget) chain via step-level `dependencies`, which is already the implicit/automatic shape. Recipe authors are not converging on explicit-step patterns. Option (a) and option (c) both ask authors to learn and use a new explicit step — and the data says they won't. They'll write `action = "homebrew"` and not chain anything, recreating the curl/tmux gap for every new tool.

2. **Library recipes already prove the in-action approach works.** `fixLibraryDylibRpaths` is the production proof that the chaining belongs inside the homebrew path. Lifting the `Type == "library"` gate is a generalization, not a new mechanism. Option (b) takes a battle-tested primitive (used by 4 curated recipes today) and applies it to a wider input set. Options (a) and (c) introduce a new mechanism that does the same thing.

3. **The blast radius assessment routes to tsuku-core.** The exploration's stop-signal threshold was 15+ affected recipes. The actual count is 19+ with no converged workaround. The strategic answer is "fix in tsuku-core, not in every recipe." Options (a) and (c) push the fix to recipe authors via opt-in. Option (b) puts the fix in tsuku-core where the strategic scope says it belongs.

The trade-off I'm accepting:

- Option (b) is harder to walk back if it turns out to be wrong. The plan-cache schema change in particular has a long tail. I judge this acceptable because the change is structurally a generalization of behavior that already exists for libraries — the failure mode "this is silently wrong for tool recipes" is no worse than the current "this is silently absent for tool recipes."
- The behavior change for git and wget (additional rpath entries on top of the existing `LC_LOAD_DYLIB` rewrites) is benign on macOS and Linux. The exploration confirmed that the rewritten `LC_LOAD_DYLIB` entries still resolve, and extra rpath entries are tolerated by both dynamic loaders. This is the largest live-fire test we have, and it shipped clean.

A reasonable runner-up is option (c). I'd pick it if I were less confident about the wiring fix being right (`RuntimeDependencies` → `ctx.Dependencies`). Option (c) lets recipe authors be explicit about chains the heuristic doesn't catch, and it composes cleanly with the existing `set_rpath`. But it does not auto-fix the registry the way option (b) does, and the data says authors don't reach for explicit chains.

I rule out option (a) outright. The duplicated homebrew action paths are a maintenance trap (every fix to `homebrew.go` needs mirroring), and the recipe-author surface is the worst of both worlds: a new action name to learn AND a rename burden on existing recipes.

### Assumptions (recorded for follow-on review)

1. **Decision 2 will land on a way to flow `runtime_dependencies` into `ctx.Dependencies`.** Without that wiring, option (b)'s implicit-via-deps-list shape doesn't reach the recipes that need it (curl, tmux on darwin, etc.). If decision 2 lands on "use step-level `dependencies` only," option (b) still works (git and wget already use that shape) but the recipe-author burden grows because every chain-using recipe must duplicate the dep list at the metadata level for runtime + step level for chaining.
2. **The wiring change can be done without introducing a security regression.** Specifically: passing `runtime_dependencies` versions into `ctx.Dependencies` must not allow rpath injection beyond what `validateRpath` already permits (paths under `$LibsDir` only).
3. **Plan-cache golden fixtures can be regenerated in CI.** If they can't (e.g., because the fixtures are pinned to a tagged release), this introduces a bigger migration concern that decision 2 or a follow-on issue must address.
4. **Tool deps installed under `$ToolsDir` (not `$LibsDir`) are not in scope for chaining.** The existing `fixLibraryDylibRpaths` constructs paths against `$LibsDir`. If a tool depends on another tool that ships dylibs in `$ToolsDir/<name>-<version>/lib`, the strengthened action will not find them. The exploration noted this; the design accepts it because the curated registry's chained-dep cases are all library deps.
5. **Existing Pattern C library recipes (pcre2, etc.) don't regress when the gate is replaced with a non-library-specific condition.** This is verifiable via existing library test fixtures plus a new library-recipe-with-deps test.

### Rejected options (one-line summary)

- **(a) New composite `homebrew_chained`**: forces every recipe to opt in, and recipe authors don't — the curated count of converged workarounds is zero. Maintenance: two homebrew action paths to keep in sync.
- **(c) New recipe-callable `chain_deps_into_rpath`**: cleanest backward-compat story but doesn't auto-apply to existing or future recipes; requires every author to know they need to add the step. Functional overlap with `set_rpath` adds documentation burden.

---

## File / line index for follow-up

| File | Line(s) | What's there |
|------|---------|--------------|
| `internal/actions/homebrew_relocate.go` | 103 | Type=library gate (the surgical change point for option b) |
| `internal/actions/homebrew_relocate.go` | 574-674 | `fixLibraryDylibRpaths` — body that becomes generic in option b |
| `internal/actions/homebrew_relocate.go` | 334-401 | `fixElfRpath` — Linux tool path that needs the equivalent dep walk added |
| `internal/actions/homebrew_relocate.go` | 433-572 | `fixMachoRpath` — macOS tool path with single `@loader_path` rpath today |
| `internal/actions/homebrew.go` | 475-574 | `HomebrewAction.Decompose` — primitive step list (extension point for option a) |
| `internal/actions/set_rpath.go` | 33-118 | `SetRpathAction.Execute` — recipe-callable analog (option c is a layer over this) |
| `internal/actions/registry.go` | (registry of action names) | Registration site for new actions in options a and c |
| `cmd/tsuku/install_deps.go` | 386-408 | `SetResolvedDeps` site — where wiring change for runtime deps lands |
| `cmd/tsuku/install_lib.go` | 75-100 | Library-install site — same wiring change |
| `internal/executor/plan_generator.go` | 702-714 | `generateDependencyPlans` — iterates InstallTime only today |
| `internal/recipe/types.go` | 169-172 | `Dependencies` and `RuntimeDependencies` recipe schema |
| `internal/actions/homebrew_relocate_test.go` | 1-205 | Existing tests; option b adds gate-replacement coverage |
| `recipes/c/curl.toml` | 1-58 | Pattern D recipe that the fix unblocks for darwin |
| `recipes/w/wget.toml` | 32-43 | Pattern E recipe that gains additional benign rpath entries under option b |
| `recipes/p/pcre2.toml` | 79-88 | Pattern C library recipe whose chaining must remain bit-identical |

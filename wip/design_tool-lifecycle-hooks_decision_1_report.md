# Decision 1: Recipe Schema and Lifecycle Phase Model

## Question

How should recipes declare lifecycle steps, what new declarative actions are needed, and what gets stored in state for cleanup?

## Options Evaluated

### Option A: Phase field on Step struct

Add a `phase` string field directly to the `Step` struct (values: `"install"` (default), `"post-install"`, `"pre-remove"`, `"pre-update"`). Steps without a phase default to `"install"` for backward compatibility. The executor filters steps by phase at each lifecycle point.

**Recipe example (niwa shell-init):**

```toml
[[steps]]
action = "github_archive"
repo = "tsukumogami/niwa"
asset_pattern = "niwa-v{version}-{os}-{arch}.tar.gz"
archive_format = "tar.gz"
strip_dirs = 1
binaries = ["niwa"]

[[steps]]
action = "install_shell_init"
phase = "post-install"
source_file = "shell/niwa.sh"
target = "niwa"

[[steps]]
action = "remove_shell_init"
phase = "pre-remove"
target = "niwa"
```

**Pros:**
- Minimal schema change: one new field on a struct that already has `Action`, `When`, `Note`, `Description`, `Params`
- Backward compatible: omitting `phase` means `"install"`, so every existing recipe works unchanged
- Steps stay in a single `[[steps]]` array, preserving the flat recipe structure authors already know
- `When` clauses compose naturally with `phase` (e.g., a shell-init step that only runs on Linux)
- The executor already iterates `recipe.Steps` and checks `shouldExecute(step.When)` -- adding a phase filter is a one-line check
- Step ordering within a phase is explicit (array order), matching how install steps work today

**Cons:**
- Phase is conceptually different from platform conditions (it's a lifecycle qualifier, not a filter), but it lives alongside `When` at the step level
- All phases share one array, so a recipe with many hooks might feel cluttered
- Requires TOML parser update to extract the new field, though this follows the same pattern as `note` and `description`

### Option B: Phase field on WhenClause

Add `phase` to the `WhenClause` struct alongside `os`, `arch`, `libc`, `gpu`, and `package_manager`. Steps with `when.phase = "post-install"` only run during that lifecycle point. The existing `shouldExecute()` method gains phase awareness.

**Recipe example (niwa shell-init):**

```toml
[[steps]]
action = "install_shell_init"
source_file = "shell/niwa.sh"
target = "niwa"
[steps.when]
phase = "post-install"

[[steps]]
action = "remove_shell_init"
target = "niwa"
[steps.when]
phase = "pre-remove"
```

**Pros:**
- Reuses existing WhenClause infrastructure and `shouldExecute()` method
- No new top-level Step fields needed
- Platform and phase conditions can be combined in a single `when` block

**Cons:**
- Conflates two fundamentally different concepts: `phase` is a lifecycle qualifier that determines *when in the install flow* a step runs, while `os`/`arch`/`libc` are *platform filters*. Platform conditions are AND'd together; phase is a mode selector.
- Every step without an explicit `when.phase` implicitly runs during install. This means `shouldExecute()` must handle a missing phase differently from a missing OS -- it's not just "match all" but "default to install phase." The IsEmpty() shortcut breaks.
- Forces recipe authors to use the nested `[steps.when]` syntax even for steps that have no platform conditions, just because they need a phase. This is more verbose than a top-level `phase` field.
- The WhenClause is already validated, matched, and serialized in multiple places. Adding phase semantics (which don't follow the same matching logic) creates a special case in every one of those paths.

### Option C: Separate step arrays per lifecycle phase

Add `[[post_install_steps]]`, `[[pre_remove_steps]]`, and `[[pre_update_steps]]` as new top-level TOML arrays in the Recipe struct. Each uses the same Step schema. The existing `[[steps]]` array remains the install phase.

**Recipe example (niwa shell-init):**

```toml
[[steps]]
action = "github_archive"
repo = "tsukumogami/niwa"
asset_pattern = "niwa-v{version}-{os}-{arch}.tar.gz"
archive_format = "tar.gz"
strip_dirs = 1
binaries = ["niwa"]

[[post_install_steps]]
action = "install_shell_init"
source_file = "shell/niwa.sh"
target = "niwa"

[[pre_remove_steps]]
action = "remove_shell_init"
target = "niwa"
```

**Pros:**
- Cleanest separation: each lifecycle phase has its own array, making intent obvious
- No changes to Step or WhenClause structs
- Easy to validate that only appropriate actions appear in each phase (e.g., `remove_shell_init` only valid in `pre_remove_steps`)

**Cons:**
- Requires three new fields on the Recipe struct (`PostInstallSteps`, `PreRemoveSteps`, `PreUpdateSteps`)
- Breaks the "one steps array" mental model that recipe authors have learned
- TOML serialization in `ToTOML()` needs new encoding blocks for each array
- The loader, validator, and `NewStep` construction all need to handle multiple arrays
- Future phases (post-remove, pre-install, etc.) each require another top-level array -- the schema grows linearly with phases
- Step reuse across phases (e.g., a cleanup step used in both pre-remove and pre-update) requires duplication

## Chosen

**Option A: Phase field on Step struct.**

The phase field is the smallest change that solves the problem. It adds one field to Step, requires one check in the executor's step loop (`if step.Phase != currentPhase { continue }`), and keeps recipes in the familiar `[[steps]]` array format. Backward compatibility is automatic: every existing recipe has no `phase` field, so all steps default to `"install"`.

The conceptual concern about mixing lifecycle phase with platform conditions is real but manageable. Phase is a top-level Step field, not nested inside WhenClause, so the two concepts stay visually and structurally separate. Recipe authors write `phase = "post-install"` next to `action = "install_shell_init"` -- it reads naturally.

Option C's clean separation doesn't justify the schema proliferation. Each new phase would require a new TOML array, a new Recipe struct field, new loader code, and new validation paths. Option A scales to any number of phases without touching the schema.

## New Declarative Actions

Three new actions are needed for the initial lifecycle hook support:

1. **`install_shell_init`** -- Copies or generates a shell initialization snippet to `$TSUKU_HOME/shell.d/{target}.sh`. Params: `source_file` (path relative to tool install dir) or `content` (inline), `target` (name for the shell.d file). Phase: `post-install`.

2. **`remove_shell_init`** -- Removes `$TSUKU_HOME/shell.d/{target}.sh`. Params: `target`. Phase: `pre-remove`.

3. **`install_completions`** -- Installs shell completions to `$TSUKU_HOME/completions/{shell}/{tool}`. Params: `command` (to generate completions) or `source_file`, `shells` (array of target shells). Phase: `post-install`. Lower priority than shell-init.

Each action is declarative: it takes structured parameters, not arbitrary scripts. This preserves tsuku's no-code-execution trust model.

## Cleanup State

At install time, the executor records cleanup instructions in `VersionState` as a new `Hooks` field:

```go
type HookState struct {
    ShellInit    *ShellInitHook    `json:"shell_init,omitempty"`
    Completions  *CompletionsHook  `json:"completions,omitempty"`
}

type ShellInitHook struct {
    Target string `json:"target"` // shell.d filename (without extension)
}

type CompletionsHook struct {
    Target string   `json:"target"` // completions filename
    Shells []string `json:"shells"` // which shells have completions
}
```

The `install_shell_init` action writes to `$TSUKU_HOME/shell.d/` and records `ShellInitHook{Target: "niwa"}` in the version's state. During removal, the remove flow reads state (which it already does) and cleans up `$TSUKU_HOME/shell.d/niwa.sh` -- no recipe needed.

This means `remove_shell_init` as a recipe action is technically redundant for cleanup, since the state-driven approach handles it. But having it available gives recipe authors explicit control and makes the recipe self-documenting about what it creates and destroys. The state-based cleanup acts as a safety net if the recipe's pre-remove steps are missing or fail.

**What gets stored, where, when:**
- **What:** Hook type and the minimum info needed for cleanup (target names, shell list)
- **Where:** `state.json` under `installed.{tool}.versions.{version}.hooks`
- **When:** Written by the post-install action execution, read by the remove flow

## Assumptions

- The `$TSUKU_HOME/shell.d/` directory pattern (from exploration decisions) is the target for shell initialization files. Users source a single `$TSUKU_HOME/env` that globs `shell.d/*.sh`.
- Recipe authors will rarely use more than 1-2 lifecycle hooks per recipe. The single-array approach won't become unwieldy.
- The executor will gain a method like `ExecutePhase(phase string)` that filters and runs steps for a given phase, reusing the existing step execution machinery.
- The remove flow already loads ToolState/VersionState. Adding hook cleanup to that flow is straightforward.

## Rejected Alternatives

**Option B (Phase on WhenClause):** Conflates lifecycle semantics with platform filtering. WhenClause's matching logic (AND across dimensions, empty = match all) doesn't fit phase selection semantics. Would require special-casing phase throughout the matching, validation, and serialization code that handles WhenClause today.

**Option C (Separate step arrays):** Too much schema surface area for the benefit. Each new lifecycle phase requires a new Recipe struct field, TOML array, loader path, and validator. Doesn't scale well and fragments recipe authoring across multiple arrays.

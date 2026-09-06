# Explore Scope: activation-injection

## Visibility

Public

## Core Question

Two defects in `internal/shellenv` are proven and reproduced: a declared tool
name is never validated and reaches `filepath.Join` inside `Config.ToolBinDir`,
so `..` escapes `$TSUKU_HOME/tools`; and `FormatExports` quotes with `%q`, which
emits a Go string literal rather than shell-quoted text, so `$( )` and backticks
in an emitted value execute when the hook output is `eval`'d. "Does this bug
exist" is settled.

What is not settled is the **extent of the class**. Which other
externally-supplied values reach path construction or shell-evaluated text
without validation, and are these two the whole set or the two someone happened
to find?

## Context

From tsukumogami/tsuku#2553 and the dispatch brief.

Two specific reasons to doubt the answer is "just these two":

- **A validated half next to an unvalidated half.** `ValidateRequested` and
  `ValidateVersionString` guard the *version* component of the `ToolBinDir`
  interpolation; the *name* component is guarded nowhere. The codebase already
  validates one component of a composed path and not the other. That asymmetry
  suggests the guard was written against the input someone was thinking about
  rather than against the composition, which is a pattern that repeats.
- **A belief, not a slip.** `%q` was chosen for shell emission by someone who
  believed it quoted for a shell. Wherever else that belief was applied is the
  same defect.

There is also a **false record of a control**, which is part of the defect
rather than a nicety. `docs/designs/current/DESIGN-shell-env-activation.md:419`
rates "PATH injection via malicious .tsuku.toml" as Low, mitigated by "All paths
constrained to $TSUKU_HOME/tools/, name validation". Both halves are false, and
the residual-risk cell *in the same row* reads "Tool names with unusual
characters could construct unexpected paths" — the table records the gap and
rates it Low one cell away from claiming it is mitigated. Whether that is the
only such row is itself a research question.

### Exposure, stated accurately

The activation hook (`tsuku hook install --activate`) is opt-in and was found
installed on neither host checked, so the drive-by vector needs a user who
enabled it. But `tsuku shell` needs no hook at all, so anyone running
`eval "$(tsuku shell)"` in an untrusted repository is exposed regardless. "Most
users are unaffected" is true; "nobody is affected" is not.

### Settled before this explore ran

The file overlap with the sibling chain on tsuku#2543 is resolved (agreed with
`tsuku_activation_pins`):

- That chain's draft PR #2554 (`fix/shellenv-activation-pins`) moves
  `activate.go` from `internal/shellenv` to a new `internal/activation`. **`main`
  is untouched**, so this work is written against
  `internal/shellenv/activate.go` and stays cherry-pickable against `main`.
- This PR lands **first**; that chain rebases and git carries the commit through
  the rename.
- The tool-name check goes at the head of the `ComputeActivation` tool loop and
  that chain keeps it as the first step of its resolution sequence rather than
  reimplementing it.
- The one genuine contact point is `FormatExports`: that chain adds a
  `_TSUKU_STATE_STAMP` emission in both the bash/zsh and fish branches plus the
  deactivation `unset`. **That new value must go through the quoter this work
  introduces.** A third `%q` landing one issue after three are removed reopens
  exactly what was closed. This is a design input, not only a criterion on the
  other chain: the quoter should be shaped so that adding an emitted value
  without it is awkward rather than merely discouraged.

## In Scope

- Values that originate outside the user's control and end up in a filesystem
  path built by `filepath.Join`, `path.Join`, string concatenation, or
  equivalent.
- Values that originate outside the user's control and end up in text a shell
  evaluates: activation output, hook snippets, generated scripts, shim
  wrappers, anything `eval`'d or written to a file a shell sources.
- The guards that exist today and precisely which sinks they cover, so gaps are
  demonstrated rather than asserted.
- Claims about these controls in the committed design record.

## Out of Scope

- A general audit of tsuku. Anything that is neither a path component nor
  shell-evaluated text is not this exploration's business, however interesting.
- tsuku#2543's own subject: non-exact pin resolution, `latest`/prefix handling,
  and the non-exact-pin reporting contract. Owned by `tsuku_activation_pins`.
- The `/`-walk discovery scope defect (`.tsuku.toml` discovery walks to `/`, so
  a file in `/tmp` applies to every hooked shell beneath it). Real, medium
  severity, changes `internal/project` discovery semantics, wants its own owner.
  File it if not already filed; do not fix it here.
- Memory-safety, supply-chain, or network-transport concerns.
- Exploitation tooling. Reproductions belong in the issue; no exploit harness
  enters the repository.

## Research Leads

1. **Which externally-supplied values reach filesystem path construction as a
   path component, and which of them are validated?** (lead-path-sinks)
   `ToolBinDir(name, version)` is the known one. Sweep every path composition
   whose components come from `.tsuku.toml`, recipe TOML, the registry, state
   files, env vars, or CLI arguments, and record for each whether a guard runs
   before the join. The question is not "is there a `filepath.Join`" but "can a
   component carry `..` or an absolute path".

2. **Where else does tsuku emit text that a shell evaluates, and how is each
   emission quoted?** (lead-shell-sinks)
   `FormatExports` is the known one. Find every place that writes shell syntax:
   hook install snippets, the shell.d init cache, `tsuku shell`, `hook-env`,
   doctor output that suggests commands, generated shims or wrappers, anything
   templated into a `.sh`/`.fish` file or passed to `sh -c`. For each, record
   the quoting used and whether an interpolated value can be attacker-supplied.
   `%q` used as a shell quoter is the specific belief being chased.

3. **What is the real inventory of attacker-controlled input, and how far does
   each reach?** (lead-input-inventory)
   Enumerate what a repository the user cloned can actually set: every field of
   `.tsuku.toml`, not just tool names and versions. Then separately, what a
   recipe from the registry or a distributed recipe from GitHub can set. These
   are different trust levels and should not be averaged. The output is a table
   of input -> the sinks it can reach.

4. **What validation exists today, what does each guard actually cover, and
   where does a guard stop short of the composition it appears to protect?**
   (lead-guard-coverage)
   Map `internal/validate`, `internal/version` pin validation, `internal/recipe`
   validation, and `internal/project`'s `MaxTools` onto the sinks from leads 1
   and 2. The `ToolBinDir` asymmetry is the template: a guard on one component
   of a composed value and none on the other. Look for the same shape
   elsewhere. Also record which guards run at parse time versus at use time,
   since that determines blast radius.

5. **Does the same class reach process execution rather than shell text?**
   (lead-exec-sinks)
   Narrow extension of lead 2: values that become an `os/exec` argv, an
   interpreter argument, a generated script body, or an environment variable
   fed to a child process. Argv is not shell-evaluated and is much weaker, so
   the point is to bound this and say so, not to grow the finding list. Report
   only cases where a shell is genuinely in the loop or where a value can
   become a flag rather than an operand.

6. **What do the committed design and test records claim about these controls,
   and is the line-419 row the only false claim?** (lead-record-claims)
   Read `docs/designs/current/DESIGN-shell-env-activation.md` in full, plus any
   other design doc or security section asserting path constraint or name
   validation. Check each asserted control against the code. Also assess the
   existing test surface in `internal/shellenv` and `internal/project`: does any
   test claim to cover name handling, and would it catch a wrong implementation
   or merely illustrate the right one?

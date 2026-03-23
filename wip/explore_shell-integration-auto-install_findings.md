# Exploration Findings: shell-integration-auto-install

## Core Question

Does the current 5-block design adequately deliver the use case where a project
declares its tools in `tsuku.toml`, and those tools are automatically installed
on first use -- across both interactive shells and non-interactive contexts
like scripts and CI?

## Answer: No. The design has a concrete architectural gap.

The 5-block design correctly identifies building blocks but leaves the convergence
between Track A (auto-install) and Track B (project config) unresolved. The current
design says "Block 3's detailed design should accept an optional ProjectConfig
parameter" -- but that's insufficient. The two tracks are triggered at different
execution points and never converge at command invocation time in non-interactive
contexts.

---

## Lead Findings

### Lead 1: Trigger Models

Command-not-found hooks only work in interactive shells. In scripts and CI, there
is no shell hook mechanism.

For non-interactive contexts, two models work:
- **Explicit wrapper** (`tsuku run <cmd>`): works everywhere, requires command changes
- **Shims** (volta-style): transparent, works everywhere, ~15-30ms overhead per command

No tool in the ecosystem solves "on first use, no hooks, no command changes"
without shims or a wrapper invocation. tsuku's requirement is stricter than what
existing tools deliver.

**Key insight:** For scripts and CI, the most reliable pattern is either `tsuku run`
or `eval $(tsuku shell)` once at the top of the script. Full transparency requires
shims.

### Lead 2: Binary Index and Project Config are Separate Concerns

The binary index (issue #1677) answers: "user typed unknown command `jq` → which
recipe provides it?" It is a discovery mechanism for unknown commands.

The project config path answers: "tsuku.toml says `koto = >=0.3` → install it."
The recipe name is already known. No reverse lookup needed.

**The binary index is NOT a prerequisite for the tsuku.toml auto-install use case.**

These are two complementary lookup paths:
- Path A (discovery): command name → binary index → recipe → install
- Path B (project-declared): recipe name in tsuku.toml → direct install

Track B (project config, issues #1680/#1681) can be implemented in parallel
with or before Track A's binary index. The parent design's dependency graph
already shows this (no arrow from #1677 to #1680), but it's worth making
explicit.

### Lead 3: Comparable Tools

- **mise**: auto_install requires prompt hooks; CI needs explicit `mise install`
- **asdf**: no native auto-install; always manual
- **devbox**: `devbox run <cmd>` wrapper; works in CI but requires command changes
- **volta**: shim architecture, transparent install-on-run, no command changes -- closest to the target UX
- **nix-shell**: `--run` pattern, works in CI, requires command changes

Every tool satisfying "works in CI" requires either command changes (`devbox run`, `nix-shell --run`) or shims (volta). None satisfy all three: no hooks, no command changes, works in CI -- without shims.

### Lead 4: The Missing Convergence Block

**The flow breaks here:** Developer types `koto` in a project with tsuku.toml.
No shell hooks are set up.

1. Command-not-found handler does not fire (no hooks)
2. Binary index is not consulted
3. `tsuku.toml` is never read
4. Shell fails with "command not found"

Block 3 cannot "just accept ProjectConfig" because it only fires when the user
explicitly runs `tsuku run koto` -- which requires the developer to know to use
that wrapper. In scripts and CI, that's a manual wrapping requirement.

**The architectural fix is a 6th block: Project-Aware Exec Wrapper.**

`tsuku exec <command> [args]`:
1. Reads tsuku.toml from current directory upward
2. If command declared, ensures correct version is installed
3. Consults binary index if command not in tsuku.toml (falls back to Track A)
4. Executes the command

Plus optional shim generation: `tsuku shim install` creates thin wrappers in
`$TSUKU_HOME/bin/` for each project-declared tool. Shims call `tsuku exec` under
the hood. Once shims are on PATH, the developer's commands are transparent.

The convergence diagram becomes:

```
User types "koto"
    ↓
[If shim installed]: Block 6 shim intercepts
[If no shim]: tsuku exec koto (explicit)
    ↓
Block 6: LoadProjectConfig(tsuku.toml)
    ├── koto declared → use version constraint (>=0.3)
    └── koto not declared → fall through to Block 1 binary index
    ↓
Block 3: Auto-Install (install constrained version if needed)
    ↓
Exec binary
```

### Lead 5: Koto/Shirabe Integration

koto has zero tsuku references and relies entirely on PATH. shirabe's README
claim that "koto is installed automatically on first skill invocation" is
aspirational -- there is no implementation behind it.

The integration model is entirely tsuku-side. koto does not need to call tsuku.
The recipe for koto already exists in tsuku's registry (`.tsuku-recipes/koto.toml`).
The missing piece is on tsuku's side: the project config + exec wrapper that
ensures koto is installed before it's invoked.

koto doesn't change. tsuku delivers the behavior.

---

## Consolidated Gap Analysis

| Gap | Severity | Current Design Handles? | Fix |
|-----|----------|------------------------|-----|
| No trigger for project-declared tools in CI | **Critical** | No | Block 6 (tsuku exec + shims) |
| Binary index not needed for project-declared tools | Low (design clarity) | Partially (tracks are independent, but design doesn't say it clearly) | Clarify in parent design |
| Track A / Track B never converge at command invocation | **Critical** | No (only at shell setup) | Block 6 |
| Shell activation (Block 5) insufficient for scripts/CI | High | No | Block 5 + eval $(tsuku shell) for scripts; shims for full transparency |
| koto/shirabe have no tsuku integration | Known gap | Aspirational (README) | tsuku delivers, koto unmodified |

---

## What the Design Gets Right

- The block decomposition is correct for the features each block delivers independently
- The parallel track structure is sound (Track B has no dependency on Track A)
- Issue #1677 (binary index) is correctly scoped for Track A
- The 50ms performance budget is appropriate
- Shell hooks as optional (not required) is the right stance

---

## Decision: Crystallize

The gaps are identified, the solutions are concrete, and the next steps are clear.
No further research rounds needed.

The exploration converges on: **update the parent design to add Block 6 and file
a new issue for Block 6's detailed design.** This is a design document update
with a concrete new issue.

# Research: Shell Integration Auto-Install Convergence Architecture
**Conducted by:** Lead Convergence Block Analysis  
**Date:** March 22, 2026  
**Question:** Is there a missing 6th block in the 5-block design -- "project-aware command dispatch" -- and how should the convergence between Track A (auto-install) and Track B (project config) be structured?

---

## Executive Summary

The 5-block design has a real architectural gap: **Track A (auto-install) and Track B (project config) are not actually convergent without a new primitive**. The statement "Block 3's detailed design should accept an optional ProjectConfig parameter" is necessary but insufficient. This research identifies why the tracks diverge, examines how comparable tools solve this, and proposes a structured set of alternatives for proper convergence.

**Key finding:** The tracks are triggered at different execution points:
- **Track A** (auto-install) triggers on unknown command execution via shell hook or `tsuku run`
- **Track B** (project config) triggers on shell setup via prompt hook or explicit `tsuku shell`

These are orthogonal flows. Block 3 cannot "just accept ProjectConfig" without changing how tools discover which version to install. The gap is **project-aware dispatch at command invocation time**, not just at shell initialization.

---

## Part 1: Current Flow Analysis

### Scenario: Developer types `koto` in a project with `tsuku.toml`

**Setup:**
- Project has `tsuku.toml` declaring: `tools = { koto = ">=0.3" }`
- No shell hooks are set up (non-interactive CI context assumed)
- `koto` is not installed
- Developer runs just `koto [args]` (no `tsuku run` wrapper)

**What happens with current 5-block design:**

| Flow Step | Track A (auto-install) | Track B (project config) | What Actually Happens |
|-----------|------------------------|--------------------------|----------------------|
| 1. User types `koto` | Shell command-not-found handler fires (if installed) | — | **No handler** if no shell hooks; command fails |
| 2. Handler invokes `tsuku suggest koto` | Binary Index looks up `koto` | — | Index returns "recipe: koto" (no version info) |
| 3. Handler suggests installation | Suggests `tsuku install koto` | — | User sees suggestion but not version constraint |
| 4. OR: User runs `tsuku run koto [args]` | Block 3: Auto-Installer checks Binary Index | — | Auto-Installer installs latest `koto` |
| 5. Installation uses version | **Block 3 does NOT check project config** | Project config sits unused | ❌ **Wrong version installed** (latest ≠ >=0.3 declared in tsuku.toml) |
| 6. Shell activation for next session | — | Block 5: Prompt hook would fix it next shell session | ❌ **Too late** for this invocation |

**The core problem:** Block 3 (Auto-Installer) and Block 4 (Project Config) are separate trigger points:
- Block 3 triggers on command execution (too late to ask shell)
- Block 4 triggers on shell entry (too early for command execution)

There is **no single point** where both are consulted for a command invocation in a script/CI context.

---

## Part 2: How Comparable Tools Handle This

### mise (parallel track design)
- **Project config file:** `.mise.toml` (or `.tool-versions`)
- **Auto-install model:** `auto_install = true` enables installation on first use
- **Trigger:** Prompt hook (`PROMPT_COMMAND` on bash, `chpwd` on zsh)
- **Key difference:** mise **only** uses prompt hooks for auto-install. There is no command-not-found handler. If hooks aren't installed, `tsuku.toml` tools don't auto-install at all.
- **CI workaround:** Developers must explicitly call `mise install` in CI (manual invocation)
- **Trade-off:** Simple architecture, but requires hook setup for "on first use" behavior

### asdf (plugin-based)
- **Project config file:** `.tool-versions` (flat format, no version constraints)
- **Auto-install:** No native auto-install. Plugins must implement it or users run `asdf install` manually
- **Shim mechanism:** Shim scripts in PATH wrap command execution (~120ms overhead pre-Go)
- **CI model:** Same as mise — developers manually invoke `asdf install` in CI scripts
- **Trade-off:** Mature ecosystem but requires active plugin development

### direnv (orthogonal approach)
- **Not a package manager** — uses external tools (nix, brew, asdf, etc.)
- **Trigger:** `.envrc` file, executed on `cd` via shell hook
- **Project awareness:** Arbitrary shell code execution, can call `asdf install` or similar
- **CI model:** Developers must source `.envrc` manually or set `DIRENV_NOSHELL=1` for non-shell contexts

### devbox
- **Model:** Container-based isolation, Nix-backed
- **Trigger:** Prompt hook on `cd`
- **CI model:** Container image includes pre-installed tools
- **Not directly comparable** — different architecture, not relevant to tsuku's design

### Observation: **None of these tools solve "on-first-use in CI without hooks"**

All comparable tools require either:
1. Hook setup (mise, devbox), OR
2. Manual invocation (asdf, direnv), OR
3. Container/Nix pre-provisioning (devbox)

**Tsuku's constraint is stricter:** "must work in CI without hooks and without developer changing commands."

---

## Part 3: Do We Actually Need the Binary Index for Project-Declared Tools?

### Analysis: Two Separate Lookup Paths

**Path 1: Unknown command discovery (Block 1 + Block 2 + Block 3)**
- User types `jq` (command they don't know)
- Need: reverse lookup from command name → recipe
- **Requires Binary Index** to map `jq` → recipe

**Path 2: Project-declared tools (Block 4 + Block 5)**
- Project declares `tools = { jq = ">=1.6" }` in `tsuku.toml`
- Need: recipe name → install latest matching version
- Recipe name is **already known** — no reverse lookup needed
- **Does NOT require Binary Index**

**Implication:** Issue #1677 (Binary Index design) is essential for Block 2+3 (command interception) but **orthogonal** to Block 4+5 (project config). These are genuinely independent tracks with separate value propositions.

**Binary Index is foundational for discovery**, but project-declared tools use a **different path** (config file tells us recipe name directly).

---

## Part 4: The Missing Block — "Project-Aware Command Dispatch"

### The Convergence Gap

The design says: "Block 3's detailed design should accept an optional ProjectConfig parameter."

**What this actually means (and what's missing):**
- Block 3 installs a tool when user runs `tsuku run koto [args]`
- Block 3 should check if `koto` is declared in `tsuku.toml` with a version constraint
- If declared, install the constrained version, not latest
- If not declared, install latest

**But Block 3 as described doesn't have a trigger for "project-aware" behavior in non-interactive contexts.** The assumption is always that user invokes `tsuku run` explicitly. In scripts or CI, how does the command get wrapped with `tsuku run`?

**The missing piece is a wrapper or dispatch layer** that:
1. Intercepts command invocation (not just shell hooks)
2. Consults project config
3. Installs required versions
4. Executes the command

This could be:
- A shim in PATH (asdf-style, adds ~10-50ms overhead)
- An `exec` wrapper that replaces command with `tsuku exec <cmd>`
- A process substitution or alias wrapper
- Or simply: Block 3 gains a "project-aware" mode that developers use explicitly

---

## Part 5: Proposed Alternatives for Convergence

### Alternative A: "Enhance Block 3 with ProjectConfig Reading"

**Mechanism:**
- Extend `tsuku run` to read `tsuku.toml` from current directory upward
- If tool is declared, use declared version constraint instead of latest
- If tool not declared, use latest (current behavior)

**Command:** `tsuku run koto [args]`

**Before/After:**
```bash
# Before: installs latest koto
$ tsuku run koto --help
# (installs koto v0.4 even if tsuku.toml says >=0.3,<0.4)

# After: respects project config
$ tsuku run koto --help
# (installs koto v0.3.2 if that's what tsuku.toml declares)
```

**Trigger:** Explicit invocation by developer
**Implementation:** Simple — add `LoadProjectConfig()` call in Block 3's RunCommand handler
**Tradeoff:** 
- ✅ Works in scripts/CI (user can wrap commands)
- ❌ Requires developer to know to use `tsuku run` (breaks "don't change your commands")
- ❌ Inefficient in scripts with many invocations (repeats install check each time)

**Convergence Quality:** **Partial** — satisfies "consult project config" but breaks "no command changes"

---

### Alternative B: "Enhance Block 5 with Eager Installation + Add Optional Block 2/3 Hooks"

**Mechanism:**
- Block 5 shell activation installs project tools eagerly (on `tsuku shell` or prompt hook)
- Block 2+3 (command-not-found hooks) are purely optional enhancements
- For scripts/CI without hooks, developers must explicitly run `tsuku shell` before main script

**Command:** `eval $(tsuku shell)` once at start of script/session

**Before/After (CI context):**
```bash
# Before: no project activation
$ koto --version  # fails, koto not installed

# After: explicit activation
$ eval $(tsuku shell)
$ koto --version  # works, koto@0.3.2 from tsuku.toml
```

**Trigger:** Explicit shell activation
**Implementation:** Block 5's detailed design ensures tools are installed
**Tradeoff:**
- ✅ Works in scripts/CI (one-time invocation)
- ✅ Single source of truth (shell activation manages versions)
- ❌ Requires one-time setup in each script (not transparent)
- ❌ Adds output/shell code generation to Block 5 (may be complex)

**Convergence Quality:** **Good** — works reliably, but requires explicit activation

---

### Alternative C: "Add Block 6: Project-Aware Command Shim"

**Mechanism:**
- New Block 6: Shim executable(s) in `$TSUKU_HOME/bin/` that wrap common commands
- Shim checks if command is declared in `tsuku.toml`, installs if needed, then execs real binary
- Works for any command in `tsuku.toml`

**Example shim for `koto`:**
```bash
#!/bin/bash
# $TSUKU_HOME/bin/koto (shim)
exec tsuku exec koto "$@"
```

**tsuku exec** (new command):
```bash
tsuku exec <command> [args]
  1. LoadProjectConfig from current dir
  2. If <command> in config, ensure version installed
  3. Exec real binary from $TSUKU_HOME/tools
```

**Setup (one-time, in project):**
```bash
# In CI/project root, create shims for project tools
for tool in $(tsuku list-project-tools); do
  ln -s $(tsuku which-shim) $TOOL_PATH/$tool
done
```

**Before/After (CI context):**
```bash
# Before: script must wrap commands
$ tsuku run koto --help
$ tsuku run shirabe --help

# After: shims handle it transparently
$ koto --help           # finds shim, installs v0.3.2, execs
$ shirabe --help        # finds shim, installs from tsuku.toml
```

**Trigger:** Automatic (shim in PATH)
**Implementation:** 
- New `tsuku exec` command
- Optional shim generation for project tools
- ~15-30ms overhead per command (negligible for most workflows)

**Tradeoff:**
- ✅ Transparent (no command changes needed)
- ✅ Works in scripts/CI without explicit setup
- ✅ Respects project config on every invocation
- ❌ Adds complexity (new command, shim setup)
- ❌ Requires project-level setup or auto-shim generation
- ❌ ~15-30ms overhead per command invocation

**Convergence Quality:** **Excellent** — solves all constraints, minimal overhead

---

### Alternative D: "Shift Responsibility to Tools (Koto/Shirabe Call tsuku)"

**Mechanism:**
- Koto and Shirabe detect they're running in a tsuku-managed project
- On startup, they call `tsuku ensure-installed <self> [args]` to check/install
- Then proceed with normal execution

**Implementation:**
- Koto startup: `tsuku ensure-installed koto` before parsing flags
- Returns immediately if already installed, installs if needed
- No overhead for repeated invocations (install is cached)

**Before/After (project with koto invocation):**
```bash
# Before: koto relies on being pre-installed
$ koto next my-workflow
# (fails if koto not installed)

# After: koto self-installs if needed
$ koto next my-workflow
# (checks tsuku.toml, installs v0.3.2 if missing, proceeds)
```

**Trigger:** Koto/Shirabe startup code
**Implementation:**
- Add `tsuku ensure-installed` command
- Koto/Shirabe call it (internal, not user-facing)
- Minimal overhead (~10ms cached lookup)

**Tradeoff:**
- ✅ Transparent to users
- ✅ Works in scripts/CI without setup
- ✅ Respects project config
- ❌ Requires cooperation from Koto/Shirabe teams
- ❌ Creates circular dependency (koto depends on tsuku, tsuku defines koto recipe)
- ❌ Not a general solution (only works if tools implement this pattern)

**Convergence Quality:** **Very Good (specific to Koto/Shirabe)** — solves the koto+shirabe case elegantly, but not general

---

## Part 6: Assessment Against Design Constraints

All scenarios must satisfy:
1. **No shell hooks required** (works in CI/scripts)
2. **On first use** (implicit install, not manual)
3. **No command changes** (user doesn't wrap with `tsuku run`)
4. **Works in non-interactive contexts** (scripts, CI, cron)

| Alternative | No Hooks | First-Use | No Cmd Change | Non-Interactive | Overall |
|-------------|:--------:|:---------:|:-------------:|:---------------:|:-------:|
| A: Enhance Block 3 | ✅ | ✅ | ❌ | ✅ | Partial |
| B: Enhance Block 5 + hooks optional | ✅ | ❌ (requires setup) | ✅ | ❌ (setup needed) | Weak |
| C: Block 6 Shim (tsuku exec) | ✅ | ✅ | ✅ | ✅ | **Full** |
| D: Koto/Shirabe self-install | ✅ | ✅ | ✅ | ✅ | **Full** (but limited scope) |

---

## Part 7: Koto/Shirabe Integration Model

Based on koto's design documents, **koto does not currently call tsuku**:
- Koto is distributed via `install.sh`, curl|sh, and `tsuku install koto` (recipe exists in tsuku)
- Koto is a standalone binary that manages workflow state (`koto-<name>.state.jsonl` files)
- Koto does not invoke `tsuku` during execution
- Koto doesn't have a concept of "tool requirements" beyond being present on PATH

**Implication:** The D (self-install) approach would require adding this integration point to koto, which is possible but not currently designed.

---

## Part 8: Recommendation

The 5-block design **is incomplete without a 6th block or architectural clarification**. Here are the options:

### Option 1 (Recommended): **Define Block 6 — Project-Aware Exec Wrapper**

Add a new block that provides the missing dispatch layer:
- `tsuku exec <command> [args]` — wraps execution with project config awareness
- Optional shim setup for transparent invocation
- Works as a bridge between Block 3 (auto-install) and Block 4 (project config)

**Why:** Cleanest architectural solution, true convergence, minimal overhead, works in all contexts.

**Design Impact:** Block 6 detailed design should define:
- `tsuku exec` command semantics and project config consultation
- Optional shim generation and management
- Error handling (command not in config, version constraints fail, etc.)
- Performance budget (target <50ms for cached installs)

---

### Option 2 (Lighter): **Block 3 Detailed Design Clarifies ProjectConfig Integration**

Enhance Block 3's detailed design to explicitly address:
1. **When to read project config:** Every `tsuku run` invocation checks `tsuku.toml` upward
2. **Version selection logic:** If tool declared → use constraint; else → latest
3. **Explicit wrapper recommendation:** Document that scripts should use `tsuku run` for project awareness
4. **CI pattern:** Recommend `tsuku run` as the wrapping mechanism

**Why:** Lighter lift, leverages existing Block 3, clear guidance without new architecture.

**Trade-off:** Doesn't fully satisfy "no command changes" — scripts still need to wrap with `tsuku run`.

---

### Option 3 (Koto-Specific): **Implement D (Koto Self-Install)**

If koto+shirabe are the primary use case:
- Add `tsuku ensure-installed` command
- Integrate into koto startup (and shirabe, if applicable)
- Creates self-healing behavior for project-scoped tools

**Why:** Leverages koto's position as a sibling project, elegant for the specific case.

**Trade-off:** Only works for tools that cooperate, not a general mechanism.

---

## Conclusion

The 5-block design successfully identifies independent building blocks but **leaves the convergence point underspecified**. The statement "Block 3's detailed design should accept an optional ProjectConfig parameter" is correct but insufficient—it doesn't define how project-declared tools get installed in script/CI contexts without hooks.

**The core issue:** Block 4 (Project Config) and Block 3 (Auto-Install) are triggered at different times:
- Block 4 at shell setup (too early for command invocation)
- Block 3 at command execution via hook or explicit `tsuku run` (requires hook or manual wrapping)

**Recommendation:** Define Block 6 (Project-Aware Exec Wrapper) as a new building block that provides the missing dispatch point. This enables transparent, hook-free, project-aware command invocation in all contexts (interactive shell, scripts, CI).

---

## Appendix: Flow Diagrams

### Current 5-Block Design (Divergent)

```
┌─ Track A: Command Interception ───┐       ┌─ Track B: Project Config ───┐
│                                   │       │                             │
│  User types "koto"                │       │  Project declares koto in   │
│       ↓                           │       │  tsuku.toml                 │
│  (If hook installed)              │       │       ↓                     │
│  Block 2: command_not_found       │       │  Block 4: LoadProjectConfig │
│       ↓                           │       │       ↓                     │
│  Block 1: Binary Index lookup     │       │  Block 5: Shell activation  │
│  (finds recipe, no version info)  │       │  (installs version,         │
│       ↓                           │       │   modifies PATH)            │
│  Block 3: Auto-Install            │       │       ↓                     │
│  (installs latest, might be       │       │  Triggered on prompt hook   │
│   WRONG version)                  │       │  or `tsuku shell`           │
│                                   │       │  (too late for this session)│
└───────────────────────────────────┘       └─────────────────────────────┘

❌ PROBLEM: Tracks never converge at command invocation time.
           Block 3 doesn't consult Block 4.
           Block 5 doesn't affect this invocation.
```

### With Block 6 (Proposed)

```
┌─────────────────────────────────────────────────────────┐
│  User types "koto"                                      │
│       ↓                                                 │
│  Block 6: Project-Aware Exec (tsuku exec koto [args])  │
│       ├→ LoadProjectConfig (from tsuku.toml)           │
│       │  (if koto declared, read version constraint)   │
│       ├→ Consult Block 1 Binary Index (recipe lookup)  │
│       │                                                │
│       ├→ Block 3: Auto-Install                         │
│       │  (installs constrained version if needed)      │
│       │                                                │
│       └→ Exec binary with correct PATH                 │
│                                                        │
│  ✅ CONVERGED: Both tracks consulted at invocation time │
└─────────────────────────────────────────────────────────┘
```


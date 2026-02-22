# Architect Review: DESIGN-gem-exec-wrappers.md

**Reviewer**: architect-reviewer
**Date**: 2026-02-22
**Design doc**: `docs/designs/DESIGN-gem-exec-wrappers.md`
**Code under review**:
- `internal/actions/gem_exec.go` lines 470-492 (broken symlink path)
- `internal/actions/gem_install.go` lines 218-234 (working wrapper path)

---

## 1. Is the problem statement specific enough?

Mostly yes. The problem statement correctly identifies two distinct bugs:

1. **Runtime failure**: bare relative symlinks from `bin/<exe>` to `ruby/<ver>/bin/<exe>` point at bundler-generated scripts that use `#!/usr/bin/env ruby`. At runtime those scripts resolve whatever `ruby` is on `$PATH`, which has no knowledge of the isolated `GEM_HOME`, so gem lookup fails.

2. **Secondary install-time failure**: `install_binaries` looks for `.gem/bin/<exe>` (hardcoded prefix) but the real path is `ruby/<ver>/bin/<exe>`, so checksum verification fails with `lstat ... no such file or directory`.

The statement is specific enough to evaluate solutions against. The error message (`Gem::GemNotFoundException`) is shown verbatim. The 83-gem blast radius is quantified.

**One gap**: the problem statement doesn't explain *why* the decomposed path creates symlinks while the direct path creates wrappers. Knowing that `gem_install`'s direct execution path was written first, and `gem_exec`'s `executeLockDataMode()` was added later without carrying the wrapper pattern forward, would make it clear this is an omission rather than a deliberate design choice. Including that history would strengthen the case for the chosen fix and preempt the question "why didn't the original author do this?"

---

## 2. Are there missing alternatives?

The two decisions covered are:
- Decision 1: How to create working executables (wrappers vs. symlinks + something else)
- Decision 2: How to find the ruby bin dir at install time

**One legitimate missing alternative for Decision 1**: the design doesn't consider using `binstubs`. Bundler has a `bundle binstubs <gem>` command that generates stub scripts in `bin/` with the bundler environment already configured. These would eliminate the need for a custom bash wrapper template entirely. The rejection rationale would likely be: binstubs depend on the presence of the `Gemfile`/`.bundle/config` at runtime rather than baking environment variables into the script, which reduces self-containment. Worth explicitly evaluating and rejecting rather than omitting.

**One missing alternative for Decision 2**: for the system-bundler fallback case (see below), the design has an unaddressed gap rather than a missing alternative per se.

No other meaningful alternatives appear to be missing. The three options covered (env vars in TOML, absolute symlinks, bash wrappers) span the reasonable solution space for Decision 1.

---

## 3. Is the rejection rationale specific and fair?

### Option 1A rejected: Environment variables in recipe TOML

Rejection: "pushes complexity into every recipe (83 files)... can't handle relocatability since paths would be hardcoded at recipe authoring time."

**Fair and specific.** The first point (83-file scatter) is accurate and quantified. The second point (hardcoded paths at authoring time) is correct -- recipe authors don't know the user's `$TSUKU_HOME`. This option would require either a template substitution mechanism that doesn't exist, or literal paths that break on any non-default install. Rejection holds.

### Option 1B rejected: Absolute symlinks

Rejection: "doesn't solve the GEM_HOME/GEM_PATH problem... absolute symlinks also break relocatability."

**Fair but incomplete.** Both points are correct. However the rejection doesn't mention that the `install_binaries` secondary bug would also remain unfixed by this option -- the secondary bug depends on *what* the file at `bin/<exe>` is, not just whether it runs. A complete rejection would note both bugs persist.

### Decision 2 alternatives

**Rejected: Search for ruby directly via LookPath or glob**

Rejection: "`findBundler()` is more reliable -- if bundler was found, ruby must be in the same directory."

**Specific, but carries an unstated assumption** (see section 4).

**Rejected: Store ruby path during decomposition**

Rejection: "adds a new parameter to the action schema and the ruby installation path might change between plan generation and execution."

**Fair and specific.** The second reason (path could change) is particularly solid -- the design correctly identifies that discover-at-execution-time is more resilient than record-at-plan-time.

---

## 4. Unstated assumptions that need to be explicit

### Assumption A: `findBundler()` always returns a tsuku-managed ruby path

This is the most significant unstated assumption in the design. `findBundler()` has a system fallback:

```go
// Try system bundler
path, err := exec.LookPath("bundle")
if err == nil {
    return path
}
```

If the tsuku ruby is not installed (or `ctx.ToolsDir` is empty), `findBundler()` returns the system bundler (e.g., `/usr/bin/bundle`). The proposed fix then does:

```go
rubyBinDir := filepath.Dir(bundlerPath)  // e.g., /usr/bin
```

The generated wrapper would then contain:
```bash
export PATH="/usr/bin:$PATH"
exec ruby "$SCRIPT_DIR/<exe>.gem" "$@"
```

This hardcodes a system path into the wrapper at install time. If the system's ruby moves, or if the user's environment changes, the wrapper breaks. More critically, it embeds a non-relocatable path in a context where the design claims relocatability as a goal.

The design should either:
a. Explicitly state "this fix assumes tsuku's ruby is installed and `findBundler()` returns a tsuku path," and add a guard that errors if only system bundler is found, or
b. Use a discovery mechanism at wrapper runtime instead of hardcoding the ruby bin dir at install time.

This assumption is load-bearing because it affects correctness guarantees. It should be explicit.

### Assumption B: Bundler-generated scripts are always at `ruby/<ver>/bin/<exe>`

The design mentions `findBundlerBinDir()` with a fallback list of four patterns. The wrapper template always calls `exec ruby "$SCRIPT_DIR/<exe>.gem"`, which means `<exe>.gem` must be in the same directory as the wrapper itself (`bin/`). But `findBundlerBinDir()` could return a `gems/*/exe/` path for certain bundler versions. In that case `binDir` != `bin/` and the rename-to-.gem step would put `<exe>.gem` somewhere other than where the wrapper expects it.

The design should state explicitly: "the wrapper template assumes the renamed `.gem` script lives in the same directory as the wrapper." If `findBundlerBinDir()` returns a non-`bin/` path, the implementation must copy the script to `bin/<exe>.gem` rather than rename it in place.

### Assumption C: The `.gem` rename doesn't conflict across multiple gems

If two gems share an executable name (pathological but worth noting), the rename loop would silently overwrite. This is a minor correctness note, not a structural one.

### Assumption D: Bash is available at runtime

The design correctly acknowledges this in the Consequences/Negative section. It notes bash is already a requirement. This is adequately stated.

---

## 5. Is any option a strawman?

### Option 1A (TOML env vars) is borderline

The description says paths "would be hardcoded at recipe authoring time," which slightly misrepresents the option. A competent implementation of TOML env vars would use tsuku template variables (e.g., `$INSTALL_DIR`) substituted at install time, not literal paths. The design rejects this on the correct first-order reason (83-file scatter) but uses a weaker secondary reason that doesn't fully credit the option.

This isn't a full strawman -- the primary rejection reason (84-file scatter vs. one-function fix) is accurate and sufficient. But the secondary reason makes the option look weaker than it is.

### Option 1B (absolute symlinks) is not a strawman

Both reasons for rejection are accurate, and the option genuinely can't solve either bug in isolation. No concern here.

### Decision 2 alternatives are not strawmen

Both are rejected on fair technical grounds.

---

## Summary of Findings

| Finding | Severity | Notes |
|---------|----------|-------|
| System-bundler fallback hardcodes non-tsuku path into wrapper | **Blocking** | Assumption A -- contradicts relocatability goal; needs explicit guard or runtime ruby discovery |
| `.gem` script location assumption breaks for non-`bin/` bundler layouts | **Advisory** | Assumption B -- implementation must handle the copy-vs-rename case |
| Missing bundler binstubs alternative | **Advisory** | Not evaluating it leaves a gap; one sentence rejection would suffice |
| Option 1B rejection doesn't mention secondary bug remains | **Advisory** | Completeness; doesn't affect the correctness of the chosen option |
| Problem statement missing history of why decomposed path lacks wrappers | **Advisory** | Clarity for reviewers; doesn't affect the design decision |

### Recommendation

The chosen option (bash wrappers matching `gem_install`, ruby bin dir from `findBundler()`) is structurally correct and consistent with the existing pattern. The architectural fit is good: it reuses a proven mechanism, changes a single function, and doesn't introduce a parallel pattern.

The one blocking concern is the system-bundler fallback path. The implementation should add an explicit check: if `findBundler()` returns a path outside `ctx.ToolsDir`, either error out with a message like "gem_exec requires tsuku-managed ruby; system ruby found at X" or use runtime PATH resolution in the wrapper instead of a hardcoded directory. The current design text implies the hardcoded path will always be a tsuku-managed path, but the code doesn't enforce that.

The optional extraction of the wrapper template to `gem_common.go` mentioned in the Implementation Approach section is the right call and should be non-optional. Without it, the two templates will drift as soon as someone fixes a bug in one without updating the other. Given that both paths are supposed to produce identical results, diverging templates are a maintenance liability.

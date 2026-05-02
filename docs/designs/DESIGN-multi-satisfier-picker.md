---
status: Planned
problem: |
  `tsuku install <name>` cannot route the user through a choice when an alias
  is claimed by multiple recipes. The four-recipe OpenJDK family in PR #2362
  needs `tsuku install java` to present openjdk, temurin, corretto, and
  microsoft-openjdk so the user picks one.
decision: |
  Add a non-ecosystem `aliases` key to `[metadata.satisfies]` and a parallel
  `aliasIndex` in the loader (multi-valued, distinct from the existing 1:1
  `satisfiesIndex`). Add a new `internal/tui` package that wraps
  `golang.org/x/term` plus hand-rolled ANSI for the picker. Hook the install
  command's resolution flow before the discovery fallback, branching on
  satisfier count (0 → discovery, 1 → auto-resolve, 2+ → picker on TTY or
  error under `-y`/no-TTY). A hidden `--pick` flag short-circuits the picker
  for CI tests. Validator and registry index-build both reject
  multi-satisfier aliases in `runtime_dependencies`.
rationale: |
  Two index data structures (existing 1:1 ecosystem-keyed + new multi-valued
  alias) cleanly separates the new behavior from the established one without
  changing semantics for any existing recipe. Hand-rolled ANSI in
  `internal/tui` matches the established `internal/progress` spinner pattern
  and avoids a multi-megabyte TUI dependency for a single-select prompt.
  Hooking before discovery preserves the curated > discovery priority that's
  consistent across tsuku.
upstream: docs/prds/PRD-multi-satisfier-picker.md
---

# DESIGN: Multi-Satisfier Alias Picker

## Status

Planned

## Context and Problem Statement

`tsuku install <name>` resolves a name through three layers today:

1. **Direct provider lookup** — `loader.Get(name)` walks the provider chain
   (local file, registry, embedded). Returns the loaded recipe if any
   provider has a file at `recipes/{first-letter}/{name}.toml`.
2. **Satisfies fallback** — if step 1 fails, the loader's
   `satisfiesIndex` (a `map[string]satisfiesEntry` keyed by package name)
   is consulted. The current type is one-entry-per-key: each ecosystem
   declaration like `[metadata.satisfies] homebrew = ["openjdk"]` adds
   exactly one mapping `openjdk → openjdk`. Two recipes claiming the
   same package name would collide; the build is first-wins by provider
   priority.
3. **Discovery fallback** — `tryDiscoveryFallback` in `cmd/tsuku/install.go`
   probes external registries (npm, PyPI, RubyGems, Homebrew, crates.io).
   If multiple ecosystems return hits for the typed name, an
   `AmbiguousMatchError` is raised and rendered as
   `Multiple sources found for "X". Use --from to specify`.

PR #2362 ships four curated recipes that are all valid answers to "give me
Java" (`openjdk`, `temurin`, `corretto`, `microsoft-openjdk`). The current
satisfies index can register only one of them against the alias `java`.
Today's behavior on `tsuku install java` is the discovery error pointing at
unrelated rubygems and npm packages — neither matches the four-vendor
choice the milestone exists to provide.

PRD-multi-satisfier-picker.md (Accepted) specifies the user-visible
behavior: arrow-driven picker on a TTY, structured error under `-y` or
when piped, parallel to the existing `Multiple sources found` error.

This design picks the data structures, package layout, and integration
points that deliver the PRD's requirements with the smallest blast
radius across the existing code.

## Decision Drivers

- **Backward compatibility** — every recipe in the registry today must
  continue to validate and install identically. The new `aliases` key
  is purely additive.
- **Determinism for plans and dependencies** — plan generation, plan
  caching, and `runtime_dependencies` resolution must remain
  deterministic. Pickers must not engage during dependency resolution
  (PRD R10).
- **Established patterns** — match `internal/progress/` for ANSI/TTY
  handling so the next maintainer doesn't need to learn a new style.
- **Curated > discovery priority** — already enforced everywhere else
  in tsuku; the alias picker should suppress discovery hits when any
  satisfier matches (PRD R7).
- **Test surface** — the picker's interactive UX must be exercisable
  in CI without per-platform PTY harness flakiness (PRD R4a).
- **Binary size** — tsuku ships as a single binary; no multi-MB TUI
  dependency for a feature that needs ~200 lines of arrow-key
  handling.

## Considered Options

### Decision 1 — Index structure for the new alias mapping

**Question:** how does the loader represent multi-valued aliases alongside
the existing single-valued `satisfiesIndex`?

- **Option A: Separate `aliasIndex`** (chosen). Add a second field
  `aliasIndex map[string][]string` on the `Loader`, populated alongside
  `satisfiesIndex` from a new code path that reads only the `aliases`
  key. The existing `satisfiesIndex` stays exactly as is (1:1, ecosystem
  collation), so no existing call site changes behavior.
  - Pro: zero risk of regressing existing satisfies callers.
  - Pro: a single place owns the multi-valued semantics; the existing
    code stays simple.
  - Pro: `LookupAllSatisfiers(alias) []string` lives next to
    `LookupSatisfies(name) (string, bool)` with clear distinct
    contracts.
  - Con: two indexes to keep in sync (mitigated: both built lazily in
    one pass over the providers).

- **Option B: Make `satisfiesIndex` multi-valued** —
  `map[string][]satisfiesEntry`. Every existing 1:1 ecosystem entry
  becomes a length-1 slice. Picker engages whenever len > 1.
  - Pro: single index, no parallel structure.
  - Con: changes the type signature of `LookupSatisfies` and every
    caller. The existing semantic ("one entry, one recipe") is no
    longer true; readers have to think about why a length-1 slice
    isn't ambiguous.
  - Con: ecosystem-keyed entries (`homebrew`, `npm`) would
    accidentally become picker-eligible if two recipes both claim
    `homebrew = ["foo"]` — a behavior change with no opt-in.

- **Option C: Top-level `provides` field** (apt-style) — separate from
  satisfies entirely. Add `Provides []string` to `MetadataSection`.
  - Pro: cleanest semantic separation.
  - Con: doubles the schema surface (two ways to declare equivalence
    for one recipe). Recipe authors must understand the distinction.
  - Con: doesn't fit the current `Satisfies map[string][]string`
    structure; requires a new struct field, two new validator paths,
    two index-build code paths.

**Choice: Option A.** Smallest blast radius, clearest contracts, no
behavior change for existing recipes.

### Decision 2 — Picker package and TUI library

**Question:** where does the picker code live and what does it depend on?

- **Option A: New `internal/tui` package, hand-rolled ANSI on
  `golang.org/x/term`** (chosen). Mirrors `internal/progress` (which
  already does ANSI cursor manipulation for the spinner) and reuses
  an already-imported dependency.
  - Pro: zero new external dependencies, zero binary size growth.
  - Pro: ~200 lines of code, fully auditable.
  - Pro: matches the established style of `internal/progress`.
  - Con: more code to maintain than a library call.

- **Option B: `charmbracelet/bubbletea` or `huh`** —
  popular Go TUI frameworks.
  - Pro: less code in tsuku, more features available later.
  - Con: ~1.2–1.5 MB binary growth per the explore findings.
  - Con: scope is single-select; the framework is built for
    multi-page forms — overkill.
  - Con: introduces a transitive dependency tree we don't need.

- **Option C: `manifoldco/promptui`** — older single-select library.
  - Pro: smaller than bubbletea (~200 KB).
  - Con: unmaintained since 2020. Not a solid foundation for tsuku
    going forward.

**Choice: Option A.** Established pattern, no dependency cost,
appropriate scope for a single-select prompt.

### Decision 3 — Test surface for the interactive picker

**Question:** how does CI exercise picker behavior without a flaky PTY
harness?

- **Option A: Hidden `--pick <recipe-name>` flag** (chosen). Available
  on `tsuku install` only, only valid when the typed name resolves to a
  multi-satisfier alias, returns the same install plan as if the user
  had picked that recipe via the picker. Documented as test-only.
  - Pro: every picker behavior except literal key-input rendering is
    testable in CI without a PTY.
  - Pro: power users who discover the flag get a deterministic CLI
    override; no harm done.
  - Con: one more flag in `--help` output (mitigated: marked test-only,
    excluded from primary help text).

- **Option B: PTY harness for every picker test** —
  `creack/pty` to spawn a pseudo-terminal, scripted keystroke
  injection, ANSI byte-level assertions.
  - Pro: tests the actual interactive code path end-to-end.
  - Con: PTY behavior in CI containers and macOS runners is flaky;
    every test would race terminal initialization with input writes.
  - Con: every assertion is at the byte level; brittle to ANSI
    rendering tweaks.

- **Option C: `--pick` plus one PTY-gated integration test** (this
  design adopts a hybrid). The PTY test exists but is gated to the
  Linux integration job and tests one happy path plus one cancel
  path (PRD AC9, AC10). All other picker tests use `--pick`.

**Choice: Option C.** Get the broad coverage of Option A and the
end-to-end confidence of one carefully-scoped Option B test.

### Decision 4 — Validator vs index-build for R10 enforcement

**Question:** where does the rejection of multi-satisfier aliases in
`runtime_dependencies` live?

- **Option A: Both validator and registry index-build** (chosen).
  - `tsuku validate --strict` adds a check that walks `runtime_dependencies`,
    consults the loader's `aliasIndex`, and rejects if any dep maps to ≥2
    satisfiers. Recipe authors see this when running validate locally.
  - The registry index-build (CI ingestion of new recipes) runs the
    same check against the post-merge state. Catches the cross-recipe
    race where author A's recipe was valid yesterday and author D adds
    a new satisfier today.
  - Pro: defense in depth; the failure mode the PRD calls out (R10) is
    covered both at PR review time and at ingest time.
  - Pro: same code path for both, just different invocation contexts.
  - Con: two callers to thread the loader/aliasIndex through.

- **Option B: Validator only.** Recipe authors are expected to keep
  their dependencies up to date.
  - Pro: less code.
  - Con: leaves the cross-recipe race unhandled. A future recipe
    addition silently invalidates an existing recipe's dependency
    semantics; the only signal is at install time when a user runs
    into the picker for what they thought was a deterministic dep.

- **Option C: Index-build only.** Skip the validator path; trust CI.
  - Pro: less local validator complexity.
  - Con: terrible UX for recipe authors, who only learn about the
    problem after pushing.

**Choice: Option A.** The PRD's R10 explicitly names both; this is a
correctness gate, not a polish item.

### Decision 5 — Where in `cmd/tsuku/install.go` does the new branch slot in?

**Question:** what's the exact integration point?

- **Option A: Before `tryDiscoveryFallback`** (chosen). Lines 305–312
  in the install command currently call `loader.Get(toolName)`; on
  failure, they call `tryDiscoveryFallback`. The new code inserts a
  satisfier-count check between these two:

  ```go
  _, recipeErr := loader.Get(toolName, recipe.LoaderOptions{})
  if recipeErr != nil {
      satisfiers, _ := loader.LookupAllSatisfiers(toolName)
      switch len(satisfiers) {
      case 0:
          if result := tryDiscoveryFallback(toolName); result != nil {
              continue
          }
      case 1:
          // single-satisfier alias: rewrite toolName and fall through to install
          toolName = satisfiers[0]
      default:
          // multi-satisfier: picker (TTY) or error (-y/no-TTY)
          chosen, err := resolveMultiSatisfier(toolName, satisfiers)
          if err != nil {
              handleAmbiguousAliasError(toolName, satisfiers, err)
              continue
          }
          toolName = chosen
      }
  }
  // existing install proceeds with toolName
  ```

  - Pro: minimal change to the existing flow; the new branch is a single
    switch wedged between two existing calls.
  - Pro: the `case 0` branch is the literal existing code path
    unchanged.
  - Con: `tryDiscoveryFallback` and the new code both inspect the
    same recipe-not-found state; mild duplication of intent.

- **Option B: Inside `loader.Get`** — push the multi-satisfier
  check down into the loader, returning a special error
  (`ErrAmbiguousAlias`) that the install command unwraps.
  - Pro: keeps the install command simpler.
  - Con: the loader is shared across commands (`tsuku run`,
    `tsuku info`, etc.) that have different ambiguity policies.
    Pushing the check down would force every caller to handle the
    new error or accidentally inherit the picker behavior. Hostile
    to the PRD's "install only" scope.

**Choice: Option A.** Keeps the picker scope inside the install
command where it belongs.

## Decision Outcome

The chosen design:

1. **Schema** (`internal/recipe/types.go`, no struct change): the
   existing `Satisfies map[string][]string` already accepts arbitrary
   keys. Recipes opt in by adding `aliases = ["java"]`.
2. **Validator** (`internal/recipe/validator.go`): two changes.
   - In `validateSatisfies`, skip the ecosystem-name validation for
     the `aliases` key (any string list accepted).
   - New function `validateRuntimeDepsNotMultiSatisfier(r *Recipe,
     loader *Loader)` walks `runtime_dependencies`, consults the
     loader's alias index, and reports an error per dep that has ≥2
     satisfiers. Called from `validateRecipe` during `tsuku validate
     --strict` and from the registry index-build path.
3. **Index** (`internal/recipe/loader.go`): add
   `aliasIndex map[string][]string` parallel to the existing
   `satisfiesIndex`, populated by extending `buildSatisfiesIndex`
   with a second loop over the `aliases` key. Add public methods
   `LookupAllSatisfiers(alias string) ([]string, bool)` and
   `HasMultiSatisfier(alias string) bool`.
4. **Picker** (`internal/tui/picker.go`, new package): single-select
   arrow-driven prompt built on `golang.org/x/term`. API:
   ```go
   type Choice struct{ Name, Description string }
   func Pick(prompt string, choices []Choice) (int, error)
   var ErrCancelled = errors.New("picker: cancelled")
   ```
5. **Install command** (`cmd/tsuku/install.go`): the switch from
   Decision 5; new functions `resolveMultiSatisfier` (consults TTY +
   `-y` + `--pick` and either calls picker or errors) and
   `handleAmbiguousAliasError` (parallel to
   `handleAmbiguousInstallError`).
6. **Test-only flag**: `--pick <recipe-name>` on `tsuku install`,
   marked hidden, validated to only apply when the resolved name is a
   multi-satisfier alias.
7. **Plan emit reuse**: `tsuku eval --json --recipe <name>` already
   serializes an `InstallationPlan`. No new emit command needed; AC12
   uses the existing one.
8. **Registry index-build hook** (`internal/registry/cache_manager.go`):
   the existing recipe-ingestion path adds a call to
   `validateRuntimeDepsNotMultiSatisfier` (or a thin wrapper) for every
   recipe being ingested.
9. **Recipe declarations** (`recipes/o/openjdk.toml`,
   `recipes/t/temurin.toml`, `recipes/c/corretto.toml`,
   `recipes/m/microsoft-openjdk.toml`): each gets `aliases = ["java"]`
   under `[metadata.satisfies]`.

## Solution Architecture

### Components

```
                           tsuku install <name>
                                   |
                                   v
                          loader.Get(<name>)
                                   |
                       +---------- + ----------+
                       |                       |
                  recipe found             not found
                       |                       |
                       v                       v
                proceed install        loader.LookupAllSatisfiers
                                                |
                                +---------------+---------------+
                                |               |               |
                              0 hits         1 hit          2+ hits
                                |               |               |
                                v               v               v
                       tryDiscoveryFallback  rewrite       resolveMultiSatisfier
                                |             toolName            |
                                v               |          +------+------+
                        existing flow           v          |             |
                                          proceed install  TTY        -y/no-TTY
                                                              |             |
                                                              v             v
                                                      tui.Pick()   handleAmbiguousAliasError
                                                              |             |
                                                              v             v
                                                       proceed install   exit 10
```

### Data structures

```go
// internal/recipe/loader.go (additions)
type Loader struct {
    // ...existing fields...
    satisfiesIndex map[string]satisfiesEntry   // existing, 1:1, ecosystem-keyed
    aliasIndex     map[string][]string         // NEW: alias → []recipeName
    aliasOnce      sync.Once
}

func (l *Loader) LookupAllSatisfiers(alias string) ([]string, bool)
func (l *Loader) HasMultiSatisfier(alias string) bool
```

```go
// internal/tui/picker.go (new package)
type Choice struct {
    Name        string
    Description string
}

var ErrCancelled = errors.New("picker: cancelled")

// Pick renders an arrow-driven single-select prompt to stderr and returns
// the index of the chosen entry. Cancellation (Ctrl-C) returns
// ErrCancelled. Caller is responsible for TTY-readiness.
func Pick(prompt string, choices []Choice) (int, error)

// IsAvailable reports whether stderr is a TTY so a picker can render.
// Wraps term.IsTerminal for one-stop calls from the install command.
func IsAvailable() bool
```

```go
// cmd/tsuku/install.go (additions)
var installPick string  // bound to --pick flag, hidden

// resolveMultiSatisfier picks a recipe from the candidate list per the
// PRD's case-C truth table:
//   - if --pick is set, return the named choice (validate it's in the list)
//   - else if -y or stderr is not a TTY, return ErrAmbiguous (caller emits
//     the parallel "Multiple recipes satisfy" error)
//   - else call tui.Pick and return the chosen recipe name
func resolveMultiSatisfier(alias string, candidates []string) (string, error)

// handleAmbiguousAliasError prints the "Multiple recipes satisfy" error
// (text + JSON) and exits with ExitAmbiguous. Parallel to the existing
// handleAmbiguousInstallError for discovery-layer ambiguity.
func handleAmbiguousAliasError(alias string, candidates []string, err error)
```

### Resolution flow detail

The switch in `cmd/tsuku/install.go` lines 305–312 changes from the current
two-branch (recipe found / try discovery) to a four-branch (recipe found /
1 satisfier / multi-satisfier / discovery). The current line 309
`tryDiscoveryFallback` call moves into the `case 0` arm of the switch
unchanged. The other two arms are new code.

### Picker implementation outline

`internal/tui/picker.go`:

```go
func Pick(prompt string, choices []Choice) (int, error) {
    // 1. Stash current terminal state, set raw mode
    fd := int(os.Stderr.Fd())
    oldState, err := term.MakeRaw(fd)
    if err != nil {
        return 0, err
    }
    defer term.Restore(fd, oldState)

    // 2. Hide cursor; restore on exit
    fmt.Fprint(os.Stderr, "\x1b[?25l")
    defer fmt.Fprint(os.Stderr, "\x1b[?25h")

    // 3. Print prompt + initial render
    cursor := 0
    render(prompt, choices, cursor)

    // 4. Key loop
    buf := make([]byte, 3)
    for {
        n, err := os.Stdin.Read(buf)
        if err != nil { return 0, err }
        switch {
        case isUpArrow(buf, n) && cursor > 0:
            cursor--
        case isDownArrow(buf, n) && cursor < len(choices)-1:
            cursor++
        case isEnter(buf, n):
            clear(prompt, choices)
            return cursor, nil
        case isCtrlC(buf, n):
            clear(prompt, choices)
            return 0, ErrCancelled
        }
        rerender(prompt, choices, cursor)
    }
}
```

ANSI sequences used: cursor up `\x1b[A`, cursor down `\x1b[B`, cursor
visible/hidden `\x1b[?25h/l`, clear line `\x1b[2K`, move to column 0
`\r`. All standard, no platform-specific escape codes.

### `--pick <recipe>` semantics

`cmd/tsuku/install.go` adds:

```go
installCmd.Flags().StringVar(&installPick, "pick", "",
    "Test-only: pre-resolve a multi-satisfier alias to the named recipe "+
    "without rendering the picker.")
_ = installCmd.Flags().MarkHidden("pick")
```

`resolveMultiSatisfier` short-circuits when `installPick != ""`:

```go
if installPick != "" {
    if !slices.Contains(candidates, installPick) {
        return "", fmt.Errorf("--pick %q: not a satisfier of alias %q (candidates: %v)",
            installPick, alias, candidates)
    }
    return installPick, nil
}
```

The flag rejects pre-resolved direct-name matches and single-satisfier
aliases by virtue of never being reached in those cases (the switch's
`case 1:` and direct-find branches don't call `resolveMultiSatisfier`).

### Validator + index-build wiring

`internal/recipe/validator.go` gains:

```go
// validateRuntimeDepsNotMultiSatisfier rejects recipes whose
// runtime_dependencies list contains an alias that maps to two or more
// satisfying recipes. Multi-satisfier aliases must not appear in dep
// chains because plan generation would become non-deterministic.
//
// Called from validateRecipe (tsuku validate --strict path) and from
// the registry index-build path (CI ingestion).
func validateRuntimeDepsNotMultiSatisfier(
    r *Recipe,
    loader *Loader,
    result *ValidationResult,
)
```

`internal/registry/cache_manager.go` (or whichever entry point ingests new
recipes) calls the same function during ingestion. Both invocations
accept the same `*Loader` so the alias index they consult is the same
post-merge view of the registry.

## Implementation Approach

Phased so each phase has a meaningful CI green and can stand alone if
later phases are deferred.

### Phase 1 — Schema + index (no behavior change)

- Add `aliasIndex` field and `aliasOnce` sync.Once to `Loader`.
- Extend `buildSatisfiesIndex` to populate `aliasIndex` from the
  `aliases` key (keyed by alias, value is `[]string` of recipe names,
  multi-recipe per key).
- Add public methods `LookupAllSatisfiers` and `HasMultiSatisfier`.
- Validator accepts `aliases` as a non-ecosystem key in
  `validateSatisfies`.
- Unit tests for the loader index + validator acceptance.

No CLI behavior change after this phase. Recipes can declare
`aliases = ["java"]` and the validator passes; nothing consumes the
alias index yet.

### Phase 2 — Picker package

- New `internal/tui/picker.go` with `Pick`, `IsAvailable`, `Choice`,
  `ErrCancelled`.
- Unit tests with mocked stdin/stderr (the picker accepts
  `io.Reader`/`io.Writer` injectable via package-internal seams; the
  exported `Pick` uses os.Stdin/os.Stderr).

No CLI behavior change after this phase either. The picker exists as
library code.

### Phase 3 — Install command integration

- New `--pick` flag (hidden).
- New `resolveMultiSatisfier` and `handleAmbiguousAliasError` in
  `cmd/tsuku/install.go`.
- Switch the resolution flow (lines 305–312) to the four-arm switch.
- Tests:
  - AC1 (direct-name match unchanged): table-driven against existing
    recipes.
  - AC3 (single-satisfier auto-resolve): synthetic recipe with one
    `aliases` entry.
  - AC4–7, AC8 (multi-satisfier paths via `--pick` and `-y`):
    synthetic recipes with multiple satisfiers.
  - AC9, AC10 (PTY-gated): single integration test in
    `test/functional/` exercising real arrow-key + Enter + Ctrl-C.

### Phase 4 — Validator R10 enforcement

- `validateRuntimeDepsNotMultiSatisfier` in `internal/recipe/validator.go`.
- Wire into `validateRecipe` so `tsuku validate --strict` calls it.
- AC14 test: synthetic recipe with multi-satisfier alias as a runtime
  dep.

### Phase 5 — Registry index-build hook

- Add a call to `validateRuntimeDepsNotMultiSatisfier` (or a thin
  wrapper that loops over all ingested recipes) in
  `internal/registry/cache_manager.go` (or the actual ingestion entry
  point).
- AC15 test: registry-fixture path that fails on the same scenario as
  AC14.

### Phase 6 — Recipe declarations

- Add `aliases = ["java"]` to all four JDK recipes.
- AC11 (manual / sandbox-test) confirms `tsuku install java -y`
  errors with the four-vendor list.
- The existing PR #2362 sandbox tests cover AC16, AC17 implicitly.

## Security Considerations

The picker introduces a new interactive code surface. Specific concerns:

- **Raw terminal mode with untrusted input.** `term.MakeRaw` puts the
  terminal in raw mode. The picker reads up to 3 bytes per loop
  iteration to detect arrow keys (which are 3-byte sequences:
  `\x1b[A`, `\x1b[B`). It does not interpret bytes as anything other
  than: arrow up/down, Enter, Ctrl-C. Other input is silently
  ignored. There is no command execution path from picker input —
  only an integer index is returned to the caller.
- **ANSI escape injection.** Recipe descriptions are rendered into
  the picker's row text. A malicious recipe description containing
  ANSI escape sequences could attempt to overwrite portions of the
  terminal display. **Mitigation**: the picker passes recipe
  descriptions through a sanitization helper (similar to
  `internal/progress/sanitize.go`) that strips bytes < 0x20 except
  for tab and newline. New file: `internal/tui/sanitize.go` with the
  same logic as the progress sanitizer.
- **Terminal state restoration on signals.** If the process receives
  SIGINT during picker rendering, the deferred `term.Restore` call
  runs as part of normal Go panic/return semantics. The picker's
  Ctrl-C handler is the cooperative path; an external SIGINT
  (e.g., from a parent shell sending the signal directly) bypasses
  the cooperative loop. **Mitigation**: the deferred `Restore` and
  cursor-show writes run during goroutine unwind; if a panic occurs,
  Go's runtime still runs deferred functions. The risk is a `kill -9`
  (SIGKILL) — unrecoverable, same as any other interactive command.
- **Hidden `--pick` flag.** The flag is documented as test-only but
  reachable by users. It cannot bypass any security check (no
  privilege escalation, no skip of verification). It only short-
  circuits the picker rendering. No security risk.
- **Validator R10 reduces install-time surprise.** A recipe whose
  dependency would otherwise resolve to one of multiple satisfiers
  is rejected at validate/index-build time. Removes the class of
  bug where a user installs a recipe and unknowingly pulls in a
  different vendor's downstream tool than they expected.
- **No new attack surface in plan caching, state.json, or registry
  ingestion.** The picker decision flows into the resolved recipe
  name, which already feeds into deterministic, signed-state code
  paths.

## Consequences

### Positive

- The four-recipe OpenJDK family becomes usable as the user expects
  (`tsuku install java` lists all four).
- Future virtual aliases (`python`, `node`, `gcc`, ...) gain a
  declarative path with no further CLI work.
- The picker UX is consistent with the established `internal/progress`
  spinner — same rendering style, same ANSI vocabulary.
- Validator + index-build dual-checks catch the cross-recipe race
  (author A's recipe was valid yesterday; author D adds a new
  satisfier today) before any user is affected.

### Negative

- One more package (`internal/tui`) to maintain.
- The `--pick` flag adds a CLI surface that's testing-oriented, not
  user-facing. Documentation work to ensure users don't reach for
  it instead of `--from`.
- Recipe authors must understand a new piece of schema (`aliases`
  vs ecosystem-keyed satisfies). Mitigated by the curated-recipe
  authoring skill being updated when this lands.
- The PTY-gated integration test (AC9, AC10) is the single test in
  the suite that requires a pseudo-terminal. If it's flaky in CI,
  it'll need attention.

### Mitigations

- The `internal/tui` package is small (~250 lines target) and
  self-contained. Failure modes are bounded.
- `--pick` documentation lives in code comments on the flag
  declaration plus the design doc; the recipe-authoring skill should
  prefer `--from` when documenting alias-resolution.
- The schema change ships with examples in the four JDK recipes
  themselves, so the recipe-authoring skill can point at real
  precedent.
- The PTY test runs only in the Linux integration job (one runner)
  and tests only two scenarios (happy path + Ctrl-C). If it flakes
  three times in a row, mark it skipped and file a bug — `--pick`
  coverage means the user-facing behavior is still verified.

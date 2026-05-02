---
status: In Progress
problem: |
  Users typing `tsuku install <name>` cannot pick among multiple recipes that
  satisfy the same alias. The OpenJDK family in PR #2362 ships four recipes
  that are all valid answers to "give me Java," but the satisfies index is
  one-recipe-per-alias at the type level, so `tsuku install java` either
  silently picks one default or errors with unrelated discovery hits.
goals: |
  Let `tsuku install <name>` present an arrow-driven picker on a TTY when the
  name resolves to multiple recipes via the alias index, fail with a clear
  error listing the candidates under `-y`/non-TTY, and auto-resolve
  transparently when the name resolves directly or via a single-satisfier
  alias. The same mechanism unblocks future virtual aliases (`python`,
  `node`, `gcc`, ...) as the curated registry grows.
source_issue: 2368
---

# PRD: Multi-Satisfier Alias Picker

## Status

In Progress

## Vocabulary

These terms are used in exactly these senses throughout the document.

- **Recipe** — a TOML file under `recipes/` defining one installable unit.
- **Recipe name** — the value of `metadata.name` in a recipe; also the
  filename stem (`recipes/o/openjdk.toml` ⇒ name `openjdk`).
- **Direct-name match** — the user-typed `<name>` matches a recipe's filename
  stem exactly. Resolution stops here; lower-precedence layers are not
  consulted.
- **Alias** — a string under `[metadata.satisfies] aliases = [...]` declared
  by zero or more recipes. The same alias may be declared by multiple
  recipes (the new behavior this PRD introduces).
- **Satisfier** — a recipe that declares an alias under `aliases`. *Used
  only in this sense.* A direct-name match is **not** a "satisfier" — it's
  a separate resolution layer.
- **Single-satisfier alias** — an alias declared by exactly one recipe.
- **Multi-satisfier alias** — an alias declared by two or more recipes.
- **Discovery hit** — a candidate recipe found by the discovery layer
  (npm/PyPI/RubyGems/Homebrew/crates.io probes), distinct from a
  direct-name match or a satisfier.
- **Picker** — the interactive arrow-key UI rendered on a TTY when
  `tsuku install <name>` resolves the name to a multi-satisfier alias.

## Problem Statement

`tsuku install <name>` resolves the name through a chain of providers (local
recipe file, registry, embedded), then falls back to a satisfies index that
maps a package name to a single recipe. When that fails, the discovery layer
probes package registries (npm, PyPI, RubyGems, Homebrew) and, if more than
one discovery hit comes back, surfaces a `Multiple sources found` error that
suggests `--from <ecosystem>:<package>` flags.

The schema and resolution model assume that any given name resolves to at
most one curated recipe. PR #2362 ships four curated recipes for the
OpenJDK family (`openjdk`, `temurin`, `corretto`, `microsoft-openjdk`) — all
four are valid answers to "give me a Java runtime" but the satisfies index
can only register one of them against the alias `java`. The user typing
`tsuku install java` today gets one of:

- A confusing error pointing at unrelated discovery hits (a Ruby gem named
  `java`, an npm package named `java`) — the current state.
- A silent default to whichever recipe wins the satisfies-index race — the
  state if a single recipe declared `satisfies.homebrew = ["java"]`.

Neither matches the multi-vendor choice the milestone is built around. The
same shape will recur for every "virtual" alias as the registry grows:
`python` (cpython vs python-standalone vs pyenv), `node` (system nodejs vs
nvm-managed), `gcc` (gcc vs gcc@N), and so on. A one-recipe-per-alias model
forces every choice into a default baked into a single recipe file, which
doesn't match how users mentally model the choice.

## Goals

The user types `tsuku install <name>` and the right thing happens:

1. **Direct-name match or single-satisfier alias** — install proceeds
   without prompting. No surprise behavior change for any name that's
   already unambiguous.
2. **Multi-satisfier alias on an interactive TTY** — an arrow-key driven
   picker shows the candidates with their descriptions; the user picks one
   with Enter; install proceeds. Number-typing, fzf-style search, and
   other UX modes are explicitly out of scope.
3. **Multi-satisfier alias under `-y` or no TTY** — install fails with an
   error listing every candidate and the explicit `--from <recipe-name>`
   flag invocation that would select each one. Mirrors the existing
   `Multiple sources found` error format so scripts and dashboards already
   wired for that pathway continue to work without changes.
4. **All four PR #2362 OpenJDK recipes** declare they satisfy `java` and
   `tsuku install java` lists all four.

## User Stories

These are CLI workflows, not end-user product flows. Use-case format is used
where a "user story" framing would feel forced.

- **As a developer with a Java project**, I want `tsuku install java` to
  show me which JDK distributions are available so I can pick the one that
  matches my team's existing tooling, instead of being surprised by an
  arbitrary default or an unrelated package.
- **As a CI script author**, I want `tsuku install java -y` to fail loudly
  with the candidate list when the alias is ambiguous, so my pipeline doesn't
  silently install a different vendor's JDK between runs.
- **As a recipe author**, I want to declare that my recipe satisfies a
  shared alias (e.g., `java`, `python`) without taking sole ownership of
  that alias, so the user gets to choose between my recipe and other valid
  options.
- **Use case: dependency resolution in a recipe** — when one recipe
  declares `runtime_dependencies = ["java"]`, plan generation MUST resolve
  the name deterministically to a single recipe. Pickers cascading through
  dep resolution would be hostile (interactive prompts in the middle of
  an install). The validator and the registry index-build step (R10)
  reject recipes whose `runtime_dependencies` reference a multi-satisfier
  alias.

## Requirements

### Functional

- **R1.** A new `aliases` key under `[metadata.satisfies]` accepts a list of
  alias strings. Multiple recipes may list the same alias.
- **R2.** The recipe satisfies index supports one-to-many lookup for the
  `aliases` namespace via `LookupAllSatisfiers(alias) []string`. The
  existing ecosystem-keyed entries (`homebrew`, `npm`, `pypi`, etc.) keep
  their one-to-one semantics — no behavior change for any recipe that
  doesn't use the new `aliases` key.
- **R3.** When `tsuku install <name>` runs, resolution order is:
  1. **Direct-name match** — if `recipes/{first-letter}/{name}.toml`
     exists, resolve to that recipe. Stop. Lower layers are not consulted,
     including the alias index and the discovery layer.
  2. **Single-satisfier alias lookup** — if exactly one recipe declares
     `<name>` under `aliases`, resolve to that recipe transparently
     (no prompt, no error).
  3. **Multi-satisfier alias lookup** — if two or more recipes declare
     `<name>` under `aliases`, branch by mode: TTY without `-y` →
     picker (R4); `-y` or non-TTY → error (R5).
  4. **Discovery fallback** — only reached when zero recipes match in
     R3.1, R3.2, or R3.3. Behavior unchanged from today.
- **R4.** Multi-satisfier resolution on a TTY (without `-y`) presents an
  arrow-driven picker:
  - One candidate per row, displayed as `<recipe-name> — <description>`,
    sorted alphabetically by recipe name.
  - Up/Down arrow keys move the cursor; Enter confirms; Ctrl-C cancels.
  - **On Ctrl-C cancel**: stderr emits a single line `Cancelled.`,
    exit code is 130 (the conventional SIGINT exit code = 128 + 2). No
    state is mutated, no install proceeds.
  - Picker output goes to stderr; stdout is reserved for install output
    that follows confirmation.
  - The picker is single-select only — no multi-select, no fuzzy search.
- **R4a.** **Test surface for the picker.** A hidden flag
  `--pick <recipe-name>` resolves a multi-satisfier alias to the named
  recipe without rendering the picker. The flag is rejected when the name
  doesn't resolve to a multi-satisfier alias (so it can't be used to
  override direct-name or single-satisfier matches), and it is documented
  as an internal/test surface only (not advertised in `tsuku install --help`
  beyond a "test-only" note). This makes R4 testable in CI without a PTY
  harness.
- **R5.** Multi-satisfier resolution under `-y` or on a non-TTY fails with
  a structured error to stderr, exit code 10 (`ExitAmbiguous`, the same
  code the existing `Multiple sources found` error uses), in this format:
  ```
  Multiple recipes satisfy alias "<alias>". Use --from to specify a recipe:
    tsuku install <alias> --from <recipe-name>
    tsuku install <alias> --from <recipe-name>
    ...
  ```
  Candidates are listed alphabetically by recipe name. When `--json` is
  set, the same information is emitted as a structured payload mirroring
  the existing `Multiple sources found` JSON shape (fields:
  `error`, `alias`, `candidates: [{recipe, from}]`, `exit_code`).
- **R6.** The `--from` flag accepts a recipe-name form (no colon) for
  alias selection: `tsuku install java --from temurin`. The existing
  `<ecosystem>:<package>` form is unchanged. The parser distinguishes by
  colon presence.
- **R7.** When R3 resolves the name via the alias index (R3.2 single or
  R3.3 multi), the discovery layer (R3.4) is **not** consulted. Discovery
  hits remain reachable only via the explicit
  `--from <ecosystem>:<package>` flag. R3.1 (direct-name match) likewise
  bypasses R3.4 — the existing behavior. (This requirement merely names
  the implication of R3's ordering; no separate code path.)
- **R8.** Plan caching is unaffected: the picker's choice is baked into
  the resolved `InstallationPlan.Tool` field, plan hashes stay stable,
  and `tsuku install --plan plan.json` replays without re-prompting.
  The plan file is produced by the existing `tsuku eval --json
  --recipe <name>` command (which already serializes an
  `InstallationPlan`); no new emit command is introduced.
- **R9.** State.json records the resolved recipe name (existing
  behavior). `tsuku update <name>` reads from state and refreshes the
  same recipe — no re-pick needed even when `<name>` is a multi-satisfier
  alias on first install.
- **R10.** A recipe whose `runtime_dependencies` list contains a
  multi-satisfier alias is **invalid**. This is enforced at two points:
  - **At `tsuku validate --strict` time** — the validator queries the
    registry's alias index; if the dependency name has two or more
    satisfiers, validate fails with an error naming the alias and
    every satisfier. The recipe author sees this when running validate
    locally before opening a PR.
  - **At registry index-build time** — when CI ingests new recipes into
    the curated registry, the same check runs against the post-merge
    state. A new recipe that adds itself as a satisfier of an alias
    that another already-merged recipe depends on causes the index
    build to fail with an error pointing at both recipes. This catches
    the "author A's recipe was valid yesterday; author D added a new
    satisfier today" race.
  - The error in both cases lists the dependent recipe, the alias, and
    every satisfying recipe so authors can either rename the dep to a
    specific recipe (`temurin` instead of `java`) or coordinate with
    other satisfiers.
- **R11.** **Direct-name + multi-satisfier collision** (truth-table
  case E). If a recipe with filename matching `<name>` exists *and*
  multiple other recipes declare `<name>` as an alias, R3.1 wins
  unconditionally: install resolves to the direct-name recipe with no
  picker, no error, no consultation of the alias index. Recipe authors
  retain control of their own canonical names.
- **R12.** All four PR #2362 OpenJDK recipes (`openjdk`, `temurin`,
  `corretto`, `microsoft-openjdk`) declare `aliases = ["java"]` and
  `tsuku install java` lists all four in the picker / error.

### Non-functional

- **NF1.** No new external dependencies. Picker is built on
  `golang.org/x/term` (already imported) plus hand-written ANSI escape
  sequences for cursor movement, mirroring the existing
  `internal/progress/` spinner pattern.
- **NF2.** Picker code budget: under 250 lines (target 200) including
  TTY detection, key-input loop, ANSI rendering, and structured output.
- **NF3.** Cross-platform: works on Linux (glibc + musl) and macOS in
  the same shells tsuku already supports. No Windows requirement.
- **NF4.** Backward compatibility: every existing recipe in the registry
  continues to validate and install identically. Only recipes that opt
  in to the new `aliases` key get multi-satisfier behavior.
- **NF5.** Error UX consistency: `Multiple recipes satisfy` uses the
  same exit code (10) and `--json` shape as the existing `Multiple
  sources found` error.

## Acceptance Criteria

Each AC corresponds to a deterministic test that can run in CI at PR-merge
time. ACs are grouped by truth-table case for traceability.

**Case A — direct-name match (R3.1, R11):**
- [ ] **AC1.** `tsuku install openjdk` on a fresh TSUKU_HOME exits 0,
  installs the openjdk recipe, and emits no picker or
  `Multiple recipes satisfy` output to stderr.
- [ ] **AC2.** A test fixture creates a synthetic recipe at
  `recipes/j/java.toml` (name `java`) plus two other recipes declaring
  `aliases = ["java"]`. `tsuku install java` resolves to the
  `recipes/j/java.toml` recipe with no picker and no error. (R11)

**Case B — single-satisfier alias (R3.2):**
- [ ] **AC3.** A test fixture has exactly one recipe declaring
  `aliases = ["solo-alias"]`. `tsuku install solo-alias` exits 0,
  installs the satisfying recipe, and emits no picker or error.

**Case C — multi-satisfier alias (R3.3, R4, R4a, R5, R6):**
- [ ] **AC4.** With the four JDK recipes declaring `aliases = ["java"]`,
  `tsuku install java -y` exits 10, writes the
  `Multiple recipes satisfy alias "java"` error to stderr, lists all
  four recipes alphabetically, and prints a `--from <recipe-name>` line
  for each. (R5)
- [ ] **AC5.** Same setup, `tsuku install java` with stdout piped (no
  TTY) produces identical output to AC4 — exit 10, no half-rendered
  picker on stderr (no ANSI cursor escape sequences). (R5)
- [ ] **AC6.** Same setup, `tsuku install java --json -y` emits a
  JSON payload to stdout containing `error`, `alias = "java"`,
  `candidates = [{recipe, from}, ...]` (4 entries), and `exit_code = 10`.
  Process exit is 10. (R5)
- [ ] **AC7.** Same setup, `tsuku install java -y --from temurin`
  exits 0, installs temurin, and produces no picker or error output.
  (R6)
- [ ] **AC8.** Same setup, `tsuku install java --pick temurin`
  (the test-only flag from R4a) exits 0, installs temurin, and
  produces no picker render. (R4a)
- [ ] **AC9.** With AC8's setup, an integration test using a PTY harness
  confirms that without `--pick` and without `-y`, the picker renders
  to stderr with the four JDK recipes, responds to up/down arrow input,
  and confirms on Enter. The harness lives in `test/functional/` and
  is gated to Linux only (since macOS PTY behavior in CI is flaky).
  This is the single PTY-bound test; all other interactive-mode
  behavior is verified via `--pick`. (R4)
- [ ] **AC10.** Same setup, sending Ctrl-C (SIGINT) to the picker
  produces stderr line `Cancelled.`, exits 130, and TSUKU_HOME shows no
  install attempt (no entries in `tsuku list`, no state.json mutation).
  Verified via the same PTY harness as AC9. (R4)

**Case D — no match (R3.4):**
- [ ] **AC11.** `tsuku install some-unknown-name-xyz123` falls through
  to the discovery layer with current behavior — verified by snapshot
  of stderr against the pre-PR baseline. No regression.

**Case E — collision precedence (R11):**
- Covered by AC2 above.

**Plan + state behavior (R8, R9):**
- [ ] **AC12.** `tsuku eval --recipe recipes/t/temurin.toml --json` emits
  a plan with `Tool = "temurin"`. Saving that JSON to `plan.json` and
  running `tsuku install --plan plan.json` installs temurin without
  prompting (verifying R8's "no re-prompt on plan replay" claim, with
  the test setup using the documented eval emit path).
- [ ] **AC13.** With state.json containing `temurin` as the resolved
  recipe for tool `java` (pre-seeded in the test, no interactive run
  required), `tsuku update java` updates the temurin recipe and emits
  no picker or error. (R9)

**Validator (R10):**
- [ ] **AC14.** A test fixture defines two recipes both declaring
  `aliases = ["dep-alias"]`, and a third recipe declaring
  `runtime_dependencies = ["dep-alias"]`. Running `tsuku validate
  --strict` on the third recipe exits non-zero with an error message
  that contains the strings `dep-alias` and the names of both
  satisfying recipes. (R10 validator path)
- [ ] **AC15.** A registry-index-build test (driver TBD by design;
  `internal/registry/` package has the relevant entry point) runs over
  a fixture registry containing the same three recipes from AC14 and
  fails with the same error class. (R10 index-build path)

**Backward compatibility (NF4):**
- [ ] **AC16.** `tsuku validate --strict` over every recipe in
  `recipes/**.toml` exits 0 (this is what the existing
  `Validate Recipes` CI workflow already does — confirm the workflow
  passes on the implementing PR; no new test is needed).
- [ ] **AC17.** `Linux Sandbox Tests`, `Linux Integration Tests`, and
  `macOS x86_64`/`macOS arm64` workflows in `.github/workflows/` all
  pass on the implementing PR. (These cover install + verify across
  every supported family at PR-merge time.)

## Out of Scope

- Picker UX modes other than arrow-driven single-select: no number-typing,
  no fuzzy search, no multi-select, no `gum`-style multi-page navigation.
- Picker engagement during dependency resolution. Dependencies that
  resolve to a multi-satisfier alias are an authoring error
  (R10 enforces this at validate time and at registry-index-build time).
- Other commands gaining picker behavior. `tsuku install` only;
  `tsuku run`, `tsuku info`, `tsuku versions`, etc. retain their
  existing first-match-or-error semantics.
- A separate `provides`/virtual-package concept independent of satisfies.
  This PRD extends the existing satisfies struct rather than introducing
  a parallel mechanism.
- Pre-population of the picker with a "default" suggestion. The picker
  starts on the first row in alphabetical order; the user always picks
  explicitly.
- Migration of existing 1:1 ecosystem-keyed entries (`homebrew`, `npm`,
  etc.) into the new multi-satisfier model. Those keep their existing
  semantics unchanged.
- Windows support for the picker (and tsuku in general).

## Decisions and Trade-offs

These decisions are settled from the exploration phase
(`wip/explore_2368-multi-satisfier-picker_decisions.md`) and should not
be re-litigated during design or implementation.

- **TUI: `golang.org/x/term` + hand-rolled ANSI** rather than
  bubbletea/huh/promptui/survey. Picker scope is narrow (single-select),
  the dependency is already imported, and it matches the established
  `internal/progress/` pattern. No new external dependency, no binary
  size growth.
- **Schema: extend `[metadata.satisfies]` with a non-ecosystem
  `aliases` key**, not a new top-level `provides` field. The existing
  `Satisfies map[string][]string` already accepts arbitrary keys; only
  the validator and index builder change. Smallest blast radius.
- **Direct-name match wins over the alias index** (R11). Recipe
  authors retain control of their own canonical names; matches the
  established `TestSatisfies_GetWithContext_ExactMatchTakesPriority`
  precedent.
- **Subsume discovery hits when an alias resolves** (R7). The curated
  > discovery priority everywhere else in tsuku argues against mixing
  the two lists. Discovery hits remain reachable via explicit
  `--from <ecosystem>:<package>`.
- **`-y` and "TTY available" are independent gates.** `-y` means "force
  approval"; non-TTY means "no human to ask." Either triggers the
  non-interactive error path; both being set is the same as either
  alone.
- **Reuse exit code 10 / `ExitAmbiguous` and the `--json` shape** of
  the existing `Multiple sources found` error. Scripts already wired
  for that exit code keep working.
- **Validator and registry-index-build both reject multi-satisfier
  aliases in `runtime_dependencies`** (R10). Plan generation must be
  deterministic; pickers in dep resolution would be hostile UX.
  Validating at both points catches the "satisfier added later"
  cross-recipe race.
- **Test-only `--pick <recipe>` flag** (R4a) makes the picker
  testable in CI without per-platform PTY harness flakiness, except
  for one PTY-gated integration test that exercises the actual key
  input loop (AC9, AC10).
- **Plan emit reuses `tsuku eval --json`** (R8) rather than introducing
  a new `--write-plan` flag. The eval command already serializes the
  plan; the install path can consume the same JSON.

## Known Limitations

- The picker runs only when the install command is the user's top-level
  invocation. Recipes pinned via `runtime_dependencies` to a
  multi-satisfier alias fail at validate time and at registry-index-build
  time (R10). Authors who want flexibility for downstream consumers
  should depend on a specific recipe name, not an alias.
- The picker shows the alphabetical order of recipe names; there's no
  notion of "recommended" or "default" highlighting. Future work could
  add a recipe-side `recommended-for-alias = "java"` flag, but this
  PRD intentionally avoids that complexity.
- ANSI rendering assumes a reasonably modern terminal. Very old
  terminals (no cursor movement support) will see a degraded display
  but the picker still functions because key-by-key reads work.
- The `--pick <recipe>` flag is documented as test-only and not
  advertised in `tsuku install --help` beyond a brief note. Power users
  who discover it can use it as a deterministic CLI override, but the
  blessed user-facing flag for picking a recipe is `--from <recipe>`.

## Downstream Artifacts

- **Design**: [`docs/designs/current/DESIGN-multi-satisfier-picker.md`](../designs/current/DESIGN-multi-satisfier-picker.md) (Current).
- **Picker mechanism implementation**: PR [#2369](https://github.com/tsukumogami/tsuku/pull/2369) closes [#2368](https://github.com/tsukumogami/tsuku/issues/2368) — ships R1–R11 (the schema, alias index, picker package, install integration, validator, and registry index-build hook).
- **R12 fulfillment (JDK recipes declare `aliases = ["java"]`)**: deferred to PR [#2362](https://github.com/tsukumogami/tsuku/pull/2362) which already authors the four OpenJDK family recipes; the alias declarations land alongside that PR.
- **Broader alias declarations on existing recipes**: tracked under [#2370](https://github.com/tsukumogami/tsuku/issues/2370) for the wider ecosystem (`ripgrep→rg`, `awscli→aws`, `golang→go`, etc.) — gated on a tsuku release containing the picker mechanism.

The PRD remains **In Progress** until R12 lands via PR #2362; status transitions to Done once `tsuku install java` actually presents the four-recipe picker on a tsuku release.

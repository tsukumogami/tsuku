# Explore Scope: 2368-multi-satisfier-picker

## Topic

`tsuku install <alias>` should present an arrow-driven picker when multiple
recipes satisfy the alias (interactive TTY), or fail with a candidate list when
running non-interactively (`-y`).

## Pre-decided constraints (from user prompt — NOT to re-litigate)

1. Exact alias matches under `-y` → auto-satisfied without prompting.
2. Non-exact (multi-satisfier) under `-y` → fail with helpful error listing candidates.
3. Default (interactive TTY) → arrow-driven picker.
4. Picker is **arrow-key driven**, not number-typing.

## What we already know

- Single satisfier today: `[metadata.satisfies] homebrew = ["openjdk"]`-style;
  index is `map[string]satisfiesEntry` (1:1).
- `tsuku install java` today errors with `Multiple sources found` referencing
  the discovery layer (rubygems:java, npm:java) — a *different* mechanism than
  the satisfies index.
- `--from <ecosystem>:<package>` flag exists today for the discovery
  ambiguity case.
- TTY detection precedent: tsuku already uses some progress reporter that
  detects TTY (per the update-output regression work earlier this week).
- The four JDK recipes (openjdk, temurin, corretto, microsoft-openjdk) are
  the proof point in PR #2362.

## Research leads (Round 1)

Each lead is a question whose answer will inform PRD requirements and
unblock the eventual design without re-deciding what the user already
specified.

1. **Arrow-driven TUI options in Go** — what's the right library
   (bubbletea, promptui, survey, hand-rolled with golang.org/x/term)?
   Trade-offs: dependency size, binary growth, terminal compatibility,
   project license, maintenance status. Look at what tsuku already
   imports for UI work.

2. **Schema shape for multi-satisfier aliases** — `[metadata.satisfies]
   aliases = ["java"]` vs `provides = ["java"]` (apt-style) vs reusing the
   ecosystem-keyed `satisfies.homebrew`. Read the existing types.go,
   loader.go satisfies index, and a few real recipes that use satisfies
   today. Goal: the smallest change that lets multiple recipes claim
   the same alias without breaking the existing 1:1 ecosystem semantics.

3. **Resolution semantics: exact vs non-exact** — what counts as "exact
   match" for alias resolution? Recipe-name match (e.g., `tsuku install
   openjdk` → recipes/o/openjdk.toml exists)? Single-satisfier alias
   (e.g., one recipe satisfies "openjdk")? Both? Is there a precedent
   in the loader/install path for distinguishing these cases? Make sure
   the user's pre-decided constraint #1 ("exact alias matches under -y
   are auto-satisfied") has a precise meaning.

4. **Existing `Multiple sources found` discovery flow** — read where this
   error is raised, how `--from <ecosystem>:<package>` is parsed, and
   how it integrates with the install plan generation. Check whether
   the new picker should subsume this flow or stay parallel to it.
   Decision driver: should the picker show the discovery hits too,
   or only first-class satisfier recipes?

5. **TTY detection in tsuku** — what package does tsuku currently use
   to detect "is stdout a TTY"? Are there other interactive prompts
   in the CLI to model after (confirmation prompts, etc.)? Check
   `cmd/tsuku/`, `internal/progress/`, and any `term.IsTerminal` calls.

6. **Plan caching + determinism** — picker output is non-deterministic
   (depends on user choice). How does this interact with `--plan` (use
   pre-computed plan), plan hashing, and `-y`/`--force`? The
   non-interactive path resolving via `--from` should produce a
   stable plan hash; the interactive path's resolved plan is also
   stable once the user picks. Worth confirming the model.

7. **Edge cases** — what if the alias collides with an actual recipe
   name? (e.g., a future `recipes/j/java.toml` plus other recipes
   satisfying "java"). What if a recipe satisfies an alias *and* is
   the canonical recipe for that name? Are there precedents in the
   codebase for prefer-direct-name-over-satisfies semantics?

8. **CI / non-TTY error format** — the user wants a "helpful error
   listing candidates" under `-y`. Look at the existing
   `Multiple sources found` error wording; the new error should be
   parallel in shape (printed to stderr, lists candidates, exits
   non-zero) so scripts/dashboards already wired for it Just Work.

## Coverage notes

Out of scope for this exploration:
- Whether to add the picker (decided).
- Picker style (arrow-driven, decided).
- `-y` semantics (decided per constraint #1 / #2).
- Specific recipe-side schema (one of the leads picks a winner; no
  open-ended ideation).

# Exploration Decisions: 2368-multi-satisfier-picker

## Round 1

- **TUI: `golang.org/x/term` + hand-rolled ANSI** (no bubbletea/huh/promptui/survey): already imported, matches the established `internal/progress/` pattern, zero added binary size, ~200 LOC.
- **Schema: `[metadata.satisfies] aliases = ["java"]`** (extend existing satisfies, not a new top-level `provides` field): `Satisfies map[string][]string` already accepts arbitrary keys, smallest blast radius.
- **Resolution precedence: direct-name match wins over satisfies index**: matches current behavior and the established `TestSatisfies_GetWithContext_ExactMatchTakesPriority` precedent.
- **Discovery: subsume** (when ≥1 first-class satisfier exists, hide discovery hits): consistent with curated > discovery elsewhere; avoids the `tsuku install java` → `--from rubygems:java` wrong-tool problem.
- **TTY detection: `golang.org/x/term.IsTerminal`**: already used at `cmd/tsuku/install.go:31-34`.
- **Error format under `-y`: parallel to existing `Multiple sources found`**: same exit code (10), same `--json` structure, recipe-name form of `--from` (no colon).
- **No executor/cache changes**: picker decision bakes into `InstallationPlan.Tool`; plan hash is stable; state.json already records resolved recipe name.
- **No picker for runtime dependencies**: deps must resolve deterministically (one recipe). Validator should reject a recipe whose runtime_dependencies includes a multi-satisfier alias.
- **Skip the `provides` (apt-style) shape**: extending satisfies is smaller and reads cleaner alongside ecosystem entries.

## Round 1 → Crystallize

- Recommendation routes to PRD per user's explicit instruction (PRD → DESIGN → PLAN → /work-on).

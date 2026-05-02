# Explore Findings: 2368-multi-satisfier-picker

## Round 1 — six leads dispatched, all returned

### Lead 1: TUI library
**Pick: `golang.org/x/term` + hand-rolled ANSI** (no external TUI library).
- `golang.org/x/term` is already a direct dependency.
- tsuku's `internal/progress/` already uses raw ANSI + x/term for the spinner; the picker would follow the same precedent and live alongside in `internal/tui/picker.go`.
- bubbletea (~1.2 MB), huh (~1.5 MB), promptui (unmaintained 2020), survey (last release 2022), fzf bindings (overkill) all rejected.
- ~150–200 lines for arrow-key single-select with Enter to confirm.

### Lead 2: Schema shape
**Pick: extend existing `[metadata.satisfies]` with a non-ecosystem `aliases` key.**
- `Satisfies map[string][]string` already accepts arbitrary keys, so no struct change is needed.
- Validator change: `validateSatisfies` skips ecosystem-name validation for the `aliases` key.
- Index builder change: `buildSatisfiesIndex` flattens `Satisfies["aliases"]` entries into the lookup index but tracks them as multi-eligible (vs. ecosystem entries which stay 1:1).
- Smallest blast radius; existing recipes unaffected.

### Lead 3: Resolution semantics
**Truth table** (codegrounded against `loader.go:resolveFromChain`):

| Case | Direct name match | Satisfies entries | `-y` off (TTY) | `-y` on |
|------|:-:|:-:|---|---|
| A. Canonical recipe exists, no alias | ✓ | n/a | auto-resolve | auto-resolve |
| B. Alias with 1 satisfier | ✗ | 1 | auto-resolve | auto-resolve |
| C. Alias with 2+ satisfiers | ✗ | ≥2 | **picker** | **fail with candidate list** |
| D. No match | ✗ | 0 | discovery fallback | recipe-not-found |
| E. Direct name + multi-satisfier collision | ✓ (wins) | ≥2 | auto-resolve to direct | auto-resolve to direct |

**Exact match definition:** either (i) a recipe of that exact name exists OR (ii) the satisfies index returns exactly one recipe. Both are unambiguous and skip the picker.

**Direct-name-vs-satisfies precedence:** keep current "direct name wins" — already enforced and tested (`TestSatisfies_GetWithContext_ExactMatchTakesPriority`). Recipe authors retain control of their canonical names.

**New API needed:** `loader.LookupAllSatisfiers(alias) []string` — returns ALL satisfying recipe names. Existing `LookupSatisfies` returns just one.

### Lead 4: Discovery interaction
**Pick: subsume.** When at least one first-class satisfier exists for an alias, hide discovery hits entirely. `--from rubygems:java` still works as an explicit override.
- Rationale: tsuku's curated > discovery priority everywhere else.
- Avoids the wrong-tool problem (`tsuku install java` should not show the Ruby gem).
- Code change: in `cmd/tsuku/install.go` lines 305–312, call satisfier lookup BEFORE `tryDiscoveryFallback`. If satisfiers > 0, branch to picker/error path.

### Lead 5: TTY detection + error format
**TTY detection:** `golang.org/x/term.IsTerminal(int(os.Stdout.Fd()))` — already used at `cmd/tsuku/install.go:31-34`. Reuse for picker gating.

**`-y` semantics:** `-y` and "TTY available" are independent gates. `-y` forces approval regardless of TTY; non-TTY auto-approves where prompts exist. Pattern: `if !autoApprove && isInteractive() { prompt }`.

**Existing prompt patterns:** `(y/N)` to stderr is the established style.

**Existing error format** (verbatim):
```
Multiple sources found for "TOOL". Use --from to specify:
  tsuku install TOOL --from BUILDER:SOURCE
  tsuku install TOOL --from BUILDER:SOURCE
```
Exit code: 10 (`ExitAmbiguous`). `--json` mode emits the same in structured form.

**Proposed parallel error** for multi-satisfier under `-y`:
```
Multiple recipes satisfy alias "java". Use --from to specify a recipe:
  tsuku install java --from openjdk
  tsuku install java --from temurin
  tsuku install java --from corretto
  tsuku install java --from microsoft-openjdk
```
Same exit code (10) and `--json` shape for consistency.

**Note on `--from`:** today's flag parses `<ecosystem>:<package>`. The new picker selection should reuse the same flag with a recipe-name form (no colon) — `--from temurin`. The existing parser distinguishes by colon presence, so this is additive.

### Lead 6: Cache + edge cases
- **Plan caching:** picker decision is baked into `InstallationPlan.Tool` (the resolved recipe name). Plan hash is stable once the user picks. `--plan plan.json` replays without re-prompting. **No executor/cache changes needed.**
- **State.json:** records the resolved recipe name. `tsuku update` reads from state, so it knows which recipe to refresh — no re-pick needed.
- **Runtime dependency containing an alias:** plan generation is deterministic; the picker MUST NOT engage in dep resolution. Recommendation: validate at recipe parse time that any dependency named as an alias has exactly one satisfier (otherwise the recipe is unbuildable).
- **--from interaction:** orthogonal code path, no conflict.

## Cross-cutting consistency check

All six leads agree on these load-bearing points:
- `golang.org/x/term` is the right dependency surface (lead 1, lead 5).
- `aliases` as a satisfies key is the cleanest schema (lead 2), and it slots into the picker code path (lead 3) without changing the plan/cache layer (lead 6).
- Subsuming discovery (lead 4) and the parallel error format (lead 5) give the user a single mental model: "tsuku considers curated recipes first; if multiple satisfy, you pick."
- Direct-name-wins (lead 3) and dep-resolution-no-picker (lead 6) preserve determinism for everything except the explicit user-typed top-level install.

No contradictions surfaced.

## Open questions

None that block PRD writing. The user's pre-decided constraints settled the four "what" questions; the leads settled the "how" questions concretely.

A few items worth surfacing in the PRD as explicit decisions (so the design phase doesn't reopen them):
- "How does the picker render visually?" — line per candidate with arrow on the selected row, Enter to confirm, Ctrl-C to cancel. Show recipe name + description.
- "What gets recorded in state.json after a pick?" — the resolved recipe name (already standard for any install).
- "Does the picker engage for `tsuku run <alias>` and `tsuku info <alias>`?" — out of scope for this issue; install only. Other commands continue to error or use first match per their existing semantics.

## Decision: Crystallize

Ready for the artifact-type decision. Recommendation: **PRD** (per user instruction; multi-feature change with both schema and CLI surfaces, requirements worth pinning before design).

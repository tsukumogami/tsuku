# Crystallize: 2368-multi-satisfier-picker

## Decision

Route to **PRD** (`/prd`) per the user's explicit instruction. Then DESIGN, then PLAN (single-pr), then /work-on.

## Why PRD specifically

The exploration converged on a coherent set of constraints (TUI choice, schema shape, resolution semantics, discovery interaction, TTY detection, error format, cache neutrality), but writing them up as a PRD before designing has real value here:

- The schema change touches every recipe author's mental model. The PRD is where we pin the requirement that the new `aliases` key behaves differently from the existing ecosystem-keyed entries (multi-satisfier, picker-eligible) so the design can refer back to a stable spec.
- The CLI behavior matrix (cases A–E) is requirement-shaped, not design-shaped. The PRD locks the truth table; the design picks the code structure that delivers it.
- The user's pre-decided constraints (1–4) become AC items in the PRD, which makes them auditable through implementation rather than buried in chat history.

## Carry-forward inputs for /prd

The PRD should incorporate (without re-litigating):

**From the user's prompt — pre-decided requirements:**
1. Exact alias matches under `-y` → auto-satisfied (no prompt).
2. Non-exact (≥2 satisfiers) under `-y` → fail with helpful error listing candidates.
3. Default (interactive TTY) → arrow-driven picker.
4. Picker is arrow-key driven, NOT number typing.

**From exploration leads — picked answers (move forward, don't reopen):**
- Schema: `[metadata.satisfies] aliases = ["java"]`.
- TUI: `golang.org/x/term` + ANSI (no external lib).
- Direct-name precedence: keep current behavior (direct match wins).
- Discovery interaction: subsume when satisfiers exist.
- Error format: parallel to existing `Multiple sources found` (exit 10, `--json`, `--from <recipe-name>` for selection).
- Plan/cache: no changes; resolved recipe name is the single source of truth post-pick.
- Runtime deps: validator rejects multi-satisfier aliases in `runtime_dependencies`.

## Handoff

Next command: `/prd 2368-multi-satisfier-picker --auto`. Issue context: github.com/tsukumogami/tsuku#2368.

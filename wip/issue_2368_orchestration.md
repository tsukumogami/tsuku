# Orchestration plan: issue #2368

**Original prompt** (typos fixed; refer back to this after every compaction):

> in a new branch, let's first /explore 2368 to make sure we understand
> it from all possible angles and prevent getting blindsided. What I think
> is that exact matches on alias should be satisfied automatically when
> running non-interactively (-y flag); non-exact matches should fail if
> running non-interactively, or show a picker if running interactively
> (default). I want the picker to be arrow driven, not number typing.
> I want you to run in --auto all the way to implementation. You MUST
> start with a PRD (/prd), proceed into a DESIGN (/design), then a
> single-pr PLAN (/plan), and then implementation (/work-on). All of
> this in a single branch, all of this in --auto mode (approve the docs
> yourself). Whenever you need to make a decision, use the /decision
> skill.
>
> You have all night to finish this, so take your time. Write this
> prompt in the wip/ folder and take a note to refer back to it after
> each compaction.

## Sequence

1. **`/explore 2368 --auto`** — investigate from all angles before committing to an artifact
2. **`/prd <topic from explore> --auto`** — capture requirements
3. **`/design <prd-path> --auto`** — produce technical design
4. **`/plan <design-path> --single-pr --auto`** — single-PR plan
5. **`/work-on` each issue/outline produced by /plan, --auto`** — implement

## Branch

- `feat/2368-multi-satisfier-picker` (already created off main, has the
  plan-doc commit recording #2368 as a co-blocker for #2327)
- Single branch for everything per user instruction

## Mode

- `--auto` for every step. Approve PRD/design/plan myself in --auto mode.
- Use `/decision` skill (or `koto decisions record`) for any non-obvious
  decision encountered along the way.

## User-stated design constraints (from the prompt)

These are NOT to be re-litigated by the explore/PRD/design — they're
already-decided requirements I must carry forward:

1. **Exact alias matches under `-y` (non-interactive)** are auto-satisfied
   without prompting.
2. **Non-exact matches (multi-satisfier) under `-y`** must FAIL with a
   helpful error listing the candidates.
3. **Default mode (interactive TTY)** shows a picker.
4. **Picker is arrow-key driven, not number typing.** This rules out
   simple `read`-based prompts; needs a TUI library (likely `bubbletea`
   or similar) or terminal cursor handling.

## Refer-back note

After every context compaction, re-read this file first to remind
myself of:
- The user's exact prompt
- The four design constraints (above)
- Where I am in the sequence
- That I'm authorized to approve docs myself in --auto mode

## Status

- [x] /explore — round 1, 6 leads, no contradictions; crystallized to PRD
- [x] /prd — `docs/prds/PRD-multi-satisfier-picker.md`, Accepted (now In Progress), 17 ACs
- [x] /design — `docs/designs/DESIGN-multi-satisfier-picker.md`, Accepted, 5 decisions + security review
- [ ] /plan (single-pr)
- [ ] /work-on (one or more issues, depending on what /plan produces)
- [ ] PR open
- [ ] CI green

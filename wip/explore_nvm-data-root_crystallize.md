# Crystallize Decision: nvm-data-root

Evaluated in `--auto` mode per the dispatch brief's autonomy mandate. The
recommendation was followed rather than presented for confirmation; this file is
the record.

## Chosen Type

Design Doc

## Rationale

What to build was given, not discovered: issue #2464 carries six acceptance
criteria and they survived the exploration intact. What the exploration produced
instead is an option space — twelve candidate shapes, ten of which were eliminated
with concrete evidence — and a live architectural choice between the two that
remain, gated on a product question about removal semantics.

That is the Design Doc signature exactly. It also matters that the *eliminations*
are the expensive part of what this exploration produced. `nvm cache clear` deleting
a symlink, `install_shell_init` copying rather than sourcing nvm.sh, `set_env`
single-quoting its values, `ReapVersion` no-opping instead of objecting on an
unrecognized version, `nvm-exec` self-locating through an unresolved
`BASH_SOURCE[0]` — each of those is a line of evidence that kills an option a future
contributor would otherwise re-propose. `wip/` is deleted before the PR merges. If
that reasoning is not written to a durable document it is lost, and the next person
to look at nvm starts from the same three candidates the issue listed.

## Signal Evidence

### Signals Present

- **What to build is clear, but how to build it is not** — the acceptance criteria
  are settled; the mechanism is not.
- **Technical decisions need to be made between approaches** — a tsuku-owned stable
  root versus nvm's own default root, plus how `nvm exec` survives a program/data
  split, plus where migration logic lives.
- **Architecture and integration questions remain** — a new `set_env` template
  variable is required either way; whether `$TSUKU_HOME/share/` is the right tree for
  irreplaceable user data when it also holds three directories tsuku freely
  regenerates; whether tsuku grows its first `delete_dir` producer.
- **Exploration surfaced multiple viable implementation paths** — twelve enumerated,
  two survive.
- **Decisions made during exploration that should be on record** — ten eliminations,
  each with a citation.
- **The core question is "how should we build this?"** — yes.

### Anti-Signals Checked

- *What to build is still unclear* — not present. The issue states it and the
  exploration only sharpened the problem statement, it did not reopen it.
- *No meaningful technical risk or trade-offs* — not present. The two survivors
  differ on whether user data lives inside `$TSUKU_HOME`, which changes what
  `tsuku remove` means.
- *Problem is operational, not architectural* — not present.

Score: 6 signals, 0 anti-signals.

## Alternatives Considered

- **Decision Record** — scored well (a single "which option and why" with compared
  alternatives) but carries its own anti-signal: *multiple interrelated decisions
  need a design doc*. There are at least four here — data root location, how
  `nvm exec` survives, what `tsuku remove nvm` does, and where migration runs — and
  they are coupled, since the removal answer determines the location answer. Demoted
  below Design Doc by the demotion rule. The design should still run the decision
  framework internally on the location question.

- **PRD** — anti-signal present: requirements were provided as input. The issue's
  acceptance criteria were not produced by this exploration.

- **Plan** — two anti-signals: the technical approach is still debated and open
  architectural decisions come first. A plan cannot sequence work that has not been
  decided. It is the right *next* artifact, which is why the design flows straight
  into one.

- **No Artifact** — anti-signal present: architectural decisions were made during
  exploration. Urgency does not override capturing them; the correct response is a
  lean design doc plus immediate implementation, not skipping the doc.

- **Spike Report** — feasibility was the exploration's input question and it is now
  answered ("the split is legal, and here is the one line that costs"). Nothing
  remains time-boxed or uncertain enough to spike.

- **VISION, Roadmap, Rejection Record, Competitive Analysis** — no signals. Single
  feature inside an existing project, proceeding rather than rejected, and the repo
  is public (competitive analysis is private-only).

## Deferred Types

None matched. Prototype was checked and does not fit — the empirical questions were
answered during exploration rather than left for a proof-of-concept.

## Handoff

Proceed to the tactical chain via `/scope`, which drives DESIGN then PLAN. The
design's first job is the location decision; the exploration's findings and option
table are its input, and the option ranking should not be re-derived from scratch.

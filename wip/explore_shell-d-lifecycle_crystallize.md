# Crystallize Decision: shell-d-lifecycle

## Recommended: Design Doc, then Plan

Score 6, zero anti-signals. Every Design Doc signal is present:

- **What to build is clear, how to build it is not.** The dispatch brief supplies
  explicit acceptance criteria — multi-version removal, activate, rollback,
  `ContentHash` consistency, both writing actions, a subshell-level test. What none of
  that settles is the mechanism.
- **Technical decisions need to be made between approaches.** Round 1 characterized six
  candidate mechanisms and produced a strongest-objection argument against each. None
  is dominated.
- **Exploration surfaced multiple viable implementation paths.** Replay the stored
  plan, a render capability separate from `Execute`, regenerate from the recipe, store
  rendered content in state, a stable per-tool indirection path, delete-plus-lazy
  regeneration. Prior art suggests the answer combines two of them rather than picking
  one.
- **Architecture and integration questions remain.** Whether to sever the
  `version -> install` import edge determines where the re-render hook can physically
  sit, and therefore whether `internal/updates`' direct `Manager.Activate` call is
  covered.
- **Decisions made during exploration belong on the record.** Round 1 corrected three
  premises the work started from: stable indirection is *not* closed off, storing
  content was ruled out for the wrong reason, and only removing the *active* version is
  broken. It also found two latent bugs adjacent to this work — `ToStoragePlan` silently
  dropping `Phase`/`Dependencies`/`Verify`, and `doctor --fix` being non-convergent.
  `wip/` is deleted before merge, so anything left only in research files is lost.
- **The core question is "how should we build this?"** Yes.

## Alternatives considered

**PRD** — demoted on an anti-signal. Requirements were given as input to the
exploration rather than discovered by it; the brief's acceptance criteria are the
requirements contract. Writing a PRD would restate them.

**Plan** — demoted on two anti-signals: the technical approach is still debated and
open architectural decisions must be made first. A Plan cannot sequence work whose
mechanism is undecided. It is the right *second* artifact, once the design lands.

**Decision Record** — demoted. The scoring rule sends multiple interrelated decisions
to a design doc, and there are at least four here (mechanism, driver placement, writer
scope, repair path) that constrain one another. A standalone ADR would fragment them.

**No artifact** — demoted. Architectural decisions were made during exploration, and
the work touches four packages plus three plugin skills.

## Routing

`shirabe:scope` drives the tactical chain to a PLAN. BRIEF and PRD are redundant here —
the brief already frames the problem and states acceptance criteria — so the chain
should enter at DESIGN and terminate at PLAN.

The design phase must use the `shirabe:decision` framework on the mechanism question.
It is the contested choice, the brief designates that framework for exactly this, and
six characterized candidates with no dominant winner is precisely its input shape.

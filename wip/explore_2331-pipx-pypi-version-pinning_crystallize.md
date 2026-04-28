# Crystallize Decision: pipx PyPI version pinning (#2331)

## Chosen Type

Design Doc

## Rationale

Requirements were given as input — issue #2331 states what to build
("pipx_install recipes must succeed when latest PyPI release drops
support for bundled Python") and acceptance criteria. Exploration
did not need to discover what; it needed to figure out **how**.
Across five research leads, the exploration:

- Reframed the problem (root cause is tsuku pre-pinning to absolute
  latest, not pip's behavior)
- Surfaced four viable implementation paths (auto-filter / manual
  constraint / hybrid / no pre-resolve)
- Made architectural decisions worth permanent record (Option A
  chosen, recipes carry no version pins, scope narrowed to PyPI
  provider only, azure-cli deferred to a follow-up)
- Identified concrete components requiring design (PEP 440 specifier
  evaluator, bundled-Python constants package, PyPIProvider
  integration point)

These decisions need to live somewhere durable — `wip/` is wiped
before merge, so without a design doc, the rationale for choosing
A and the deferral of azure-cli would be lost.

## Signal Evidence

### Signals Present (Design Doc)

- **What to build is clear, but how to build it is not.** Issue
  #2331 names the failure (ansible recipe broken on Python 3.10);
  exploration discovered the actual root cause and resolution
  approach.
- **Technical decisions need to be made between approaches.**
  Four candidates were evaluated; one chosen with explicit
  trade-offs (Option A: ~100 LOC, no recipe burden, doesn't help
  azure-cli).
- **Architecture, integration, or system design questions remain.**
  Specifically: where to plug Python-compat filtering (provider
  construction vs. Decompose), how to expose bundled Python
  major.minor (constants package vs. plumbing through factory),
  what PEP 440 subset to support.
- **Exploration surfaced multiple viable implementation paths.**
  See findings file's "Decision space" section.
- **Architectural or technical decisions were made during
  exploration that should be on record.** A chosen over B/C/D;
  scope narrowed to PyPI; azure-cli deferred. All need durable
  documentation.
- **The core question is "how should we build this?"** Confirmed
  by the convergence pattern.

### Anti-Signals Checked

- **What to build is still unclear** — not present. Issue states
  the goal precisely.
- **No meaningful technical risk or trade-offs** — not present.
  Trade-offs evaluated explicitly across A/B/C/D.
- **Problem is operational, not architectural** — not present.
  The fix touches version-resolution architecture.

## Alternatives Considered

- **PRD** — Demoted. Anti-signal "requirements were provided as
  input" is present (issue #2331 states acceptance criteria
  explicitly). The "what" was not discovered during exploration;
  only the "how."
- **Decision Record** — Scored 2 raw but demoted by anti-signal
  "multiple interrelated decisions need a design doc." This isn't
  a single A-vs-B choice — it's filter mechanism + new constants
  package + new PEP 440 evaluator + scope deferral, which is
  design-doc territory.
- **No Artifact** — Demoted. Anti-signal "any architectural,
  dependency, or structural decisions were made during exploration"
  is strongly present. Implementing directly would lose the
  rationale for choosing A over B/C and for deferring azure-cli.
- **Plan** — Demoted. Anti-signal "open architectural decisions
  need to be made first" is present — the design has been chosen
  conceptually but not written. A Plan presupposes a written
  design to decompose.
- **Spike, VISION, Roadmap, Rejection Record, Competitive Analysis**
  — clearly don't fit; not scored in detail.

## Deferred Types

None.

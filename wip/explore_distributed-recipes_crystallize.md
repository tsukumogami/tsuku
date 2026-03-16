# Crystallize Decision: distributed-recipes

## Chosen Type
PRD

## Rationale
The exploration confirmed that requirements are the primary gap, not technical
approach. The issue (#2073) is a brain dump of 10 design questions with no
prioritization or user-perspective framing. The user explicitly identified that
the issue is "just a seed" and that a PRD should capture WHAT we're building
and WHY before committing to HOW.

Multiple user personas emerged (tool authors shipping recipes, end users
installing from remote sources, enterprise teams locking down registries) that
need requirement-level definition before architecture.

The user also expanded scope beyond the original issue: all registries (embedded,
central, local, distributed) should be unified under one model. This scope
expansion is a requirements decision, not a technical one.

## Signal Evidence
### Signals Present
- Single feature area with unclear requirements boundaries (what's in v1?)
- Multiple user personas with different priorities (tool author vs end user vs enterprise)
- "What to build" is the open question (the issue lists 10 areas without prioritizing)
- Requirements were discovered during exploration (registry unification, trust model with strict config)
- Scope was explicitly expanded by the user beyond the original issue

### Anti-Signals Checked
- Requirements were NOT given as input (they were discovered) -- not present
- Multiple independent features -- not present (this is one cohesive feature area)

## Alternatives Considered
- **Design Doc**: Partially fits since there are technical decisions (manifest schema,
  RecipeProvider interface). But the "what" isn't settled enough for "how" to be
  productive. The PRD will feed into a design doc.
- **Plan**: Too early -- neither requirements nor approach are documented.
- **No artifact**: Not appropriate -- significant decisions were made during exploration
  (trust model, registry unification scope) that need to be captured permanently.

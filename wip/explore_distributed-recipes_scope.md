# Explore Scope: distributed-recipes

## Core Question

How should tsuku model registries as a unified concept, where embedded recipes,
the central registry, local user recipes, and third-party repo recipes are all
instances of the same abstraction? The distributed/remote case is the new
capability, but the PRD should define the registry model holistically so that
existing sources and new remote sources share a common interface.

## Context

Issue #2073 proposes distributed recipe support -- letting tool authors ship
tsuku recipes in their own repos. The issue is detailed (install syntax, manifest
format, security model) but reads as a brain dump, not validated requirements.

The user's insight is that this isn't just "add remote sources" -- it's an
opportunity to unify ALL registries under one model. Today tsuku has embedded
recipes (compiled into the binary), a central registry (fetched from tsuku.dev),
and a local cache ($TSUKU_HOME/registry). These are handled by different code
paths. A unified registry abstraction would make distributed recipes a natural
extension rather than a bolt-on.

Claude Code's `.claude-plugin/marketplace.json` is cited as a reference for how
a tool ecosystem standardizes third-party package discovery and distribution.

koto (tsukumogami/koto) is the validation candidate -- its recipe should move
from the central registry to koto's own repo as a proving ground.

No external users yet, so breaking changes are acceptable.

## In Scope

- Registry abstraction model (what makes something "a registry")
- How existing sources (embedded, central, local) fit the unified model
- Install syntax for remote/distributed sources
- Trust and security model for third-party registries
- Manifest schema that works across all registry types
- Claude Code marketplace.json as prior art
- koto as validation scenario

## Out of Scope

- Discovery system integration (#1301) -- TBD, likely limited to official/trusted registries
- Recipe contribution workflow (#1299) -- adjacent but separate concern
- Enterprise/private git server support -- may be deferred from v1
- Recipe generation from remote sources (the `--from` builder path)

## Research Leads

1. **How does Claude Code's plugin marketplace model work, and what can we learn from it?**
   The `.claude-plugin/marketplace.json` pattern is explicitly cited as a reference.
   Understanding its schema, trust model, and registry-of-registries approach
   would ground the PRD in concrete prior art.

2. **What do Homebrew taps, Cargo alternative registries, and Go module proxies look like from the user's perspective?**
   These are the analogues the issue mentions. What UX patterns do they share,
   where do they diverge, and what friction points have their communities reported?

3. **How does tsuku currently model its different recipe sources, and where does the abstraction break?**
   Embedded recipes, central registry, and local cache are three existing "registries."
   Mapping the current code paths would reveal what unification requires and what
   the natural seams are.

4. **What trust and security models do other tools use for third-party sources?**
   Homebrew taps are trust-on-first-use. Cargo registries require explicit opt-in.
   Go modules use checksums via sumdb. What's appropriate for tsuku's threat model
   and user base?

5. **What does "registering a registry" look like in practice?**
   The user mentioned "installing/registering a registry" as a possible trust
   mechanism. What prior art exists for registry-of-registries patterns, and
   what UX overhead do they impose?

6. **What's the minimal viable registry manifest schema that works for all registry types?**
   The current `manifest.go` serves the central registry with schema versioning
   and deprecation notices. Could a variant work for a repo's `.tsuku-recipes/`,
   for embedded recipes, and for local user recipes?

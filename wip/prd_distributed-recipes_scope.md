# /prd Scope: distributed-recipes

## Problem Statement

tsuku's recipe registry is centralized: every recipe lives in the monorepo's
`recipes/` directory, and tool authors must contribute there to make their
software installable. This creates friction for authors who already manage
their own releases, and it couples tsuku's growth to a single contribution
bottleneck. Meanwhile, the CLI's internal handling of recipe sources (embedded,
central registry, local cache) uses separate code paths with no shared
abstraction, making it harder to add new source types.

## Initial Scope

### In Scope
- Unified registry model: all recipe sources (embedded, central, local,
  distributed) as instances of a common abstraction
- Install syntax for remote sources (`owner/repo`, `owner/repo:recipe@version`)
- Auto-registration by default: first install from an unknown source registers
  it implicitly (Homebrew tap model)
- Strict mode config (off by default): blocks auto-registration, requires
  explicit `tsuku registry add`
- `tsuku registry add/list/remove` commands for explicit registry management
- Manifest schema that works across all registry types (per-recipe metadata
  layer + optional registry envelope)
- State tracking for distributed installs in state.json
- koto recipe migration to tsukumogami/koto as validation scenario

### Out of Scope
- Discovery system integration (#1301) -- only official/trusted registries
  for now
- Recipe contribution workflow (#1299) -- adjacent, separate concern
- Cryptographic signing / Sigstore verification -- deferred until user base exists
- Enterprise features (private git servers, SSO auth) -- may be deferred from v1
- Recipe generation from remote sources (the `--from` builder path)

## Research Leads

1. **Who are the user personas and what are their priorities?**
   Exploration identified tool authors, end users, and enterprise teams.
   The PRD needs to define primary vs secondary personas and what success
   looks like for each.

2. **What's the minimum viable third-party registry?**
   A single `.tsuku-recipes/tool.toml` in a repo? A full manifest.json?
   Something in between? The exploration found tension between no-manifest
   simplicity and manifest-required discoverability.

3. **How should recipe name conflicts across sources be resolved?**
   The exploration found that deterministic package-to-source binding is
   critical (pip's disaster). But with auto-registration, two sources could
   have the same recipe name. Need resolution rules.

4. **What does `@latest` mean for distributed recipes?**
   HEAD of default branch? Latest git tag? Latest GitHub release? Each has
   different reproducibility implications. The exploration didn't settle this.

5. **How should state.json track distributed installs?**
   Currently tracks tool name and version. Distributed installs need source
   identity (which registry, which ref). Need schema extension.

6. **What commands are affected and how?**
   install, remove, list, update, info, outdated, verify -- each needs to
   handle distributed sources. The PRD should define expected behavior per
   command.

## Coverage Notes

The exploration covered competitive landscape thoroughly (Homebrew taps, Cargo
registries, Go modules, npm, Nix, mise/aqua, Claude Code marketplace) and
established the trust model (auto-register + strict config). Gaps remaining:

- No deep dive on koto's current recipe or what migration specifically requires
- Authentication for private repos not explored in depth
- `@latest` semantics and version resolution for git-based sources
- Impact on existing commands (update, outdated, verify) not mapped
- No research on how `tsuku update-registry` should behave with multiple registries

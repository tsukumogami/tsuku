# Explore Scope: org-scoped-project-config

## Visibility

Public

## Core Question

How should `.tsuku.toml` represent org-scoped tools (like `tsukumogami/koto`) so the config is self-contained and works on fresh machines without prior `tsuku install` runs? The current TOML syntax can't represent `org/tool` as a bare key, and even quoted keys fail at lookup time because the registry isn't registered yet.

## Context

Issue #2230 documents five different syntax attempts for org-scoped recipes in `.tsuku.toml`, none of which work reliably. The only working combination requires prior manual `tsuku install` to auto-register the registry. This breaks CI use cases where there's no prior state. The user wants to draw inspiration from Claude's plugin marketplace and settings.json for how they handle namespaced tool references.

## In Scope

- `.tsuku.toml` syntax for org-scoped tools
- Registry auto-discovery from project config
- CI-friendliness (self-contained config, no prior state)
- TOML syntax constraints and workarounds

## Out of Scope

- Registry authentication / private registries
- Recipe format changes
- New registry protocol (HTTP API, etc.)

## Research Leads

1. **How does Claude's settings.json and plugin marketplace model namespaced plugin references?**
   Direct inspiration source. Need to understand the config format, namespace resolution, and any registry/marketplace discovery mechanisms.

2. **How do other CLI tools represent namespaced packages in config files?**
   npm has `@scope/package`, Cargo has `[registries]`, Homebrew has taps. What patterns exist and which translate well to TOML?

3. **What TOML-valid syntax options exist for representing `org/tool` pairs?**
   Quoted keys, dotted tables, separate `[registries]` section, or a different value format. What are the trade-offs of each?

4. **How does tsuku's current `.tsuku.toml` parsing and project config system work?**
   Need to understand the code path from config read to recipe lookup to know what's fixable vs needs rearchitecting.

5. **How does tsuku's registry system currently handle org-scoped recipes?**
   The issue says `tsuku install tsukumogami/koto` auto-registers the registry. What's the registry resolution flow, and can it be triggered from config?

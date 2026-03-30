# DESIGN: Channel-aware version resolution

## Status

Proposed

## Context and Problem Statement

Tsuku stores the user's install-time version constraint in the `Requested` field of state.json (e.g., `tsuku install node@18` stores `Requested: "18"`), but `tsuku update` ignores it entirely. Running `tsuku update node` resolves to the absolute latest version from the provider, silently jumping from Node 18.x to Node 22.x. This breaks user expectations and makes any future auto-update system unsafe.

A second bug compounds the problem: `tsuku outdated` only checks tools sourced from GitHub releases. Tools installed from PyPI, npm, crates.io, RubyGems, Homebrew, and other providers are invisible to the outdated command, even though ProviderFactory already supports resolving versions from all of these sources.

This design establishes the version pinning model and fixes both commands. It's the foundation that Features 2-9 of the auto-update roadmap depend on.

## Decision Drivers

- **Backward compatibility**: The `Requested` field already exists in state.json. Any solution must work with existing installations without migration.
- **Simplicity**: Users already type version constraints naturally (`node@18`, `kubectl@1.29`). The model should match that intuition without new syntax.
- **Provider-agnostic**: Must work identically across all version provider types (GitHub, PyPI, npm, crates.io, etc.), despite differences in version numbering conventions.
- **Performance**: Version resolution is called during install, update, and outdated. Caching `ResolveLatest` results reduces redundant network calls.
- **Foundation for auto-update**: The pin-level model defined here will be reused by background checks (Feature 2), auto-apply (Feature 3), notifications (Feature 5), and outdated display (Feature 6).

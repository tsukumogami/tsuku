---
status: Proposed
problem: |
  Org-scoped recipes (like tsukumogami/koto) have no working syntax in
  .tsuku.toml. The TOML format forbids slash in bare keys, the project install
  path lacks distributed-name detection, and the resolver's binary index uses
  bare recipe names that don't match org-prefixed config keys. This makes
  .tsuku.toml impractical for CI environments that need self-contained config.
---

# DESIGN: Org-Scoped Project Config

## Status

Proposed

## Context and Problem Statement

Issue #2230 documents five different syntax attempts for org-scoped recipes in `.tsuku.toml`, none of which work reliably. The only combination that works (`"tsukumogami/koto" = ""` with a pre-registered registry) requires prior manual `tsuku install` -- defeating the purpose of declarative project config.

The problem has three layers. First, TOML syntax: bare keys can't contain `/`, so `tsukumogami/koto = "latest"` is a parse error. Second, runtime: `runProjectInstall` passes tool names directly to `runInstallWithTelemetry` without distributed-name detection, so even quoted keys like `"tsukumogami/koto"` fail at recipe lookup. Third, the resolver: the binary index stores bare recipe names (`koto`), but the config map is keyed by the full org-scoped name (`tsukumogami/koto`), breaking shell integration version pinning.

## Decision Drivers

- CI-friendliness: `.tsuku.toml` must be self-contained -- no prior `tsuku install` or manual registry setup
- Backward compatibility: existing `.tsuku.toml` files with bare keys must continue working
- Minimal surface area: prefer reusing existing distributed-source machinery over new abstractions
- Consistency with CLI: `tsuku install tsukumogami/koto` works; the config should feel similar
- TOML ergonomics: quoted keys are a minor friction but widely precedented (mise, devcontainer.json)

## Decisions Already Made

During exploration, the following options were evaluated and eliminated:

- **Dotted keys** (`[tools.tsukumogami]` / `koto = "latest"`): parsing ambiguity between org scope and nested config table. Issue #2230 already flagged this as "misinterpreted as nested config."
- **Array of tables** (`[[tools]]` with name/version fields): completely breaking migration, excessive verbosity.
- **Value-side encoding** (`koto = "tsukumogami/koto@latest"`): stringly-typed, fragile, less self-documenting.
- **Explicit `[registries]` section in `.tsuku.toml`**: too verbose for the common case. The org prefix in the tool key already identifies the distributed source, making auto-registration sufficient.
- **Quoted-key approach chosen**: `"tsukumogami/koto" = "latest"` requires zero struct changes, full backward compatibility, and follows mise and devcontainer.json precedent.

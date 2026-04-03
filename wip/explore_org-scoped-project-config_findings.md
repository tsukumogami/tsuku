# Exploration Findings: org-scoped-project-config

## Core Question

How should `.tsuku.toml` represent org-scoped tools (like `tsukumogami/koto`) so the config is self-contained and works on fresh machines without prior `tsuku install` runs?

## Round 1

### Key Insights

- **Quoted keys are the clear syntax winner** (TOML syntax, CLI patterns): `"tsukumogami/koto" = "latest"` is valid TOML, fully backward compatible, zero struct changes to ProjectConfig, follows mise and devcontainer.json precedent. BurntSushi/toml handles it correctly.
- **The code gap is localized to `runProjectInstall`** (project config code, registry system): Project install passes tool names directly to `runInstallWithTelemetry` without distributed-name detection. The CLI path has full support via `parseDistributedName` -> `ensureDistributedSource`. Fix is to reuse this logic.
- **Distributed providers are lazy-loaded** (registry system): Even pre-registered registries don't create providers at startup. Project install must explicitly bootstrap providers for org-scoped tools.
- **Resolver has a secondary key mismatch** (project config code): Binary index stores bare recipe names (`koto`), but config key would be `tsukumogami/koto`. Shell integration breaks unless resolver strips org prefix or checks both forms.
- **Claude separates identity from resolution** (Claude plugin namespacing): `name@marketplace` pattern with marketplace manifests mapping to sources. For tsuku, the org prefix implicitly identifies the distributed source.
- **Self-contained config is critical for CI** (CLI patterns): Homebrew and mise show inline registry works. Auto-registration from the tool key is simpler than a `[registries]` section.

### Tensions

- Implicit auto-registration from org prefix (simple, CI-friendly) vs. explicit `[registries]` section (verbose but transparent). Research strongly favors implicit.
- Key-as-identity (`"tsukumogami/koto"`) vs. key-as-alias (`koto = { source = "tsukumogami/koto" }`). Research favors key-as-identity for simplicity and unambiguity.

### Gaps

None significant. Syntax options, code paths, and external precedents covered thoroughly.

### Decisions

- Eliminated dotted keys (`[tools.tsukumogami]`) and array-of-tables -- parsing ambiguity and migration cost.
- Eliminated value-side encoding (`koto = "tsukumogami/koto@latest"`) -- stringly-typed and less self-documenting.
- Eliminated explicit `[registries]` section in `.tsuku.toml` -- too verbose for the common case, auto-registration from org prefix is sufficient.

### User Focus

Auto mode. Findings converge on quoted-key approach with auto-registration. No further rounds needed.

## Accumulated Understanding

The fix has two layers. First, the TOML syntax: `"tsukumogami/koto" = "latest"` works today at the parsing level (BurntSushi/toml handles quoted keys with `/`). Second, the runtime: `runProjectInstall` must detect org-scoped keys (containing `/`), call `ensureDistributedSource` to register the provider, build the qualified name, and route through the distributed recipe path. The resolver also needs a fix: when matching binary-index recipe names against config keys, it must handle the org-prefix mismatch (strip prefix or check both `koto` and `tsukumogami/koto`). No new config sections or struct fields are required. The approach follows precedent from mise (quoted TOML keys with namespaces) and Claude Code (identity separate from resolution).

## Decision: Crystallize

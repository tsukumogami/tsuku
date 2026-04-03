# Exploration Decisions: org-scoped-project-config

## Round 1

- Eliminated dotted keys and array-of-tables: parsing ambiguity between org scope and nested config, plus breaking migration for array-of-tables.
- Eliminated value-side encoding: stringly-typed format is fragile and less self-documenting than key-based identity.
- Eliminated explicit `[registries]` section: too verbose for the common case; org prefix in tool key already identifies the source, making auto-registration sufficient.
- Chose quoted-key approach (`"tsukumogami/koto" = "latest"`): zero struct changes, full backward compatibility, proven by mise and devcontainer.json.
- Chose implicit auto-registration over explicit registry declaration: CI-friendliness requires self-contained config, and the org prefix provides enough info for source resolution.

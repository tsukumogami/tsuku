# DESIGN: Project Configuration

## Status

Proposed

## Upstream Design Reference

Parent: [DESIGN: Shell Integration Building Blocks](DESIGN-shell-integration-building-blocks.md)
Block 4 in the six-block architecture. This design specifies the project configuration file format and loading behavior that enables reproducible tool environments. Downstream consumers: #1681 (shell environment activation), #2168 (project-aware exec wrapper).

## Context and Problem Statement

Tsuku installs tools globally. When a developer clones a project that needs specific tools at specific versions, there's no way to declare those requirements in the repository. Each team member manually discovers and installs the right tools -- or gets bitten by version mismatches.

A project configuration file solves this by declaring tool requirements in a single, version-controlled location. Running `tsuku install` in that directory installs everything the project needs. Downstream building blocks (#1681, #2168) consume this configuration to activate the right versions automatically.

The design must specify:
- File naming and discovery (where tsuku looks for configuration)
- TOML schema (what the file contains)
- Version constraint syntax (how projects pin or constrain tool versions)
- The `ProjectConfig` Go interface consumed by downstream blocks
- CLI integration (`tsuku init`, `tsuku install` with no args)

## Decision Drivers

- **Discoverability**: New contributors should find and understand the config file without documentation
- **Simplicity**: The common case (pin a tool to a version) should be one line
- **Compatibility with tsuku conventions**: Use TOML (already used for recipes and user config), `$TSUKU_HOME` paths
- **Downstream interface stability**: `ProjectConfig` and `LoadProjectConfig` are consumed by #1681 and #2168; the interface must be complete and stable
- **Performance**: Loading and parsing must complete well within the 50ms shell integration budget
- **Monorepo support**: Must handle projects within larger repositories (parent directory traversal)
- **Ecosystem awareness**: Learn from mise, asdf, devbox, and volta, but don't cargo-cult their choices

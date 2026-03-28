# Design Summary: shell-env-activation

## Input Context (Phase 0)
**Source:** Freeform topic (issue #1681, Block 5 of shell integration building blocks)
**Problem:** Tsuku has no per-directory tool version activation. Projects declare tools in .tsuku.toml but there's no mechanism to automatically activate the right versions when entering a project directory.
**Constraints:** Must complete in <50ms per prompt, work on bash/zsh/fish, coexist with existing shellenv and activate commands, shell hooks must be optional.

## Current Status
**Phase:** 0 - Setup (Freeform)
**Last Updated:** 2026-03-27

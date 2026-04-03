# Design Summary: org-scoped-project-config

## Input Context (Phase 0)
**Source:** /explore handoff
**Problem:** Org-scoped recipes have no working syntax in .tsuku.toml. The project install path lacks distributed-name detection, and the resolver mismatches org-prefixed config keys with bare binary-index recipe names.
**Constraints:** Must be backward compatible, CI-friendly (self-contained config), and consistent with the existing `tsuku install org/tool` CLI syntax.

## Current Status
**Phase:** 0 - Setup (Explore Handoff)
**Last Updated:** 2026-04-03

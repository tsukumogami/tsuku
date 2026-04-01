# Design Summary: tool-lifecycle-hooks

## Input Context (Phase 0)
**Source:** /explore handoff
**Problem:** Tsuku has no lifecycle phases beyond install. Tools needing shell integration, completions, or cleanup get incomplete installations. 8-12 tools can't function without post-install setup.
**Constraints:** Preserve declarative trust model, keep shell startup under 5ms (cached), extend existing action system rather than redesign schema, store cleanup state for reliable removal.

## Current Status
**Phase:** 0 - Setup (Explore Handoff)
**Last Updated:** 2026-04-01

# Design Summary: distributed-recipes

## Input Context (Phase 0)
**Source PRD:** docs/prds/PRD-distributed-recipes.md
**Problem (implementation framing):** The Loader's hardcoded priority chain, source-unaware state tracking, and central-registry-coupled caching prevent clean addition of distributed recipe sources. A RecipeProvider abstraction, source-tracked state, and multi-origin caching are needed.

## Current Status
**Phase:** 0 - Setup (PRD)
**Last Updated:** 2026-03-15

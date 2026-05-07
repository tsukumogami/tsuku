# Design Summary: update-warnings-notifications

## Input Context (Phase 0)
**Source:** /explore handoff
**Problem:** The background auto-update path silently drops all warnings and non-fatal events because the subprocess routes to /dev/null. The design introduces an InboxReporter (same Reporter interface, inbox sink) to make the execution channel determine routing without changing call sites. Also covers success notices, version fallback warnings, and formalizing the Kind-based lifecycle taxonomy.
**Constraints:** One notice file per tool; backward compat with existing notice files (no Kind field); no call-site changes in install engine; version fallback belongs in Decompose, not the version provider.

## Current Status
**Phase:** 0 - Setup (Explore Handoff)
**Last Updated:** 2026-05-07

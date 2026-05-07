# Design Summary: update-warnings-notifications

## Input Context (Phase 0)
**Source:** /explore handoff
**Problem:** The background auto-update path silently drops all warnings and non-fatal events because the subprocess routes to /dev/null. The design introduces an InboxReporter (same Reporter interface, inbox sink) to make the execution channel determine routing without changing call sites. Also covers success notices, version fallback warnings, and formalizing the Kind-based lifecycle taxonomy.
**Constraints:** One notice file per tool; backward compat with existing notice files (no Kind field); no call-site changes in install engine; version fallback belongs in Decompose, not the version provider.

## Security Review (Phase 5)
**Outcome:** Option 2 - Document considerations
**Summary:** No new privilege escalation or supply-chain risk. Two gaps flagged: tool-name path validation in WriteNotice (Medium, one-line fix), and ANSI sanitization in InboxReporter before message accumulation (Low-Medium). Both addressed in Security Considerations section.

## Current Status
**Phase:** 5 - Security (complete) → entering Phase 6
**Last Updated:** 2026-05-07

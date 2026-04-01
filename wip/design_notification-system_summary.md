# Design Summary: notification-system

## Input Context (Phase 0)
**Source:** Freeform topic (issue #2185)
**Problem:** Tsuku's auto-update system lacks a notification layer with suppression logic, available-update messages, and a shared framework for formatting different notification types.
**Constraints:** Must integrate with existing notices package, quietFlag, and PersistentPreRun lifecycle. Must not break CI pipelines. Self-update PR #2199 changes must be accounted for.

## Current Status
**Phase:** 0 - Setup (Freeform)
**Last Updated:** 2026-04-01

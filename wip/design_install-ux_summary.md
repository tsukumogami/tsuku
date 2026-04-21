# Design Summary: install-ux

## Input Context (Phase 0)
**Source:** /explore handoff
**Problem:** tsuku install/update emit 20–50+ lines per install via raw fmt.Printf(), with
no TTY awareness, no in-place updates, and a separate download progress widget that doesn't
coordinate with step output. Need to replace with a Reporter-based architecture (modeled on
niwa) that animates in-place on TTY, degrades gracefully in non-TTY, and unifies all output
through a single channel.
**Constraints:** Must wire via ExecutionContext (zero action signature changes); niwa Reporter
pattern is the adopted reference; no separate progress bar widget; step names eliminated from
happy-path output; ~384 fmt.Printf call sites must migrate.

## Current Status
**Phase:** 0 - Setup (Explore Handoff)
**Last Updated:** 2026-04-20

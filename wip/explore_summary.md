# Exploration Summary: Discovery Telemetry Events

## Problem (Phase 1)
The discovery resolver runs through up to three stages (registry lookup, ecosystem probe, LLM discovery) but produces no telemetry. There's no way to know how often each stage fires, what tools users search for, or where the resolver fails. This makes it impossible to prioritize registry curation, identify ecosystem probe reliability issues, or measure LLM discovery effectiveness.

## Decision Drivers (Phase 1)
- Follow existing telemetry patterns (separate event struct, `Send*` method, `discovery_*` action prefix)
- Don't leak sensitive data (no API responses, no full URLs)
- Keep events lightweight (fire-and-forget, no blocking)
- Backend must dispatch discovery events to their own blob layout in Analytics Engine
- Dashboard needs queryable stats for discovery usage

## Research Findings (Phase 2)
- Three existing event categories: standard (13 blobs), LLM (16 blobs), discovery (new)
- Backend dispatches by action prefix: `llm_*` -> LLM handler, others -> standard handler
- `DiscoveryResult` struct has Builder, Source, Confidence, Reason, Metadata fields
- Chain resolver in `chain.go` iterates stages and returns first non-nil result
- Ecosystem probe runs parallel queries to 7 ecosystems with quality filtering
- `tryDiscoveryFallback()` in install.go is the main entry point from user commands

## Options (Phase 3)
1. Separate DiscoveryEvent struct with SendDiscovery method (follows LLM pattern)
2. Reuse existing Event struct with discovery_* action names
3. Generic event struct covering all categories

## Decision (Phase 5)

**Problem:**
The discovery resolver has no telemetry instrumentation. Without data on which stages fire, how often tools are found, and where resolution fails, we can't prioritize registry curation or measure ecosystem probe reliability.

**Decision:**
Add a `DiscoveryEvent` struct following the existing LLM event pattern: separate struct, `SendDiscovery()` method on the client, `discovery_*` action prefix for backend dispatch, and a dedicated blob layout in Analytics Engine. Instrument the chain resolver to emit events at each stage boundary. Add a `/stats/discovery` endpoint and dashboard section.

**Rationale:**
The separate-struct approach matches the established pattern (Event for installs, LLMEvent for LLM ops) and keeps each category's fields clean. Reusing the Event struct would require overloading fields or adding nullable discovery-specific fields to an unrelated struct. The action prefix dispatch pattern already works and scales naturally to a third category.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-02-02

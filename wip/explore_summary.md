# Exploration Summary: Pipeline Dashboard Overhaul

## Problem (Phase 1)

The pipeline dashboard and its supporting backend have accumulated structural
issues that degrade operator experience. Four of six ecosystem circuit breakers
are stuck open due to a probe selection deadlock, the main dashboard mixes
ecosystem-specific and pipeline-level health data in a single panel, and 37+
individual bugs across 12 HTML pages make cross-navigation unreliable.

## Decision Drivers (Phase 1)

- Circuit breaker recovery must not require manual intervention
- Dashboard widgets should each have a single clear purpose
- Per-ecosystem visibility is needed for triaging pipeline stalls
- Existing bug fixes (#1834-#1838) should be addressed in the same overhaul
- Changes to dashboard.json schema must preserve backward compatibility
- Automated testing should prevent regression of fixed bugs (#1839)

## Research Findings (Phase 2)

- Circuit breaker deadlock root cause: `selectCandidates()` applies backoff to half-open probes and doesn't prefer pending entries
- Dashboard index.html has 9 panels, Pipeline Health combines 3 concerns
- dashboard.json schema has no per-ecosystem queue breakdown (only by_status and by_tier)
- All timestamps use browser locale via `toLocaleDateString()` with no explicit timezone
- 12 HTML pages, 37 bugs cataloged across issues #1834-#1838

## Options (Phase 3)

- D1 Circuit breaker: pending-first probe with backoff bypass vs reset backoff vs synthetic probe entries
- D2 Widget layout: three-widget split vs tabs vs stacked bars
- D3 Readability: ET hardcoded vs browser locale vs UTC
- D4 Bug scope: include all vs separate PRs
- D5 Testing: Go tests + shell checks vs Playwright vs JSON Schema

## Decision (Phase 5)

**Problem:**
Four of six ecosystem circuit breakers are permanently stuck open because half-open probes retry failed entries instead of trying pending ones, and backoff windows prevent probe selection entirely. The dashboard mixes ecosystem-specific and pipeline-level health in one panel, timestamps lack timezone context, and 37 bugs across 12 pages degrade navigation.

**Decision:**
Fix probe selection to prefer pending entries and bypass backoff for half-open probes. Split Pipeline Health into three widgets: Pipeline Health (pipeline-level), Ecosystem Health (circuit breaker details), and Ecosystem Pipeline (per-ecosystem counts via new `by_ecosystem` field). Hardcode `America/New_York` for timestamp display. Include all 37 bugs from #1834-#1838 and add Go + shell validation tests (#1839).

**Rationale:**
Pending-first probes are the simplest fix that ensures recovery when viable entries exist. Three widgets match the three questions operators ask (is the pipeline running? which ecosystems are healthy? where is work concentrated?). Including bug fixes avoids double-touching the same files. Go tests for data contracts and shell checks for link integrity catch the bug categories found in the QA audit without introducing browser testing dependencies.

## Current Status
**Phase:** 7 - Security (complete)
**Last Updated:** 2026-02-22

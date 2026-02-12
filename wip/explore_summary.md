# Exploration Summary: Disambiguation

## Problem (Phase 1)
When multiple ecosystem registries return matches for a tool name, the current system silently picks one without user awareness, creating security risk (wrong package) and usability problems (no way to select alternatives).

## Decision Drivers (Phase 1)
- Security-sensitive operation (wrong package = untrusted code)
- Reuse existing patterns (LLM discovery confirmation UX)
- Batch pipeline compatibility (deterministic selection)
- CLI usability (clear feedback)
- Typosquatting defense (detect similar names)
- Ecosystem data limitations (not all registries have downloads)

## Research Findings (Phase 2)
- **Prior art**: Homebrew uses flags, npm prevents at registry, pip has no disambiguation
- **Edit distance**: Levenshtein ≤2 catches 45% of typosquats
- **Popularity data**: npm, crates.io, rubygems have it; PyPI, Go, Homebrew don't
- **Existing patterns**: LLM discovery has ranking, confirmation, fork detection

## Options (Phase 3)
1. **Disambiguation algorithm**: Popularity-based auto-select (>10x gap) with interactive fallback (chosen)
2. **Typosquatting detection**: Edit distance ≤2 against registry entries (chosen)
3. **Interactive prompt**: Ranked list with key metadata (chosen)
4. **Batch integration**: Deterministic selection with tracking (chosen)
5. **Code location**: Integrate into ecosystem_probe.go (chosen)

## Current Status
**Phase:** 3 - Options complete, ready for review
**Last Updated:** 2026-02-11

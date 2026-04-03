# Guides Catalog

11 guides, ~3,284 lines total. Assessed for bundle vs pointer vs skip for recipe skills.

## Bundle (reshape into skill references)

| Guide | Lines | Topics | Why Bundle |
|-------|-------|--------|------------|
| GUIDE-actions-and-primitives.md | 615 | Action types, determinism, decomposition, ecosystem primitives, composites | Core reference for all recipe authoring. Tables are agent-friendly. |
| GUIDE-system-dependencies.md | 275 | System actions, platform detection, --target-family, Docker example | Essential for recipes with system deps. Clear action tables. |
| GUIDE-library-dependencies.md | 282 | Auto-provisioning, build integration, dependency resolution | Recipe authors need dependency declaration patterns. |
| GUIDE-hybrid-libc-recipes.md | 439 | glibc/musl matrix, libc filter, migration templates A/B/C, Alpine testing | Critical for modern recipes. Migration templates are ready-to-use. |
| GUIDE-recipe-verification.md | 267 | Version/output modes, format transforms, decision flowchart | Every recipe needs verification. Decision flowchart is essential. |
| GUIDE-distributed-recipe-authoring.md | 154 | .tsuku-recipes/ setup, naming, caching, testing | Essential for distributed recipe authors. |

## Pointer (link from skill)

| Guide | Lines | Topics | Why Pointer |
|-------|-------|--------|-------------|
| GUIDE-plan-based-installation.md | 498 | Two-phase install, air-gapped, plan format, determinism | Context on how recipes execute, not needed during authoring. |
| GUIDE-distributed-recipes.md | 158 | Install syntax, trust model, registry management | User-facing perspective, complements authoring guide. |
| GUIDE-troubleshooting-verification.md | 229 | Verification tiers 1-4, diagnostic procedures | Troubleshooting reference, not primary authoring. |

## Skip (not recipe-relevant)

| Guide | Lines | Topics | Why Skip |
|-------|-------|--------|----------|
| GUIDE-command-not-found.md | 145 | Shell hook setup, managing hooks | Post-installation user docs. |
| GUIDE-local-llm.md | 222 | Local inference, GPU, config | Recipe generation tool, not recipe authoring. |

## Recommendation for Skill References

Create these bundled reference files (reshaped, not duplicated):
1. **action-reference.md** -- from GUIDE-actions-and-primitives + GUIDE-system-dependencies (action lookup tables, key params)
2. **platform-reference.md** -- from GUIDE-hybrid-libc-recipes (when clause patterns, libc decision tree, migration templates)
3. **verification-reference.md** -- from GUIDE-recipe-verification (mode selection flowchart, format transforms)
4. **dependencies-reference.md** -- from GUIDE-library-dependencies (dependency declaration, build env, auto-provisioning)
5. **distributed-reference.md** -- from GUIDE-distributed-recipe-authoring (setup, naming, testing)
6. **exemplar-recipes.md** -- curated recipe paths by category (already in PRD)

## Other Recipe-Relevant Docs

- **recipes/CLAUDE.local.md** (47 lines): Lightweight reference card, actions table. Needs expansion.
- **CONTRIBUTING.md**: Developer-focused, covers testing patterns but not recipe authoring specifically.

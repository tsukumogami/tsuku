# Documentation Plan: registry-scale-strategy

Generated from: docs/designs/DESIGN-registry-scale-strategy.md
Issues analyzed: 1
Total entries: 2

---

## doc-1: docs/designs/DESIGN-registry-scale-strategy.md
**Section**: (multiple sections)
**Prerequisite issues**: #1278
**Update type**: modify
**Status**: pending
**Details**: After #1278 ships, update Implementation Issues table (strike through #1278 row and mark done), dependency graph (change I1278 class from ready to done), Phase 2 status paragraph (M53 fully closed), milestone list (M53 closed), Remaining Work open issues count (1 issue down from 2), and Open Issues table (remove #1278 row).

---

## doc-2: docs/designs/current/DESIGN-priority-queue.md
**Section**: Trade-offs Accepted
**Prerequisite issues**: #1278
**Update type**: modify
**Status**: pending
**Details**: The "Trade-offs Accepted" section lists "Coarse ordering: Tiered scoring (2C) doesn't distinguish within tiers" as a known limitation. After #1278 ships, add a note that within-level sub-ordering by transitive blocking impact was added to address this gap, referencing the queue-analytics tool or new CLI tool that implements it.

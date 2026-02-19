# Documentation Plan: llm-testing-strategy

Generated from: docs/designs/DESIGN-llm-testing-strategy.md
Issues analyzed: 7
Total entries: 1

---

## doc-1: docs/llm-testing.md
**Section**: (new file)
**Prerequisite issues**: #1759
**Update type**: new
**Status**: updated
**Details**: Write the manual test runbook described in the design. Three procedures: (1) full 10-case benchmark with fresh server restarts between cases and a results table, (2) soak test running 20+ sequential requests through a warm server while monitoring memory growth via `/proc/<pid>/status` (VmRSS), and (3) new model validation workflow for evaluating a model change against current baselines. Include the `-update-baseline` flag usage for updating `testdata/llm-quality-baselines/` after validation. Reference `TSUKU_LLM_BINARY` env var for local provider selection. Include result recording templates so outputs are comparable across runs.

---

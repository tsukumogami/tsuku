# Scrutiny Review: Completeness - Issue #1645

**Issue**: #1645 - docs(llm): update documentation for local inference
**Focus**: completeness
**Reviewer**: pragmatic-reviewer (scrutiny agent)

## Files Changed

- `README.md` -- Added local-first framing, Local LLM Management section, link to guide
- `docs/ENVIRONMENT.md` -- Added `TSUKU_LLM_IDLE_TIMEOUT` entry and summary table row
- `docs/GUIDE-local-llm.md` -- New file: full user guide (how it works, hardware table, config reference, env var override, pre-download, first-use, troubleshooting)

## AC Evaluation

### AC 1: "config reference" -- Claimed: implemented

**Verified.** `docs/GUIDE-local-llm.md` lines 34-74 contain a "Configuration" section with an "Options Reference" subsection. All five config keys are documented: `local_enabled`, `local_preemptive`, `local_model`, `local_backend`, `idle_timeout`. Each has a comment explaining behavior and default. `tsuku config set`/`get` examples included.

The config reference matches the design doc's "Configuration Options" section (lines 198-228 of the design doc). All options from the design doc are present in the guide.

**Severity**: No finding.

### AC 2: "env var override" -- Claimed: implemented

**Verified.** Two locations:
1. `docs/GUIDE-local-llm.md` lines 76-83: "Environment Variable Override" section documenting `TSUKU_LLM_IDLE_TIMEOUT` with format, example.
2. `docs/ENVIRONMENT.md` lines 129-141: Full entry under "Local LLM Runtime" heading with default, format, example, and explanation of override behavior. Also added to summary table at line 233.

**Severity**: No finding.

### AC 3: "hardware requirements" -- Claimed: implemented

**Verified.** `docs/GUIDE-local-llm.md` lines 18-33: "Hardware Requirements" section with a table matching the design doc's model selection table (lines 293-298). Five tiers: 8GB+ VRAM, 4-8GB VRAM, CPU 8GB+ RAM, CPU <8GB RAM, <4GB disabled. Model names, download sizes, and quality expectations included. Notes on GPU vs CPU speed and minimum RAM floor.

**Severity**: No finding.

### AC 4: "troubleshooting" -- Claimed: implemented

**Verified.** `docs/GUIDE-local-llm.md` lines 146-210: "Troubleshooting" section with four subsections:
- "Download was declined or failed" -- recovery command
- "'no LLM providers available' error" -- config check and hardware floor
- "Addon server won't start" -- stale socket cleanup
- "Slow inference on CPU" -- expected behavior, mitigations

Also includes "Falling Back to Cloud Providers" subsection with API key setup and config to disable local inference.

**Severity**: No finding.

### AC 5: "documentation location" -- Claimed: implemented

**Verified.** Three documentation locations updated:
1. `docs/GUIDE-local-llm.md` -- standalone guide (new file)
2. `docs/ENVIRONMENT.md` -- env var reference (updated)
3. `README.md` -- entry points with links to the guide (updated)

The README links to the guide at lines 128 and 228. The guide is a standalone document that doesn't require reading the design doc.

**Severity**: No finding.

## Missing ACs Check

The issue description from the design doc says: "Document config options, hardware requirements table, and troubleshooting guide for local LLM runtime."

The five mapping entries cover: config reference, env var override, hardware requirements, troubleshooting, documentation location. These align with the issue description. No ACs from the issue body appear to be missing from the mapping.

## Phantom ACs Check

All five mapping entries correspond to aspects described in the issue. No phantom ACs detected.

## Summary

All five ACs are verified against the actual file contents. The implementation is thorough: config options match the design doc, the hardware table reflects the model selection strategy, env var override is documented in both the guide and the environment reference, troubleshooting covers the common failure modes described in the design doc's error handling section, and documentation is placed in three appropriate locations with cross-linking.

No blocking or advisory findings.

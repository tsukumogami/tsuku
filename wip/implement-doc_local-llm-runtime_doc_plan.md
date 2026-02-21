# Documentation Plan: local-llm-runtime

Generated from: docs/designs/DESIGN-local-llm-runtime.md
Issues analyzed: 5
Total entries: 4

---

## doc-1: README.md
**Section**: LLM-Powered Recipe Generation
**Prerequisite issues**: #1642, #1643, #1645
**Update type**: modify
**Status**: pending
**Details**: Update the "LLM-Powered Recipe Generation" section to document that local inference works out of the box without API keys. Currently the section says LLM builders "require an API key" with no mention of local fallback. After these issues land: (1) explain that local inference is the default when no API keys are configured, (2) add the `tsuku llm download` command to pre-download the addon and model, (3) mention the first-use download prompt and approximate sizes (addon ~50MB, model 0.5-2.5GB), (4) note that cloud providers remain available for users who want them. Keep existing API key setup instructions but reframe them as optional for users who prefer cloud providers.

---

## doc-2: README.md
**Section**: Commands table (Usage section) or new "Local Inference" subsection
**Prerequisite issues**: #1643
**Update type**: modify
**Status**: pending
**Details**: Add `tsuku llm download` to the command listing or usage examples. The command pre-downloads the addon binary and model files for CI/offline use. Hardware detection selects the appropriate model. Include a brief usage example showing the command and what it does.

---

## doc-3: docs/ENVIRONMENT.md
**Section**: Environment Variables
**Prerequisite issues**: #1645
**Update type**: modify
**Status**: pending
**Details**: Add documentation for `TSUKU_LLM_IDLE_TIMEOUT` environment variable (overrides config idle timeout, useful for testing and CI). This variable is already implemented in code but not documented in ENVIRONMENT.md. Include default value (5m), format (Go duration string), and example usage.

---

## doc-4: docs/GUIDE-local-llm.md
**Section**: (new file)
**Prerequisite issues**: #1642, #1643, #1645
**Update type**: new
**Status**: pending
**Details**: New guide covering local LLM runtime usage, as specified by #1645. Contents: (1) Overview of how local inference works (addon architecture, on-demand server, idle timeout), (2) Hardware requirements table mapping resources to model selection (8GB+ VRAM -> 3B, 4-8GB VRAM -> 1.5B, CPU 8GB+ RAM -> 1.5B, CPU <8GB -> 0.5B, <4GB RAM -> disabled), (3) Configuration options reference (local_enabled, local_preemptive, local_model, local_backend, idle_timeout in config.toml), (4) Pre-downloading for CI/offline use via `tsuku llm download`, (5) First-use experience (download prompts, progress indicators, what to expect), (6) Troubleshooting section covering common issues (download failures, insufficient hardware, server startup problems, how to fall back to cloud providers).

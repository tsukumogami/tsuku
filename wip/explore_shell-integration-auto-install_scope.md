# Explore Scope: shell-integration-auto-install

## Core Question

Does the current 5-block shell integration design adequately deliver the use case
where a project declares its tools in `tsuku.toml`, and those tools are automatically
installed on first use -- across both interactive shells and non-interactive contexts
like scripts and CI?

## Context

The parent design (DESIGN-shell-integration-building-blocks.md) proposes 5 blocks
across 2 parallel tracks. Track B (project config + shell activation) addresses
project-declared tools, but its activation mechanism relies on shell prompt hooks
that aren't present in scripts or CI. Track A (binary index + command-not-found +
auto-install) handles unknown commands but doesn't consult project config for
version pinning.

The design acknowledges these tracks converge but defers the integration to Block 3's
detailed design with a single sentence: "Block 3's detailed design should accept an
optional ProjectConfig parameter." Issue #1677 (binary index design) is the next
concrete step, but the convergence architecture is unspecified.

Key user constraint: developers use koto + shirabe in a mix of interactive shell and
non-interactive contexts (scripts, CI). Shell hooks cannot be assumed to be present.
The "on first use" install must work without prompt hooks.

## In Scope

- Trigger model analysis: which mechanism delivers "on first use" without shell hooks
- Whether the binary index (issue #1677) is actually needed for project-declared tools
- How comparable tools (mise, devbox, direnv) handle project-scoped auto-install
- Whether the 5-block decomposition is complete or needs a convergence block
- The koto/shirabe integration model (does koto call tsuku, or rely on PATH only)
- tsuku.toml scope: tool allowlist + version pinning as the starting feature set

## Out of Scope

- LLM recipe discovery (Block 6, deferred in parent design)
- Binary index internals beyond what's needed for the convergence question
- Windows support
- Telemetry
- Future tsuku.toml uses beyond tool allowlist + version pinning

## Research Leads

1. **What trigger models exist for "on first use" without shell hooks?**
   Command-not-found is shell-hook-dependent. Shim-based interception (asdf) works
   without hooks but adds overhead. `tsuku exec` wrapper works anywhere. What are the
   realistic options, and which supports the non-interactive requirement?

2. **Does the binary index serve the tsuku.toml use case?**
   If a tool is declared in tsuku.toml, the recipe is already known -- no reverse
   lookup needed. The binary index solves "unknown command → find recipe." Are these
   separate lookup paths? Does this change what issue #1677 must specify?

3. **How do mise, devbox, and asdf handle project-declared tool auto-install in CI?**
   Looking specifically for "on first use" or "on invocation" patterns that work
   without shell hook setup. What tradeoffs exist?

4. **Is there a missing 6th block -- project-aware command dispatch -- that the
   current design doesn't have?**
   The design's convergence point is hand-wavy. Could a block that combines tsuku.toml
   reading + lazy install + execution be the right primitive, sitting between Block 3
   and Block 4?

5. **What's the koto/shirabe integration model?**
   Does koto call `tsuku install` when it detects missing tools? Does it rely on PATH?
   Does it use `tsuku run` as a wrapper? The integration point determines whether
   tsuku needs to be invoked at command execution time or only at shell setup time.

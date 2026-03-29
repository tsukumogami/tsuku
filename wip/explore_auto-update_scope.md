# Explore Scope: auto-update

## Visibility

Public

## Core Question

How should tsuku implement auto-update capabilities for both its own binary and the tools it manages? The system needs version channel pinning (major/minor/patch), time-cached update checks with force-on-command, automatic rollback on failure with deferred error reporting, and configurable notifications — all with similar user-facing behavior but potentially different implementation paths for self-update vs. tool update.

## Context

The user wants a comprehensive auto-update system inspired by Claude Code's channel model. Tsuku already has version providers that resolve latest versions from GitHub, PyPI, crates.io, etc. The existing `tsuku update <tool>` and `tsuku outdated` commands provide manual update workflows, but there's no automatic checking or updating. The user expects the update behavior to feel consistent whether updating tsuku itself or a managed tool, even though the mechanics differ (replacing the running binary vs. updating a tool in $TSUKU_HOME/tools). Rollback on auto-update failure is required — the previous working version should be preserved and failures reported on next tool execution. Update check frequency should be time-based with caching to avoid network calls on every run, plus a force flag for on-demand checks.

## In Scope

- Self-update mechanism for the tsuku binary
- Auto-update for managed tools
- Version channel pinning (major, minor, patch granularity)
- Stable release channel concept
- Time-cached update checks with configurable intervals
- Force-check on command
- Automatic rollback on update failure
- Deferred failure reporting (report on next tool execution)
- Configurable update notifications (including suppression)
- Listing tools with available upgrades
- Adjacent stories discovered during research (pre-release channels, org policy, etc.)

## Out of Scope

- To be determined during research — the user explicitly wants investigation to find scope boundaries

## Research Leads

1. **How do other CLI tools and package managers handle self-update?**
   Binary replacement is inherently tricky since the running process owns the file. Need to survey approaches: swap-on-restart, exec into new version, sidecar updater process, download-alongside-and-rename.

2. **What version channel/pinning models exist in the wild?**
   Claude Code, Homebrew, nvm, rustup, asdf, and others have variants of channel/pinning. Need to understand the trade-offs between simplicity and flexibility, and what model fits tsuku's "install tools without sudo" philosophy.

3. **How should cached update checks work (staleness, storage, invalidation)?**
   Time-interval caching with force-on-command is the target. Need to nail down storage format (file-based, in state.json), invalidation rules, and what happens when the cache is corrupt or missing.

4. **What are the failure modes of auto-update and how do tools handle rollback?**
   Partial downloads, corrupt binaries, incompatible recipe format changes, network failures mid-update. Need to survey rollback strategies and how other tools handle deferred error reporting.

5. **How does tsuku's existing version provider system relate to update checking?**
   Tsuku already resolves versions from GitHub releases, PyPI, crates.io, npm, RubyGems, etc. How much of this infrastructure can be reused for update checking vs. what needs to be extended or built new?

6. **What UX patterns exist for update notifications that balance awareness vs. noise?**
   Configurable notification levels, quiet mode, structured output for scripting environments, notification of out-of-channel updates. How do other tools let users control what they see?

7. **What adjacent stories should be in scope (pre-release channels, org policy, update-on-install)?**
   The user flagged "additional stories to cover." Need to survey what mature package managers support beyond basic auto-update: beta/nightly channels, organizational version locks, update-only-on-install mode, telemetry for update success rates.

8. **Is there evidence of real demand for this, and what do users do today instead?** (lead-adversarial-demand)
   You are a demand-validation researcher. Investigate whether evidence supports
   pursuing this topic. Report what you found. Cite only what you found in durable
   artifacts. The verdict belongs to convergence and the user.

   ## Visibility

   Public

   Respect this visibility level. Do not include private-repo content in output
   that will appear in public-repo artifacts.

   ## Six Demand-Validation Questions

   Investigate each question. For each, report what you found and assign a
   confidence level.

   Confidence vocabulary:
   - **High**: multiple independent sources confirm (distinct issue reporters,
     maintainer-assigned labels, linked merged PRs, explicit acceptance criteria
     authored by maintainers)
   - **Medium**: one source type confirms without corroboration
   - **Low**: evidence exists but is weak (single comment, proposed solution
     cited as the problem)
   - **Absent**: searched relevant sources; found nothing

   Questions:
   1. Is demand real? Look for distinct issue reporters, explicit requests,
      maintainer acknowledgment.
   2. What do people do today instead? Look for workarounds in issues, docs,
      or code comments.
   3. Who specifically asked? Cite issue numbers, comment authors, PR
      references — not paraphrases.
   4. What behavior change counts as success? Look for acceptance criteria,
      stated outcomes, measurable goals in issues or linked docs.
   5. Is it already built? Search the codebase and existing docs for prior
      implementations or partial work.
   6. Is it already planned? Check open issues, linked design docs, roadmap
      items, or project board entries.

   ## Calibration

   Produce a Calibration section that explicitly distinguishes:

   - **Demand not validated**: majority of questions returned absent or low
     confidence, with no positive rejection evidence. Flag the gap. Another
     round or user clarification may surface what the repo couldn't.
   - **Demand validated as absent**: positive evidence that demand doesn't exist
     or was evaluated and rejected. Examples: closed PRs with explicit maintainer
     rejection reasoning, design docs that de-scoped the feature, maintainer
     comments declining the request. This finding warrants a "don't pursue"
     crystallize outcome.

   Do not conflate these two states. "I found no evidence" is not the same as
   "I found evidence it was rejected."

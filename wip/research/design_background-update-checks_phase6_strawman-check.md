# Strawman Check: DESIGN-background-update-checks.md

## Decision 1: Cache schema and staleness model

**Directory mtime for global staleness** -- Not a strawman. Proposes a real simplification (no sentinel file, one fewer artifact). Rejection cites a genuine POSIX portability issue: `rename(2)` within a directory doesn't reliably update dir mtime across filesystems. Specific and verifiable. Pass.

**Metadata JSON file** -- Not a strawman. A `.meta.json` with structured data is a reasonable approach (Homebrew does something similar). Rejection cites a concrete cost (~0.5-1ms parse overhead against a 5ms budget). The budget constraint is established in Decision Drivers. Pass.

**In-progress sentinel** -- Borderline. The proposal (write "in-progress" marker, update to "complete") is a real pattern, but the rejection points out it's redundant with the already-decided flock mechanism. This is less "strawman" and more "already superseded by a prior constraint." The "Decisions Already Made" section locks in flock before this alternative is evaluated, so the rejection is mechanically correct but the alternative never had a chance. **Advisory**: not a strawman per se, but the deck was stacked by a prior constraint. A reader unfamiliar with the exploration phase might wonder why flock was chosen over this approach, since the flock decision isn't justified here -- it's presented as settled.

## Decision 2: Trigger integration and spawn protocol

**UpdateTrigger struct** -- Not a strawman. A struct with methods is a natural Go instinct. Rejection: `ShouldCheck()` alone has no caller -- checking without spawning is meaningless. Concrete single-caller-abstraction argument. Pass.

**Split functions (bool + spawn)** -- Not a strawman. This is the most obvious decomposition. Rejection: callers must always pair both calls, and forgetting one silently drops updates. This is a real API-misuse risk. Pass.

**PersistentPreRun only** -- Not a strawman. Single call site is genuinely simpler. Rejection cites a real problem: the background process would trigger itself (recursive spawn). Also flags `help`/`version` as unnecessary triggers. However, the rejection says "Exclusion lists are worse than three explicit call sites" -- this is debatable. The chosen solution *also* uses an exclusion list in PersistentPreRun (the skip list). **Issue**: The rejection of PersistentPreRun-only claims exclusion lists are bad, but the chosen approach uses one too. The real difference is that hook-env and cmd_run can't use PersistentPreRun (hook-env isn't a cobra command). The rejection should say that, not argue against exclusion lists it also uses.

## Decision 3: Configuration surface

**Full resolver struct** -- Not a strawman. A dedicated resolution function taking all precedence layers is a clean-room approach. Rejection: no codebase precedent, creates inconsistency with LLMConfig/telemetry patterns. Concrete and verifiable by reading the codebase. Pass.

**Separate middleware layer** -- Weak but not a strawman. An `updateconfig` package is a real option for separation of concerns. Rejection: splits one concern across two packages, inconsistent with LLMConfig embedding env checks in getters. Fair, though "no practical gain" is slightly hand-wavy -- the gain would be testability of env var resolution in isolation. **Advisory**: rejection could be sharper about why that testability isn't needed.

**Non-pointer fields with sentinel values** -- Not a strawman. This is the naive Go approach. Rejection cites Go's zero-value semantics making `false` ambiguous for a default-true field. This is a well-known Go gotcha and the explanation is precise. Pass.

## Summary

No strawmen detected. Two advisory findings:

1. **D2, "PersistentPreRun only"**: Rejection argues against exclusion lists, but the chosen approach also uses one. The actual reason this alternative fails (hook-env isn't a cobra command) goes unstated.
2. **D1, "In-progress sentinel"**: Alternative is rejected by pointing to a prior decision (flock) that itself isn't justified in this document. Readers lack the context to evaluate whether flock was the right choice over this approach.

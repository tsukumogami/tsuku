# Design Review: CLI Deterministic-Only Mode

## 1. Problem Statement Specificity

**Assessment: Well-defined and specific**

The problem statement is concrete and measurable:
- Clear scope: CLI lacks flag to set `DeterministicOnly` session option
- Quantified impact: Affects 32% of Homebrew packages (8 of 25)
- Specific failure mode: Exit code 1 with "no LLM providers available" instead of structured failure
- Clear stakeholder: Batch orchestrator needs deterministic-only mode without API keys

The in-scope/out-of-scope boundaries are well-defined. Solutions can be evaluated against whether they:
1. Enable CLI to suppress LLM fallback
2. Provide structured failure data to subprocess callers
3. Preserve `DeterministicFailedError` information
4. Work without API keys

**Minor gap:** The problem statement could be more explicit about the user personas. Right now it focuses on the batch orchestrator (automation), but doesn't explicitly state whether interactive CLI users ever need deterministic-only mode. This affects Option B's viability.

## 2. Missing Alternatives

**Three alternatives worth considering:**

### A. Environment Variable Instead of Flag

Set `TSUKU_DETERMINISTIC_ONLY=1` environment variable. CLI reads it and sets session options accordingly.

**Why consider it:**
- Environment variables are common for automation-specific settings
- Doesn't pollute `--help` output with flags most users won't use
- Easy for batch orchestrator to set (already sets `TSUKU_HOME`)
- Can be combined with exit code approach

**Trade-offs:**
- Less discoverable than `--help` flag
- Requires documentation in a different location
- Environment variables can be harder to debug (invisible in process listings)

### B. Structured JSON Output Mode

Add `--output=json` flag that makes CLI emit structured JSON for all outcomes (success, deterministic failure, LLM fallback, etc.). The orchestrator parses JSON instead of using exit codes.

**Why consider it:**
- Issue #1273 already tracks structured JSON output
- Solves the broader problem of CLI-to-automation communication
- Provides richer data than exit codes alone
- Future-proof for other failure categories

**Trade-offs:**
- Larger scope than stated (though in-scope says "out of scope: Structured JSON output for other commands", implying it's acceptable for this command)
- Requires JSON parsing in orchestrator (more complex than exit code checking)
- Mixing concerns: output format + deterministic-only mode

### C. Dedicated `tsuku create-batch` Subcommand

Create a separate subcommand for batch usage that defaults to deterministic-only and structured output.

**Why consider it:**
- Separates automation API from interactive API
- Can evolve batch-specific features without affecting `tsuku create`
- Clear signal of intent (batch vs interactive use)

**Trade-offs:**
- Code duplication or shared implementation complexity
- More commands to maintain
- Arguably over-engineered for a single flag

**Recommendation:** Option A (environment variable) deserves consideration as a variant of the proposed flag approach. The others are likely out of scope but should be acknowledged as alternatives.

## 3. Pros/Cons Fairness and Completeness

### Option A: Add `--deterministic-only` CLI Flag

**Additional pros:**
- Testable: Can write integration tests that verify flag behavior
- Composable: Works with future `--output=json` flag (orthogonal concerns)
- Precedent: Many CLI tools have automation-specific flags (e.g., `--quiet`, `--no-interactive`)

**Additional cons:**
- Requires updating two layers: CLI flag parsing + builder invocation
- Flag name might be unclear to users unfamiliar with the codebase ("deterministic" is domain-specific jargon)
- Need to decide behavior when flag is set but API keys ARE available (should it still suppress LLM? Probably yes, but worth stating)

**Missing trade-off:** What happens if someone uses the flag interactively? The design says "Interactive users shouldn't see behavior change," but if they explicitly pass `--deterministic-only`, they WILL see a change. This isn't bad, but it contradicts that statement.

### Option B: Auto-detect No-LLM Environment

**Additional pros:**
- Matches user intent: If you can't use LLM, you probably want deterministic-only behavior
- Could help interactive users who misconfigured API keys (fail fast instead of "no providers" error)

**Additional cons:**
- **Critical flaw:** If a user forgets to set API keys but WANTS LLM fallback, they get silent behavior change with no way to override it
- Violates principle of least surprise: presence/absence of API keys shouldn't change CLI semantics
- Hard to document: "The command behaves differently depending on environment variables you might not know about"
- Makes testing harder (need to control API key presence)

**Missing analysis:** The design says this "couples LLM key presence to deterministic-only mode, which are conceptually separate." This is correct, but the con understates the issue. This isn't just coupling—it's using an implementation detail (API key availability) as a proxy for user intent (wanting deterministic-only mode). These can diverge in both directions:
- User has no keys but wants to know LLM would be needed (for planning purposes)
- User has keys but wants deterministic-only output (for reproducibility testing)

### Option C: Orchestrator Parses CLI Output

**Additional pros:**
- Zero risk to CLI users (no behavior change)
- Could be implemented as a temporary workaround while designing a better solution

**Additional cons:**
- **Loses critical data:** The design mentions losing "category, formula name" but doesn't emphasize this breaks gap analysis. The batch pipeline needs to know *why* deterministic generation failed (missing package metadata vs unsupported formula type vs network error). Parsing "no LLM providers available" gives zero signal.
- Doesn't scale: Next time there's a new failure category, need to update both CLI error messages AND parser
- Internationalization nightmare if errors are ever localized
- Error messages can include dynamic data (file paths, URLs) that make regex patterns fragile

**Missing critical con:** This option doesn't actually solve the stated problem. The CLI will still attempt LLM fallback, still fail with "no LLM providers", and still exit 1. Parsing the error is just lipstick on a pig—the root issue is that deterministic-only mode isn't accessible via CLI.

## 4. Unstated Assumptions

### Assumption 1: Exit codes are the right interface

**Stated:** "Exit codes are the most reliable signal for subprocess-based callers"

**Unstated implications:**
- Assumes orchestrator continues to use exit codes as primary signal (vs parsing JSON output)
- Assumes Unix/POSIX environment where exit codes 0-255 are meaningful
- Assumes no conflict with other exit codes (design adds 9, but doesn't show full exit code table)

**Why it matters:** If issue #1273 (structured JSON output) is implemented soon, exit codes might become secondary. Should the design future-proof for that?

### Assumption 2: Deterministic-only mode is binary

**Unstated:** Either deterministic-only (no LLM fallback) OR allow LLM fallback. No middle ground like "try deterministic, report what LLM would do but don't actually call it."

**Why it matters:** For planning purposes, the batch orchestrator might want to know "which packages WOULD need LLM" without actually calling expensive APIs. This could inform prioritization of deterministic builder improvements.

### Assumption 3: Batch orchestrator is the only consumer

**Stated:** "The batch pipeline gets exit code 1..."

**Unstated:** Are there other automation contexts that need deterministic-only mode? CI pipelines? Reproducible builds? Pre-commit hooks?

**Why it matters:** If there are other use cases, the flag name and documentation should be general ("reproducible" or "no-llm" might be clearer than "deterministic-only"). If batch orchestrator is truly the only consumer, maybe the dedicated subcommand (Option C variant) makes more sense.

### Assumption 4: Error output to stderr is acceptable

**Stated:** "Print the failure category and message to stderr before exiting"

**Unstated:** Assumes orchestrator reads stderr (it might only check exit code). Also assumes stderr output format is stable enough for humans reading logs, but not so stable that it becomes an API contract.

**Why it matters:** If orchestrator does parse stderr, we're back to Option C's fragility problems. If it doesn't, why print structured data to stderr at all (vs just exit code)?

### Assumption 5: Flag won't be misused interactively

**Unstated:** Interactive users might see `--deterministic-only` in `--help` and think it means "reproducible build" or "use only local data" without understanding the LLM fallback implications.

**Why it matters:** Needs good documentation. Also suggests the flag should have a clear name (`--no-llm-fallback` is more explicit than `--deterministic-only`).

## 5. Strawman Detection

### Is Option B a strawman?

**Initial suspicion:** Yes. The cons are devastating (implicit behavior, violates least surprise, couples unrelated concepts).

**But:** The design doesn't misrepresent it. The pros genuinely include "no flag needed" and "batch pipeline doesn't need changes." These are real advantages for a lazy implementation.

**Verdict:** Not a strawman, just a weak option. It's the "what if we did nothing to the CLI and hacked the environment check" alternative. Worth including for completeness, but clearly inferior to Option A.

### Is Option C a strawman?

**Initial suspicion:** Maybe. The TODO comment at orchestrator.go:260 already says parsing is brittle, so proposing "do more brittle parsing" seems designed to fail.

**But:** This is the current de facto behavior. The design is documenting "here's what we'd have to do if we don't fix the CLI." That's useful for comparison.

**Verdict:** Not a strawman. It's the status quo extrapolated (parse the new error message instead of fixing the root cause). Correctly identified as inferior.

## Summary of Findings

**Problem Statement:** Well-defined. Minor improvement: Be explicit about whether interactive users ever need deterministic-only mode.

**Missing Alternatives:**
1. Environment variable `TSUKU_DETERMINISTIC_ONLY=1` (worth considering as variant of Option A)
2. Structured JSON output (broader scope, ties to #1273)
3. Dedicated `tsuku create-batch` subcommand (probably over-engineered)

**Pros/Cons Gaps:**
- Option A: Need to clarify behavior when flag is set with API keys available; flag name might be unclear
- Option B: Understates the severity of "implicit behavior" con—this violates user intent inference in both directions
- Option C: Doesn't solve the root problem; loses critical failure category data needed for gap analysis

**Unstated Assumptions:**
1. Exit codes remain primary interface (vs JSON output from #1273)
2. Deterministic-only is binary (no "dry run LLM" mode)
3. Batch orchestrator is the only automation consumer
4. Stderr output format is informal (not a parsed API contract)
5. Flag name won't confuse interactive users

**Strawman Analysis:** Neither Option B nor C is a strawman. Both are genuine (if flawed) alternatives that represent "do less work" approaches.

## Recommendations

1. **Proceed with Option A** with these refinements:
   - Consider environment variable as alternative to flag (or support both)
   - Document flag behavior when API keys ARE available (should still suppress LLM)
   - Show full exit code table to confirm 9 doesn't conflict
   - Clarify stderr output format expectations

2. **Acknowledge #1273 relationship:**
   - Note that structured JSON output (if implemented) would supersede exit codes for rich failure data
   - Design the flag to be composable with future `--output=json`

3. **Improve problem statement:**
   - Add a sentence about whether interactive users need this mode (seems like "no" based on Decision Drivers)
   - If batch orchestrator is the only consumer, state that explicitly

4. **Strengthen Option C's cons:**
   - Emphasize that parsing loses failure category data needed for gap analysis
   - Note it doesn't actually suppress LLM fallback attempt (still wasted CPU cycles)

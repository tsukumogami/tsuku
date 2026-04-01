# Clarity Review

## Verdict: PASS

## Ambiguities Found

**12 ambiguities identified**, grouped by severity.

### High Severity (could produce divergent implementations)

**1. "Well-known location" undefined (R2).** The requirement says shell init scripts go to "a well-known location managed by tsuku" but never names it. Two developers could choose `$TSUKU_HOME/shell.d/`, `$TSUKU_HOME/init.d/`, or `$TSUKU_HOME/hooks/` and produce incompatible layouts. The acceptance criteria mention "shell.d files" in passing, which implies a specific directory but doesn't constitute a requirement.

**2. Multi-version semantics are underspecified (R10).** The requirement says removing one version "must not delete shell integration artifacts that another installed version still uses." It doesn't define what "still uses" means. If v1 generates `init-niwa.sh` and v2 generates a different `init-niwa.sh`, which version's output wins? Is it last-write-wins, active-version-wins, or something else? The acceptance criterion says "v1 does not delete shell init files that v2 also references" but "references" is equally ambiguous -- does it mean identical file paths, identical content, or something else?

**3. Update continuity ("without a gap") is unmeasurable (R6).** The requirement says shell integration must transition "without a gap where the tool's shell features are unavailable." In practice, since init takes effect only in new shell sessions (Known Limitations), what constitutes a "gap"? If old version files are deleted before new ones are written, any shell opened in between sees nothing. The acceptance criterion repeats the same language without making it testable. A concrete requirement would specify the ordering guarantee (e.g., "new artifacts are written before old artifacts are deleted" or "update is atomic with respect to shell sessions").

**4. `source_command` trust model deferred but load-bearing.** R12 says hooks use "a limited vocabulary of declarative actions" with "no post-install code execution beyond the tool's own binary." But `source_command` (mentioned in Known Limitations) runs the tool binary to generate shell init, which IS post-install code execution. The PRD acknowledges this tension but defers the resolution to a downstream design doc. Since this is the mechanism needed for niwa (the motivating use case), two implementers could disagree on whether `source_command` is in scope or not.

### Medium Severity (likely to cause rework or spec clarifications during implementation)

**5. "Per shell type" scope is vague (R2).** "At minimum bash and zsh" -- does this mean the action generates separate files per shell, or one file that's compatible with both? Tools like direnv and zoxide produce different output for `bash` vs `zsh`. The recipe format needs to express which shells a given init step targets, but there's no indication of how.

**6. Completion registration mentioned but never specified.** The problem statement cites "200+ tools that would benefit from automatic completion registration" and R4 mentions "completions" in parenthetical, but no requirement defines a completion-specific action or delivery mechanism. It's unclear whether completions are in scope for this PRD or deferred.

**7. "A few lines of TOML" / "no more than 5 additional lines" (Goal 4, R13).** R13 says "no more than 5 additional lines," but the acceptance criterion says "a `[[steps]]` entry with `phase`, `action`, and 1-2 action-specific parameters." These are consistent but the 5-line budget is ambiguous -- does that mean 5 lines per hook, per phase, or total across all lifecycle hooks for a recipe? A tool needing both shell init and completions for both bash and zsh could easily exceed 5 lines.

**8. "Stale artifact" definition missing (R7).** The requirement says old artifacts must be removed if the updated version produces "different" artifacts. Different how? Different file paths? Different file content at the same path? If v1 writes `init-niwa.sh` and v2 also writes `init-niwa.sh` but with different content, is the old one "stale" (it was overwritten) or "cleaned up" (same path, no orphan)?

### Low Severity (unlikely to cause implementation divergence but worth tightening)

**9. "8-12 tools" and "10-20 tools" ranges.** The problem statement uses ranges ("8-12 tools that cannot function," "10-20 tools with service/daemon components") rather than exact counts. This is fine for motivation but makes it hard to evaluate coverage of the solution. A linked appendix or label query would strengthen this.

**10. 5ms performance budget lacks test methodology (R11).** The requirement specifies 5ms wall time "excluding the tsuku shellenv binary invocation" but doesn't specify: measured on what hardware class, with what shell, under what load, or with how many tools. The acceptance criterion ("10 tools providing shell init, less than 5ms") adds a tool count but still lacks reproducibility criteria.

**11. Graceful failure scope (R9).** "The failure must not block the overall install or remove operation" -- does this extend to update? If a post-install hook fails during `tsuku update`, should the update succeed (new binary installed, old shell integration left in place)? Or should the update roll back?

**12. "Artifacts created by its lifecycle hooks" scope (R4).** Only files that hooks created, or also files the tool binary itself created when invoked by a hook? If `source_command` runs `niwa shell-init bash > $TSUKU_HOME/shell.d/niwa.sh`, the hook created the file. But if the tool binary also writes to `~/.config/niwa/` as a side effect of running, is that in scope for cleanup?

## Suggested Improvements

1. **Name the shell.d directory explicitly** in R2 or add a recipe format sketch showing the target path structure.
2. **Define update atomicity** in R6: specify whether it means "write-before-delete" ordering, filesystem atomicity, or session-level atomicity (no shell opened during update sees a broken state).
3. **Resolve the `source_command` / R12 tension** within the PRD rather than deferring it. State explicitly whether running a tool's own binary to generate init output counts as "post-install code execution" under the declarative trust model.
4. **Specify per-shell generation** in R2: does the recipe declare shell targets, or does the action auto-detect available shells?
5. **Clarify completion scope**: either add a requirement for completion delivery or explicitly defer it to out-of-scope.
6. **Add a test methodology note** to R11 (e.g., "measured on a 2020-era laptop with default bash, cold shell, using `time` on the sourcing of concatenated init files").
7. **Define "references" in the multi-version acceptance criterion**: specify whether it means "same file path" or something more nuanced.
8. **Extend R9 graceful failure** to cover `tsuku update` explicitly.

## Summary

The PRD is well-structured with a clear problem statement, concrete motivating examples, and mostly testable acceptance criteria. The main risks are around multi-version semantics, update atomicity, and the unresolved tension between the declarative trust model and the `source_command` mechanism that the primary use case (niwa) requires. None of these ambiguities would cause a fundamentally wrong implementation, but they'd likely surface as spec clarification requests during design or code review. Tightening the 4 high-severity items before moving to design would prevent rework.

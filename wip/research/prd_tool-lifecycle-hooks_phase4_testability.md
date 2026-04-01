# Testability Review

## Verdict: PASS

All 10 acceptance criteria are testable. The PRD is unusually well-written for testability -- criteria are concrete, use specific tool names and commands, and describe observable outcomes. A QA engineer could write a test plan from the acceptance criteria alone without consulting the author.

## Untestable Criteria

None are technically untestable. Two criteria require careful test design:

- **AC7 (shell startup < 5ms with 10 tools):** Testable but sensitive to measurement methodology. "Sourcing the combined init content" is defined clearly enough (wall time, excluding the tsuku shellenv binary invocation), but 5ms benchmarks are noisy on shared CI runners. Needs a controlled environment or a generous margin. The criterion itself is well-specified -- the challenge is operational, not definitional.

- **AC10 (recipe requires only phase, action, and 1-2 parameters):** This is a recipe-authoring ergonomics claim. You can verify it by inspecting the schema and writing a sample recipe, but "1-2 action-specific parameters" is slightly ambiguous -- does a parameter with sub-fields count as one? Still testable by writing actual recipes and counting fields.

## Missing Test Coverage

The following requirements lack dedicated acceptance criteria:

1. **R2 (per-shell generation, bash and zsh minimum):** AC1 mentions bash and zsh but only in the context of niwa. There is no criterion that explicitly verifies a tool's init scripts are generated separately for each shell type. A tool that dumps the same script for both shells would pass AC1 but violate R2's intent. **Recommendation:** Add a criterion verifying that shell-specific init files exist for each supported shell.

2. **R12 (declarative trust model, no arbitrary shell scripts):** No acceptance criterion verifies that the hook system rejects or prevents imperative/arbitrary shell script execution. AC10 touches format but not security constraints. **Recommendation:** Add a negative test criterion: "A recipe with a step that attempts to run an arbitrary shell command in the post-install phase is rejected at recipe validation time."

3. **Error conditions beyond hook failure:** R9 (graceful failure) is covered by AC6, but only for the case where the tool binary fails to produce output. Missing scenarios:
   - What happens if the shell.d directory is read-only or missing?
   - What happens if a cleanup state file is corrupted?
   - What happens if an init script is deleted manually before `tsuku remove`?

4. **Multi-version install path:** AC8 covers the removal side of multi-version (removing v1 doesn't break v2). But there is no criterion for what happens when you *install* v2 alongside v1 -- does the shell integration switch to v2? Does it duplicate? The update path (AC3/AC4) covers sequential upgrades, not concurrent installs.

5. **Known limitation: pre-existing tools have no cleanup state.** The Known Limitations section documents this, but there is no acceptance criterion for it. A test verifying that `tsuku remove <tool-installed-before-feature>` behaves as today (no crash, no error about missing cleanup state) would be valuable.

## Edge Cases and Error Conditions

The criteria cover:
- Happy path: install (AC1), remove (AC2), update (AC3, AC4)
- Backward compatibility: AC5
- Failure handling: AC6 (hook failure during install)
- Performance: AC7
- Multi-version: AC8
- Offline removal: AC9
- Authoring ergonomics: AC10

Missing edge/error coverage:
- Concurrent operations (two installs running lifecycle hooks simultaneously)
- Interrupted operations (install killed mid-hook -- is cleanup state consistent?)
- Shell config file conflicts (two tools writing conflicting init scripts)
- Upgrade from pre-hook version of tsuku (state migration)

## Summary

The acceptance criteria are concrete, observable, and testable. Each uses specific commands (`tsuku install niwa`, `tsuku remove niwa`) with verifiable outcomes. The two gaps worth addressing before implementation are: (1) no negative test for the declarative-only security constraint (R12), and (2) no explicit per-shell-type verification (R2). Error condition coverage is adequate for a first pass but tilts toward the happy path -- adding 2-3 failure-mode criteria would strengthen confidence in the graceful-failure requirement.

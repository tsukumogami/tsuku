# Test Coverage Report: Sandbox CI Integration

## Coverage Summary

- Total scenarios: 18
- Executed: 17
- Passed: 17
- Failed: 0
- Skipped: 1

## Issues

- Issues completed: #1942, #1943, #1944, #1945, #1946, #1947
- Issues skipped: none

## Executed Scenarios

| Scenario | ID | Prerequisite | Status |
|----------|-----|-------------|--------|
| PlanVerify gains ExitCode field and format version bumps to 5 | scenario-1 | #1942 | passed |
| Shared CheckVerification function exists and handles all cases | scenario-2 | #1942 | passed |
| validate package uses shared CheckVerification | scenario-3 | #1942 | passed |
| buildSandboxScript appends verify block with marker files | scenario-4 | #1942 | passed |
| SandboxResult carries verification fields and Passed reflects both install and verify | scenario-5 | #1942 | passed |
| Plan generator copies ExitCode and plan cache includes it | scenario-6 | #1942 | passed |
| SandboxRequirements gains ExtraEnv field | scenario-7 | #1943 | passed |
| Env passthrough with key filtering protects hardcoded vars | scenario-8 | #1943 | passed |
| --env CLI flag is registered and populates SandboxRequirements | scenario-9 | #1943 | passed |
| SandboxResult gains DurationMs and timing is measured | scenario-10 | #1944 | passed |
| --json flag produces valid JSON for all sandbox result states | scenario-11 | #1944 | passed |
| Sandbox verification works end-to-end with a real recipe | scenario-12 | #1942, #1943, #1944 | passed |
| --env passthrough works end-to-end in sandbox | scenario-13 | #1942, #1943, #1944 | passed |
| test-recipe.yml Linux jobs use sandbox instead of docker run | scenario-14 | #1945 | passed |
| recipe-validation-core.yml Linux jobs use sandbox with retry | scenario-15 | #1946 | passed |
| batch-generate.yml validation phase uses sandbox | scenario-16 | #1947 | passed |
| validate-golden-execution.yml container jobs use sandbox | scenario-17 | #1947 | passed |

## Gaps

| Scenario | Reason |
|----------|--------|
| scenario-18: Full CI pipeline runs successfully after all migrations | Environment-dependent: requires pushing the branch to GitHub and manually triggering CI workflows (test-recipe.yml, recipe-validation-core.yml, batch-generate.yml, validate-golden-execution.yml). Cannot be validated by the tester agent in this environment. |

## Notes

- Scenarios 12 and 13 are marked as manual (require docker/podman runtime) but were executed and passed, indicating a container runtime was available during testing.
- Scenario 18 is the only gap. It is a full integration validation that requires CI infrastructure. All prerequisite issues (#1942-#1947) were completed, so the scenario is testable -- it just needs a human to push the branch and verify CI results.
- No issues were skipped, so no scenarios are blocked by missing prerequisites.

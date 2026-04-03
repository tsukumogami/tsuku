# Completeness Review

## Verdict: PASS

The PRD is detailed enough for an implementer to build all six workstreams without guessing, with a few gaps that should be addressed before implementation begins.

## Issues Found

1. **No AC for R1 content completeness**: R1 specifies CLAUDE.md must include repo description, monorepo structure, build/test/lint commands, CLI command table, development workflow, release process, and conventions. The acceptance criteria only check for the existence of CLAUDE.md, the Key Internal Packages section (R2), and the maintenance section (R3). There is no AC verifying the R1 content items are present. Add acceptance criteria checking for each major content section listed in R1.

2. **No AC for R7 settings.json source declarations**: R7 requires settings.json to declare the local tsuku marketplace via file source and shirabe via GitHub source with sparsePaths. The AC checks that settings.json "enables tsuku-recipes@tsuku and shirabe@shirabe" but does not verify the source declaration format (file vs GitHub, sparsePaths configuration). Add AC verifying source types and sparsePaths for shirabe.

3. **No AC for R9 version provider completeness**: R9 requires listing "all version provider types" but no AC verifies completeness against the actual codebase. The scope document mentions "13+ version providers" while the PRD says "15 version providers" -- there is no authoritative count. Add an AC that the listed providers match what internal/version/ implements, or soften R9 to "all documented version provider types."

4. **No AC for R11 verification quick-start**: R11 requires coverage of version mode, output mode, and format transforms (semver, strip_v, calver). The AC mentions "verification quick-start" in passing within the recipe-author section but only checks for "version mode, output mode, format transforms" as a single line item. This is adequate but thinly specified -- an implementer might include only a mention rather than actionable examples. Consider requiring at least one example per mode.

5. **No AC for R14 install syntax coverage**: R14 requires documenting install syntax variants (owner/repo, owner/repo:recipe, owner/repo@version). The AC says "SKILL.md covers .tsuku-recipes/ directory setup and install syntax" but does not verify that all three syntax variants are documented. Add AC enumerating the required syntax forms.

6. **R20 AGENTS.md lacks content requirements**: R20 says "Create plugins/tsuku-recipes/AGENTS.md providing recipe authoring guidance for non-Claude-Code agents" but specifies no content structure, minimum coverage, or relationship to SKILL.md content. An implementer must guess what "recipe authoring guidance" means for a non-Claude-Code agent. Specify at minimum: what topics to cover, expected length/depth, and whether it should reference the same exemplars.

7. **Scope document mentions golden file comparison; PRD partially addresses**: The scope says recipe-test covers "validate -> eval -> sandbox -> golden workflow." R15 includes "golden file validation" as a command, but it's also listed as out of scope ("Golden file snapshot validation for exemplar recipes"). These refer to different things (testing workflow documentation vs CI validation of exemplars), but the boundary could confuse an implementer. Clarify in the PRD that the recipe-test skill documents the golden file workflow for recipe authors, while the out-of-scope item refers to using golden files in CI to validate skill content.

8. **No requirement for gpu in when clause AC**: R10 requires coverage of os, libc, linux_family, and gpu filters. The AC (line 224) only mentions "os, libc, linux_family examples" -- gpu is missing. Add gpu to the AC.

## Suggested Improvements

1. **Add a file tree of deliverables**: The requirements span 6 workstreams creating files in different locations. A single tree showing every file path to be created or modified would help implementers verify completeness and reviewers check coverage.

2. **Specify AGENTS.md content guidance**: Even a brief list of required sections (overview, recipe format basics, action reference pointer, testing commands) would prevent the AGENTS.md from becoming either a stub or an uncontrolled duplication of SKILL.md.

3. **Clarify CI workflow file location and naming**: R21 describes what the CI check does but not where the workflow file lives or its naming convention. Given the monorepo structure with existing workflows in .github/workflows/, specifying the expected file name would reduce ambiguity.

4. **Add migration/rollback notes**: The PRD requires reducing CLAUDE.local.md (R4) and creating committed settings.json (R7). Both modify files that active contributors depend on. A brief note about ordering (create CLAUDE.md before trimming CLAUDE.local.md, ensure settings.json and settings.local.json coexist correctly) would help implementation sequencing.

5. **Quantify "5-8 curated recipe paths" categories**: R12 lists 7 pattern categories but requires only 5-8 recipes. Clarifying whether each category must have at least one exemplar, or whether some categories can be skipped, would prevent review disputes.

## Summary

The PRD is well-structured with clear workstream separation, explicit requirement numbering, and thorough acceptance criteria for most requirements. The main gaps are: missing AC for R1's content items and R14's syntax variants, underspecified AGENTS.md content (R20), a missing "gpu" filter in the when-clause AC, and minor ambiguity around golden file scope boundaries. These are fixable without restructuring. The 8 issues found are mostly AC coverage gaps rather than missing requirements, which indicates the requirements themselves are solid.

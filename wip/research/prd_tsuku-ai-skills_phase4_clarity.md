# Clarity Review

## Verdict: PASS

This PRD is unusually precise for its category -- most requirements specify exact file paths, exact content sections, and exact behavioral constraints, leaving little room for divergent interpretation.

## Ambiguities Found

1. **R2 "one-line descriptions"**: "At minimum: actions, config, distributed, executor, install, recipe, registry, version, updates, project, shellenv, telemetry" -- This lists 12 package names but says "at minimum," leaving the upper bound undefined. A developer could list 12 packages or 30. The acceptance criterion says "at least 12 packages" which matches R2, but two developers could produce substantially different package lists beyond the named 12.
   -> Suggested clarification: Either make the list exhaustive or add criteria for which additional packages qualify (e.g., "all packages under internal/ with 3+ source files").

2. **R8 "compact action names table covering all action categories"**: "All action categories" is clear, but "compact" is subjective. One developer might include parameter columns, another might limit to name + description.
   -> Suggested clarification: Specify the exact columns for the table (e.g., "action name, category, one-line description").

3. **R12 "5-8 curated recipe paths covering distinct pattern categories"**: The pattern categories are listed (7 categories), but the range "5-8" means a developer could skip 2 categories. Which categories are required vs. optional?
   -> Suggested clarification: Either require one exemplar per listed category (making it 7) or rank the categories by priority so developers know which to drop.

4. **R17 "common failure patterns with exit codes: exit 6 (container failure), exit 8 (verification failure)"**: Only two exit codes are named. "Common failure patterns" suggests there should be more content beyond these two codes, but what else qualifies as "common" is undefined.
   -> Suggested clarification: Either enumerate the full set of failure patterns expected, or change to "must document at least exit codes 6 and 8 with their failure scenarios."

5. **R20 "Create plugins/tsuku-recipes/AGENTS.md providing recipe authoring guidance for non-Claude-Code agents"**: No specification of what content this file should contain, what format to use, or what level of detail is expected. Two developers could produce very different files -- one might write 10 lines, another 200.
   -> Suggested clarification: Add minimum content requirements (e.g., "must cover: recipe format overview, action reference, testing workflow, and links to guide files") or reference a template.

6. **R3 "instruct contributors to assess recipe skills after changes"**: "Assess" is vague -- does this mean run a specific command, read the skill file and check for conflicts, or something else? The koto pattern is referenced but not quoted, so the expected behavior depends on external context.
   -> Suggested clarification: Define what "assess" means concretely (e.g., "read the SKILL.md and verify any referenced behavior still matches the changed code").

7. **AC "No content is duplicated between CLAUDE.md and CLAUDE.local.md"**: "Duplicated" could mean exact text duplication or conceptual overlap. A section header appearing in both files with different content could be flagged or not depending on interpretation.
   -> Suggested clarification: Change to "No sections or paragraphs from CLAUDE.local.md appear in CLAUDE.md" or define duplication as "same information conveyed in both files."

8. **R19 "autoUpdate must be omitted (defaults to false)"**: This assumes specific Claude Code plugin behavior that external readers may not know. If the default changes, the requirement becomes incorrect. Minor, but the reasoning is embedded in the requirement rather than documented as a decision.
   -> Suggested clarification: This is acceptable as-is since it captures current behavior, but consider adding a note in Known Limitations about the default assumption.

## Suggested Improvements

1. **Define AGENTS.md content requirements**: R20 is the weakest requirement in the document. Every other file has specific content expectations; AGENTS.md has none. Add 3-4 required sections or reference an existing template.

2. **Resolve the 5-8 vs. 7 categories tension in R12**: The listed categories total 7, but the range allows 5-8. Either require all 7 or specify which are optional. The acceptance criteria ("5-8 recipe paths") inherits this ambiguity.

3. **Add column spec for action names table (R8)**: "Compact" is the only subjective word in the otherwise precise Workstream C requirements. Specifying columns removes it.

4. **Clarify "assess" in maintenance protocol (R3)**: This is the requirement most likely to produce divergent implementations. A concrete assessment checklist (even 2-3 bullet points) would make the protocol actionable rather than aspirational.

## Summary

The PRD is well-structured with 22 numbered requirements and 30 binary acceptance criteria, most of which are objectively verifiable (file exists, section contains X, field does not contain Y). The 8 ambiguities found are minor -- they cluster around content depth expectations (AGENTS.md, action table columns, failure patterns) rather than structural or architectural disagreements. Two developers would build substantially the same system; the differences would be in editorial choices within specific files. The weakest area is R20 (AGENTS.md), which has no content specification at all.

# Testability Review

## Verdict: PASS

The acceptance criteria are overwhelmingly testable -- most are file-existence or content-presence checks that can be verified mechanically. A small number of criteria have ambiguity or missing edge coverage.

## Untestable Criteria

1. **"No content is duplicated between CLAUDE.md and CLAUDE.local.md"**: Subjective boundary -- what counts as "duplicated"? A section header repeated with different content? A build command mentioned in both files in different contexts? -> Define duplication as "no section headings or multi-line blocks that appear verbatim in both files" or list the specific sections that must not appear in CLAUDE.local.md.

2. **"SKILL.md contains an action names table covering composites and primitives"**: "Covering" is ambiguous -- does it mean every action, or a representative subset? The requirement (R8) says "all action categories" but the AC says "covering composites and primitives." -> Specify "lists all action names from internal/actions/" or provide an exact count/enumeration to validate against.

3. **"SKILL.md lists all version provider types with source values"**: Testable only if there's a canonical list to compare against. The requirement says 15 providers but the AC doesn't reference a count or source of truth. -> Add "all N version provider types as defined in internal/version/" with the count, or reference a file that serves as the canonical list.

4. **"settings.json does not contain credentials, env vars, or personal configuration"**: "Personal configuration" is subjective. Permissions, hooks, and env vars are clear, but what about preferred model settings or theme preferences? -> Enumerate the specific keys that must NOT appear (e.g., env, hooks, permissions, mcpServers with local paths) rather than using the open-ended "personal configuration."

## Missing Test Coverage

1. **R1 content completeness**: The AC for CLAUDE.md only checks for "Key Internal Packages" and "Plugin Maintenance" sections. There's no AC verifying the promoted content from CLAUDE.local.md (repo description, monorepo structure, build/test/lint commands, CLI command table, development workflow, release process, conventions). R1 is substantial but has no corresponding AC beyond file existence.

2. **R12 exemplar count range**: The AC checks that exemplar-recipes.md "exists with 5-8 recipe paths" but doesn't verify that the recipes cover the required distinct pattern categories (binary download, homebrew-backed, source build, platform-conditional, ecosystem-delegated, library, custom verification). A file with 6 binary-download recipes would pass the AC but fail R12.

3. **R14 distributed install syntax**: The AC says "covers .tsuku-recipes/ directory setup and install syntax" but doesn't verify the specific syntax forms (owner/repo, owner/repo:recipe, owner/repo@version) are documented. A section that mentions "install syntax" without listing the forms would pass.

4. **Error paths and edge cases**: No AC covers what happens when CI validation fails (R21) -- does it block merge? What's the expected error output? The AC only says "validates that exemplar recipe paths exist and pass tsuku validate" but not what the failure mode looks like.

5. **R3 maintenance protocol completeness**: The AC checks that the maintenance section "names internal/actions/, internal/version/, and internal/recipe/ as trigger areas" and "distinguishes broken contracts and new surface." But R3 also requires the section to "instruct contributors to assess recipe skills after changes" -- the instructional aspect (what to do, not just what to look for) has no AC.

6. **R19 sparsePaths correctness**: The AC checks the snippet "contains sparsePaths" but not that the paths are correct (.claude-plugin/ and plugins/tsuku-recipes/ only). A snippet with sparsePaths pointing to wrong directories would pass.

7. **R20 AGENTS.md content**: The AC only checks that AGENTS.md exists, with no criteria for its content providing "recipe authoring guidance for non-Claude-Code agents." An empty or placeholder file would pass.

8. **Plugin JSON schema validity**: No AC verifies that marketplace.json and plugin.json contain valid JSON matching expected schemas -- only that the files exist and declare/list the right names.

## Summary

Most criteria (roughly 75%) are cleanly testable file-existence and content-grep checks that could be automated in a CI script. The main gaps are: R1's promoted content has no AC at all (6+ content areas unverified), several criteria use "covers" or "lists all" without a canonical source to compare against, and the AGENTS.md requirement has existence-only coverage with no content validation. Fixing these would require adding ~8 new criteria and tightening ~4 existing ones.

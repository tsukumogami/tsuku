# Issue 716 Introspection

## Context Reviewed

- Design doc: `docs/DESIGN-golden-plan-testing.md`
- Sibling issues reviewed: #714 (closed), #715 (open)
- Prior patterns identified from #714 PR #730:
  - Scripts located in `scripts/regenerate-golden.sh` and `scripts/validate-golden.sh`
  - Golden files stored in `testdata/golden/plans/{first-letter}/{recipe}/v{version}-{platform}.json`
  - Non-deterministic fields (`generated_at`, `recipe_source`) stripped using jq
  - linux-arm64 excluded from generation (no CI runner)
  - One pilot golden file exists: `testdata/golden/plans/f/fzf/v0.60.0-linux-amd64.json`

## Gap Analysis

### Minor Gaps

1. **CI requirement for generation is understated**: The issue says "All files generated in CI (not locally)" but the scripts support local execution. The intent is that the _committed_ files should originate from CI for trust purposes, but developers can run locally for testing. This should be understood during implementation.

2. **Version selection method not specified**: The design doc shows using `tsuku versions <recipe>` to get latest, and the scripts support both explicit versions and auto-detection from existing golden files. The pilot should establish which specific versions to use for each recipe.

3. **Pilot recipe list in issue body is suggestive, not prescriptive**: The issue mentions specific recipes (fzf, ripgrep, lazygit, etc.) but ripgrep doesn't exist in the registry. The list should be treated as guidance for action type coverage, not a literal recipe list.

### Moderate Gaps

None identified. The issue is well-specified and the blocking dependency (#714) established clear patterns.

### Major Gaps

None identified. The scripts are functional, the directory structure is established, and the acceptance criteria are achievable.

## Validation of Issue Requirements

| Acceptance Criterion | Current State | Notes |
|---------------------|---------------|-------|
| Golden files for ~30 recipes | 1 exists (fzf) | Need to select and generate ~29 more |
| Coverage of all major action types | Partially specified | Need to verify coverage |
| Files in `testdata/golden/plans/{letter}/{recipe}/` | Structure exists | Already established |
| Naming: `v{version}-{os}-{arch}.json` | Correct | Verified in fzf golden file |
| All files generated in CI | N/A | Generation will happen via CI workflow |
| At least one recipe with dependencies | Not yet | sqlite (depends on readline) is a candidate |

### Major Action Types to Cover

Based on recipe review:

| Action Type | Example Recipes Available | Notes |
|-------------|---------------------------|-------|
| `download` | terraform | Direct URL download |
| `download_archive` | golang, nodejs | Archive download with extraction |
| `download_file` | - | (fzf uses this via github_archive expansion) |
| `github_archive` | fzf, lazygit, btop, lf, lsd | GitHub release archive |
| `github_file` | nix-portable | GitHub release single file |
| `homebrew` | readline, sqlite, ncurses | Homebrew bottle |
| `cargo_install` | cargo-audit, cargo-edit | Rust cargo install |
| `npm_install` | amplify, netlify-cli, cdk | NPM package |
| `gem_install` | jekyll, bundler | RubyGems |
| `require_system` | docker, cuda | Skipped (no plan generation) |

### Recommended Pilot Recipe Selection (~30 recipes)

**GitHub Archive/Release Downloads (10):**
1. fzf (already exists)
2. lazygit
3. btop (linux-only)
4. lf
5. lsd
6. bottom
7. glow
8. just
9. ack
10. age

**Direct URL Downloads (5):**
11. terraform
12. golang
13. nodejs
14. ruby
15. rust

**Homebrew Bottles (5):**
16. readline
17. sqlite (has dependency on readline)
18. ncurses
19. cmake
20. openssl

**Ecosystem Builds (10):**
21. cargo-audit
22. cargo-edit
23. cargo-watch
24. amplify
25. netlify-cli
26. cdk
27. jekyll
28. bundler
29. serve
30. serverless

**Platform-specific (covered above):**
- btop (linux-only)
- nix-portable (linux-only, if desired)

## Recipe Existence Verification

Verified the following recipes exist in `internal/recipe/recipes/`:
- fzf, lazygit, btop, lf, lsd, bottom, glow, just: Exist
- ack: Exists in `a/ack.toml`
- age: Exists in `a/age.toml`
- terraform, golang, nodejs, ruby, rust: Exist
- readline, sqlite, ncurses, cmake, openssl: Exist
- cargo-audit, cargo-edit, cargo-watch: Exist
- amplify, netlify-cli, cdk: Exist
- jekyll, bundler, serve, serverless: Exist

Note: ripgrep was mentioned in issue but does not exist in the registry.

## Recommendation

**Proceed**

The issue is well-specified and the blocking dependency established clear patterns. The implementation can proceed with the following understanding:

1. Use the pilot recipe list above (or similar) to achieve ~30 recipes with action type coverage
2. Scripts from #714 are functional and ready to use
3. linux-arm64 is excluded per established pattern
4. Generation should happen via CI workflow dispatch or PR automation to ensure checksums are computed on trusted infrastructure

## Proposed Amendments

None required. The minor gaps identified are implementation details that can be resolved during execution without user input.

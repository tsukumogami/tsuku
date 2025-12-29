# Issue 716 Implementation Plan

## Summary

Generate golden files for ~30 representative recipes covering all major action types using the regenerate-golden.sh script established in #714.

## Approach

Use `scripts/regenerate-golden.sh` to generate golden files for each recipe in the pilot set. Recipes are selected to cover all major action types while prioritizing tools with stable versions to minimize checksum churn.

The generation must happen in this PR (via CI) to ensure checksums are computed on trusted infrastructure, not local machines.

## Files to Create

Golden files will be created under `testdata/golden/plans/{first-letter}/{recipe}/`:

**GitHub Archive/Release (~10 recipes):**
- `f/fzf/` (already exists)
- `l/lazygit/`
- `b/btop/` (linux-only)
- `l/lf/`
- `l/lsd/`
- `b/bottom/`
- `g/glow/`
- `j/just/`
- `a/ack/`
- `a/age/`

**Direct URL Downloads (~5 recipes):**
- `t/terraform/`
- `g/golang/`
- `n/nodejs/`
- `r/ruby/`
- `r/rust/`

**Homebrew Bottles (~5 recipes):**
- `r/readline/`
- `s/sqlite/` (dependency on readline)
- `n/ncurses/`
- `c/cmake/`
- `o/openssl/`

**Ecosystem Builds (~10 recipes):**
- `c/cargo-audit/`
- `c/cargo-edit/`
- `c/cargo-watch/`
- `a/amplify/`
- `n/netlify-cli/`
- `c/cdk/`
- `j/jekyll/`
- `b/bundler/`
- `s/serve/`
- `s/serverless/`

## Implementation Steps

- [ ] Verify all pilot recipes exist and are functional
- [ ] Generate golden files for each recipe batch (using regenerate-golden.sh)
- [ ] Validate generated files with validate-golden.sh
- [ ] Commit golden files with descriptive message
- [ ] Create PR and monitor CI

## Success Criteria

- [ ] ~30 golden files generated across all action types
- [ ] All major action types covered (github_archive, download, homebrew, cargo_install, npm_install, gem_install)
- [ ] At least one recipe with dependencies (sqlite → readline)
- [ ] Naming follows `v{version}-{os}-{arch}.json` convention
- [ ] All files generated successfully with validate-golden.sh passing
- [ ] CI passes (files generated on trusted infrastructure)

## Open Questions

None - the implementation path is clear from #714's established patterns.

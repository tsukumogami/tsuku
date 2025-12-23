# Issue 559 Summary

## What Was Implemented

Added git recipe using Homebrew bottles for production and git-source test recipe that builds from source. This validates the complete toolchain with git's complex multi-dependency chain: git → curl → openssl/zlib + expat.

## Changes Made

- `internal/recipe/recipes/g/git.toml`: Production recipe using homebrew bottle with curl dependency
- `testdata/recipes/git-source.toml`: Test recipe building from source with all dependencies (curl, openssl, zlib, expat)
- `test/scripts/verify-tool.sh`: Added verify_git function testing git --version and git clone
- `.github/workflows/build-essentials.yml`: Added test-git-source job to CI matrix (tests on 3 platforms)
- `docs/DESIGN-dependency-provisioning.md`: Updated mermaid diagram to mark #559 as done

## Key Decisions

- **Use homebrew bottles for production git**: Faster installs, follows established pattern
- **Build from source for testing**: Validates configure_make with complex multi-dependency builds
- **Test git clone functionality**: Validates curl integration works end-to-end
- **Include all library dependencies**: curl, openssl, zlib, expat to validate complete dependency chain

## Trade-offs Accepted

None - follows proven patterns from PR #661 (sqlite/readline)

## Test Coverage

- **New tests added**:
  - verify_git function (git --version + git clone test)
  - test-git-source CI job (runs on 3 platforms)
- **Coverage**: Validates most complex dependency chain in the system

## Known Limitations

None - git recipe follows all established patterns

## Future Improvements

- Could add more git subcommands to verification (git config, git log, etc.)
- Could test git operations beyond clone (commit, push to test repo)

# Issue 1364 Implementation Plan

## Summary

Create a `cmd/seed-discovery` CLI tool that merges curated seed lists with priority queue entries, validates them against GitHub/Homebrew APIs, and outputs a schema v2 discovery registry. Evolve `RegistryEntry` and `ParseRegistry` to support v2 with optional metadata fields while keeping v1 backward compatibility.

## Approach

Follow the existing `cmd/seed-queue` pattern: flag-based CLI, load inputs, process, write output. Validation logic goes in a new `internal/discover/validate.go` to keep it testable and separate from the CLI. Schema evolution is additive (new optional fields on `RegistryEntry`, `ParseRegistry` accepts 1 or 2).

### Alternatives Considered

- **Separate v2 struct**: Create a distinct `RegistryEntryV2` type. Not chosen because the fields are all optional/omitempty and the existing `Lookup` method only reads builder+source, so a single struct works without breaking anything.
- **External validation binary**: Put validation in a separate tool. Not chosen because validation is tightly coupled to the seed-discovery pipeline and sharing the same types keeps things simple.

## Files to Modify

- `internal/discover/registry.go` - Add optional metadata fields to `RegistryEntry` (Description, Homepage, Repo, Disambiguation), update `ParseRegistry` to accept schema_version 1 or 2
- `internal/discover/registry_test.go` - Add tests for schema v2 parsing, mixed v1/v2 acceptance

## Files to Create

- `cmd/seed-discovery/main.go` - CLI entry point with flags, orchestration logic
- `internal/discover/validate.go` - GitHub and Homebrew API validation with retry/caching
- `internal/discover/validate_test.go` - Unit tests for validation (using httptest)
- `internal/discover/seedlist.go` - Seed list loading and merging logic
- `internal/discover/seedlist_test.go` - Tests for seed list parsing and merge behavior
- `internal/discover/generate.go` - Registry generation (merge seeds + queue, sort, write output)
- `internal/discover/generate_test.go` - Tests for generation pipeline
- `data/discovery-seeds/github-release-tools.json` - Initial seed list with ~10 GitHub release tools

## Implementation Steps

- [ ] Add optional metadata fields to `RegistryEntry` in registry.go (Description, Homepage, Repo, Disambiguation)
- [ ] Update `ParseRegistry` to accept schema_version 1 or 2 (change the version check)
- [ ] Add v2 parsing tests to registry_test.go
- [ ] Create `internal/discover/seedlist.go` with types for seed list format and loading from directory
- [ ] Create `internal/discover/seedlist_test.go` with parsing and merge tests
- [ ] Create `internal/discover/validate.go` with GitHub and Homebrew validators (interface, retry, caching, GITHUB_TOKEN support)
- [ ] Create `internal/discover/validate_test.go` with httptest-based validation tests
- [ ] Create `internal/discover/generate.go` with pipeline: load seeds + queue, merge (seeds win), validate, enrich (stub), cross-reference recipes (stub), sort, write output
- [ ] Create `internal/discover/generate_test.go` with end-to-end generation tests
- [ ] Create `cmd/seed-discovery/main.go` with flags (-seeds-dir, -queue, -output, -recipes-dir, -validate-only, -verbose)
- [ ] Create `data/discovery-seeds/github-release-tools.json` with ~10 entries (ripgrep, fd, bat, delta, lazygit, eza, hyperfine, jless, xh, yq)
- [ ] Run `go test ./internal/discover/...` and `go build ./cmd/seed-discovery`
- [ ] Run `go vet ./...` and verify no lint issues

## Testing Strategy

- **Unit tests**: ParseRegistry v1/v2 acceptance, seed list loading/merging, validation retry/caching logic (httptest servers), generation pipeline with mock validators
- **Integration tests**: Build seed-discovery binary, run with test fixtures, verify output JSON structure
- **Manual verification**: Run the validation script from the issue against the built tool

## Risks and Mitigations

- **GitHub API rate limits**: Use GITHUB_TOKEN env var for authenticated requests (5000/hr vs 60/hr). Cache responses in-memory to avoid duplicate calls. During tests, use httptest mock servers.
- **Priority queue entries with blocked/failed status**: Include all entries regardless of status. The tool validates against APIs, not queue status. Failed queue entries may still be valid homebrew formulas.
- **Large discovery.json diffs**: Alphabetical sorting ensures stable output. Schema v2 is a one-time migration; subsequent runs produce minimal diffs.

## Success Criteria

- [ ] `go build ./cmd/seed-discovery` succeeds
- [ ] `go test ./internal/discover/...` passes with new v2 and seed-discovery tests
- [ ] `ParseRegistry` accepts both schema v1 and v2 JSON
- [ ] Existing `Lookup` tests still pass unchanged
- [ ] Seed list at `data/discovery-seeds/github-release-tools.json` exists with ~10 entries
- [ ] Running `./seed-discovery -verbose` produces `recipes/discovery.json` with schema_version 2
- [ ] `-validate-only` mode validates existing discovery.json without regenerating
- [ ] Issue validation script passes end-to-end

## Open Questions

None. The issue is well-specified as a skeleton with stubs for enrichment (#1366), graduation (#1367), and full seed data (#1368).

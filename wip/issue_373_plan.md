# Issue 373 Implementation Plan

## Summary

Add `--skip-validation` flag to `tsuku create` that allows users without Docker to generate LLM recipes by skipping container validation, with an explicit consent prompt and metadata tracking.

## Approach

The implementation follows the design in DESIGN-llm-productionize.md. When `--skip-validation` is provided:
1. Show a warning about risks and require explicit y/N confirmation
2. Skip passing an executor to GitHubReleaseBuilder (which already supports nil executor)
3. Add `llm_validation = "skipped"` metadata to the generated recipe
4. Show a warning in the recipe output

The GitHubReleaseBuilder already handles the case where executor is nil (sets `validationSkipped = true`), so we primarily need CLI changes.

### Alternatives Considered

- **Adding a new field to Recipe struct**: Rejected because the design specifies adding metadata as a comment or extra field in the TOML, and adding to Recipe struct would require changes to the loader and all consumers.
- **Using environment variable instead of flag**: Rejected because consent should be explicit per-operation, not set globally.

## Files to Modify

- `cmd/tsuku/create.go` - Add `--skip-validation` flag, consent prompt, and conditional executor creation

## Files to Create

None - all changes fit within existing files.

## Implementation Steps

- [ ] 1. Add `createSkipValidation` flag variable and register with cobra
- [ ] 2. Add `confirmSkipValidation()` function for consent prompt
- [ ] 3. Modify GitHubReleaseBuilder creation to skip executor when flag is set (after consent)
- [ ] 4. Add warning to recipe output when validation was skipped
- [ ] 5. Add metadata comment/field to generated recipe for `llm_validation = "skipped"`
- [ ] 6. Add unit test for consent prompt logic
- [ ] 7. Run tests and verify implementation

## Testing Strategy

- Unit tests: Test the consent confirmation logic (accepts y/yes, rejects n/no/empty)
- Manual verification: Run `tsuku create <tool> --from github:owner/repo --skip-validation` and verify:
  - Consent prompt appears with correct warning text
  - Typing 'n' or empty exits without creating recipe
  - Typing 'y' or 'yes' proceeds
  - Generated recipe includes validation skipped warning
  - Generated recipe includes `llm_validation` metadata

## Risks and Mitigations

- **Risk**: Users ignore consent and install broken recipes
  - **Mitigation**: Clear warning text mentioning specific risks (binary path errors, missing extraction steps, failed verification)

- **Risk**: Non-interactive mode (piped input) breaks consent
  - **Mitigation**: Use existing `isInteractive()` pattern from install.go; refuse skip-validation in non-interactive mode

## Success Criteria

- [ ] `--skip-validation` flag added to create command
- [ ] Consent prompt shown with explicit risks
- [ ] Requires `y` or `yes` to proceed (default is no)
- [ ] Recipe metadata includes `llm_validation = "skipped"`
- [ ] Warning shown in recipe preview about skipped validation
- [ ] Flag documented in `--help` output

## Open Questions

None - design is clear from DESIGN-llm-productionize.md.

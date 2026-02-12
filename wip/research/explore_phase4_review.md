# Phase 4 Review: Error UX and Verbose Mode

## Summary

The design is solid overall. Key recommendations:

1. **Clarify scope**: Discovery-specific errors, not direct `--from` usage
2. **Add ecosystem probe logging**: The design depends on logging infrastructure that doesn't exist yet
3. **Consider splitting ConfigurationError**: Two distinct issues (missing key vs user flag)
4. **Avoid stage index magic numbers**: Use type switches instead of indices
5. **Document Unwrap() behavior**: Specify error chain participation
6. **Align verbose output examples**: Contract and implementation wording differ

## Full Analysis

### Problem Statement
Specific enough with one gap: doesn't clarify whether errors apply to `create --from` usage.

### Coverage of 8 Error Scenarios
- 6 of 8 covered correctly
- Ecosystem probe timeout needs explicit implementation note
- 2 correctly out of scope (homoglyph, ambiguity)

### Error Type Recommendations
- Consider splitting ConfigurationError into MissingCredentialsError and DeterministicModeError
- Document when Source field is populated in BuilderRequiresLLMError
- Stage index magic numbers (0, 1, 2) are fragile - use type switches

### Verbose Output
- Contract examples and implementation examples have different wording
- Ecosystem probe needs logging infrastructure added

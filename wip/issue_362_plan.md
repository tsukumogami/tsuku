# Issue 362 Implementation Plan

## Summary

Add `llm.enabled` and `llm.providers` configuration keys to the userconfig package, allowing users to disable LLM features or configure provider ordering via `tsuku config`.

## Approach

Extend the existing userconfig pattern (Get/Set/AvailableKeys) to support the two new keys. The `llm.enabled` key is a boolean with default `true`. The `llm.providers` key is a comma-separated list of provider names (e.g., "claude,gemini") where the first is the primary provider. Update `NewFactory` to accept a new `WithConfig` option that reads these settings.

### Alternatives Considered
- **Separate LLM config struct**: Would require more code changes and doesn't fit the flat key pattern established for telemetry. Not chosen because it adds complexity without benefit.
- **Environment variable only**: Would be inconsistent with existing config pattern and less user-friendly. Not chosen because `tsuku config` provides a better UX.

## Files to Modify
- `internal/userconfig/userconfig.go` - Add LLMEnabled and LLMProviders fields, update Get/Set/AvailableKeys
- `internal/userconfig/userconfig_test.go` - Add tests for new config keys
- `internal/llm/factory.go` - Add WithConfig option, update NewFactory to respect llm.enabled/providers
- `internal/llm/factory_test.go` - Add tests for config-based behavior
- `cmd/tsuku/config.go` - Update help text to document new keys

## Files to Create
None

## Implementation Steps
- [ ] Add LLMEnabled (bool, default true) and LLMProviders ([]string, default nil) to Config struct
- [ ] Implement Get/Set for "llm.enabled" key
- [ ] Implement Get/Set for "llm.providers" key (comma-separated string format)
- [ ] Update AvailableKeys with descriptions for both new keys
- [ ] Add unit tests for new config keys
- [ ] Add WithConfig option to factory that sets enabled flag and provider order
- [ ] Update NewFactory to check enabled flag and set primary from config
- [ ] Add factory tests for config-based behavior
- [ ] Update cmd/tsuku/config.go help text

Mark each step [x] after it is implemented and committed. This enables clear resume detection.

## Testing Strategy
- Unit tests: Test Get/Set for both keys, test defaults, test invalid values
- Unit tests: Test factory with llm.enabled=false returns error, test provider ordering from config
- Manual verification: `tsuku config get llm.enabled`, `tsuku config set llm.providers gemini,claude`

## Risks and Mitigations
- **Breaking existing behavior**: Mitigation: defaults match current behavior (enabled=true, providers=nil means auto-detect)
- **Invalid provider names**: Mitigation: validate against known providers or silently skip unknown ones

## Success Criteria
- [ ] `tsuku config get llm.enabled` returns "true" by default
- [ ] `tsuku config set llm.enabled false` disables LLM factory creation
- [ ] `tsuku config set llm.providers gemini,claude` sets gemini as primary
- [ ] All existing tests pass
- [ ] New tests cover the config keys and factory integration

## Open Questions
None - requirements are clear from issue #362.

# Issue 1639 Implementation Plan

## Summary

Implement JSON Schema to GBNF grammar conversion in Rust to constrain llama.cpp token generation to valid JSON matching tool schemas, using the native `llama_sampler_init_grammar` API.

## Approach

The implementation uses llama.cpp's sampler chain architecture. A grammar sampler is created from GBNF rules and added to the sampling pipeline. This sampler modifies logits to only allow tokens that produce valid grammar continuations, ensuring all generated output matches the specified JSON Schema.

The conversion from JSON Schema to GBNF happens in Rust before inference, targeting only the subset of JSON Schema used by tsuku tools (objects, arrays, strings, required/optional properties).

### Alternatives Considered

1. **Use llama.cpp's built-in JSON Schema converter (Python/JS)**: Not available in C API - only raw GBNF strings are accepted via `llama_sampler_init_grammar()`. Would require embedding Python/Node which adds significant complexity.

2. **Post-hoc JSON validation**: Parse output after generation and retry on invalid JSON. Wastes tokens, doesn't guarantee eventual success, and slower for complex schemas. Grammar constraints are strictly superior.

3. **Manual logit masking in Rust**: Implement grammar constraint logic manually by masking invalid tokens. Duplicates llama.cpp's proven implementation, error-prone, and misses optimization work in llama.cpp.

## Files to Modify

- `tsuku-llm/src/llama/mod.rs` - Add `grammar` module export
- `tsuku-llm/src/llama/sampler.rs` - Replace custom sampler with llama.cpp's sampler chain API for grammar support
- `tsuku-llm/src/main.rs` - Integrate grammar-constrained sampling in the inference loop when tool schemas are provided

## Files to Create

- `tsuku-llm/src/llama/grammar.rs` - JSON Schema to GBNF converter and grammar sampler wrapper

## Implementation Steps

- [ ] **Step 1: Add grammar module and GBNF primitives**
  - Create `tsuku-llm/src/llama/grammar.rs` with GBNF string builder
  - Define base rules: `char`, `string`, `number`, `boolean`, `null`, `ws` (matching json.gbnf)
  - Add tests for primitive rule generation

- [ ] **Step 2: Implement JSON Schema to GBNF conversion**
  - Parse `map[string]any` JSON Schema (passed via gRPC as JSON string)
  - Handle `type: "object"` with `properties` and `required`
  - Handle `type: "array"` with `items`
  - Handle primitives: `string`, `number`, `boolean`
  - Generate property-specific rules (e.g., `path-kv ::= "\"path\"" ws ":" ws string`)
  - Generate root rule that enforces required properties

- [ ] **Step 3: Add grammar sampler FFI bindings**
  - Verify `llama_sampler_init_grammar` is exposed in bindings
  - Add safe Rust wrapper: `GrammarSampler::new(vocab, grammar_str, root_rule) -> Option<Self>`
  - Implement Drop to call `llama_sampler_free`

- [ ] **Step 4: Integrate sampler chain into inference loop**
  - Create sampler chain with `llama_sampler_chain_init`
  - Add grammar sampler for constrained generation
  - Add greedy/dist sampler for token selection
  - Replace manual `Sampler::sample()` with `llama_sampler_sample()`

- [ ] **Step 5: Wire grammar to gRPC Complete endpoint**
  - Accept tool schema in `CompletionRequest` (already has `tools` field)
  - Convert first tool's schema to GBNF grammar
  - Create grammar sampler for that request
  - Use greedy sampling (temperature=0, already default)

- [ ] **Step 6: Add tests for all three tool schemas**
  - Test `fetch_file` schema: `{"path": string}` required
  - Test `inspect_archive` schema: `{"url": string}` required
  - Test `extract_pattern` schema: nested objects with array of objects

## Testing Strategy

### Unit Tests
- `grammar.rs`: Test GBNF generation for each JSON Schema type (object, array, string, number)
- `grammar.rs`: Test required vs optional property handling
- `grammar.rs`: Test nested object schema generation (extract_pattern)
- `sampler.rs`: Test grammar sampler creation (with mock grammar string)

### Integration Tests
- Generate grammar for each tsuku tool schema and verify it parses (llama_sampler_init_grammar returns non-NULL)
- Verify generated JSON validates against original JSON Schema (round-trip test)

### Manual Verification
- Load model and test constrained generation with simple schema
- Verify `extract_pattern` complex output is valid JSON matching schema

## Risks and Mitigations

- **Performance**: Complex grammars can slow sampling (documented in llama.cpp #4218). Mitigation: tsuku tool schemas are simple (no deep nesting), and we use temperature 0 (greedy) which is faster.

- **Grammar parse failures**: `llama_sampler_init_grammar` returns NULL on invalid GBNF. Mitigation: Extensive unit tests for GBNF generation; fallback to unconstrained generation with warning if grammar fails.

- **Binding gaps**: Grammar functions may not be fully exposed in bindgen output. Mitigation: Check generated bindings early; add allowlist entries to `build.rs` if needed.

- **Sampler chain complexity**: Switching from manual sampling to sampler chain is a significant refactor. Mitigation: Keep existing `Sampler` working until chain is verified; make switch atomic.

## Success Criteria

- [ ] `fetch_file` tool schema generates valid GBNF that constrains output to `{"path": "<string>"}`
- [ ] `inspect_archive` tool schema generates valid GBNF that constrains output to `{"url": "<string>"}`
- [ ] `extract_pattern` tool schema generates valid GBNF for nested structure with array
- [ ] Grammar sampler integrates with inference loop without breaking existing tests
- [ ] All 48 existing Rust tests continue passing
- [ ] New grammar tests provide >80% coverage of schema combinations

## Open Questions

None - the implementation approach is clear from the existing codebase and llama.cpp documentation.

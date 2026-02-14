---
summary:
  constraints:
    - Grammar must force valid JSON matching tool schemas (not just any JSON)
    - Temperature 0 for deterministic extraction (already default in sampler)
    - Grammar constraint timeout: 30-second limit per turn
    - Must support nested objects, arrays, required/optional properties
    - No additionalProperties by default (llama.cpp defaults to false)
  integration_points:
    - src/llama/mod.rs - Add grammar module export
    - src/llama/sampler.rs - Integrate grammar-constrained sampling via llama_sampler_init_grammar
    - src/main.rs - Apply grammar during Complete RPC when tools/schema provided
    - internal/llm/tools.go - Tool schemas to convert (fetch_file, inspect_archive, extract_pattern)
  risks:
    - Performance: Complex grammars can cause slow sampling per llama.cpp docs
    - Correctness: Must handle all tsuku tool schemas (nested objects, arrays)
    - API: llama_sampler_init_grammar returns NULL on invalid grammar string
  approach_notes: |
    llama.cpp has built-in JSON Schema to GBNF conversion, but that's only in Python/JS.
    The C API only takes raw GBNF strings via llama_sampler_init_grammar().

    Implementation approach:
    1. Create a Rust JSON Schema to GBNF converter (src/llama/grammar.rs)
    2. Support the subset of JSON Schema used by tsuku tools: object, array, string, required
    3. Use llama_sampler_init_grammar() to create grammar-constrained sampler
    4. Integrate grammar sampler into the inference loop

    Key insight: The grammar sampler can be layered on top of existing sampling.
    It constrains token selection to only valid grammar continuations.
---

# Implementation Context: Issue #1639

**Source**: docs/designs/DESIGN-local-llm-runtime.md (Phase 4: Model Manager and Inference)

## Key Design Points

### GBNF Grammar Generation
- Generate GBNF from JSON Schema to constrain output to valid tool call JSON
- Support: objects with properties, arrays with item schemas, primitives, required/optional
- Temperature 0 for deterministic extraction

### llama.cpp API
- `llama_sampler_init_grammar(vocab, grammar_str, grammar_root)` - Creates grammar sampler
- Returns NULL on parse failure
- Grammar sampler modifies logits to only allow valid grammar tokens

### Tool Schemas to Support
1. `fetch_file`: `{path: string}`
2. `inspect_archive`: `{url: string}`
3. `extract_pattern`: Complex nested structure with arrays of objects

### Example Grammar Structure
```gbnf
root ::= "{" ws members ws "}"
members ::= pair ("," ws pair)*
pair ::= string ws ":" ws value
value ::= string | number | "true" | "false" | "null" | object | array
string ::= "\"" ([^"\\] | "\\" .)* "\""
```

For constrained schemas, generate specific rules like:
```gbnf
root ::= "{" ws path-kv ws "}"
path-kv ::= "\"path\"" ws ":" ws string
```

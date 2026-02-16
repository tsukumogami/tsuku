# LLM Quality Expectations

This document defines acceptance thresholds for local LLM recipe generation
and documents performance characteristics across hardware profiles.

## Quality Thresholds

### Minimum Acceptable Pass Rate

The local model must achieve at least **85%** on the benchmark test matrix.
Pass rate means the provider produces a recipe that passes validation
(calls `extract_pattern` with valid mappings within the allowed turn limit).

### Maximum Repair Turns

Average repair turns across passing test cases must be below **2.0**.
A repair turn is one additional provider call after the first attempt.
Zero repair turns means the provider got it right on the first try.

### First Try Rate

At least **60%** of test cases should pass on the first attempt (zero repair turns).
This indicates the model understands the pattern without needing extra context.

### Cloud Parity

The local model's pass rate should be within **10 percentage points** of the
Claude baseline. If Claude achieves 95%, the local model should achieve at least 85%.

## Hardware Profiles

Expected inference latency per turn (single completion request):

| Profile | GPU | VRAM | Expected Model | Latency/Turn |
|---------|-----|------|----------------|--------------|
| High-end | NVIDIA RTX 4090 | 24GB | Qwen 2.5 3B Q4 | 2-5s |
| Mid-range | Apple M2 | 16GB unified | Qwen 2.5 3B Q4 | 3-8s |
| Entry GPU | NVIDIA GTX 1060 | 6GB | Qwen 2.5 1.5B Q4 | 5-10s |
| CPU-only (16GB) | None | - | Qwen 2.5 1.5B Q4 | 10-20s |
| CPU-only (8GB) | None | - | Qwen 2.5 0.5B Q4 | 15-30s |

Total recipe generation time (3-5 turns) scales linearly with per-turn latency.
A recipe on an M2 laptop should complete in 10-40 seconds. On CPU-only systems,
expect 30-150 seconds.

## Latency Budgets

| Metric | Acceptable | Degraded | Unacceptable |
|--------|-----------|----------|--------------|
| P50 per turn (GPU) | <5s | 5-15s | >15s |
| P50 per turn (CPU) | <20s | 20-45s | >45s |
| P99 per turn (GPU) | <15s | 15-30s | >30s |
| P99 per turn (CPU) | <45s | 45-90s | >90s |
| Total recipe (GPU) | <30s | 30-60s | >60s |
| Total recipe (CPU) | <120s | 120-300s | >300s |

## Known Limitations

Patterns where local models may underperform compared to cloud providers:

1. **Unusual archive layouts**: Assets with deeply nested executables or
   non-standard directory structures may confuse smaller models.

2. **Custom naming conventions**: Tools that use entirely non-standard OS or
   architecture names (e.g., "64bit" instead of "amd64") are harder for
   small models to map correctly.

3. **Multi-binary installations**: Tools that install multiple executables
   (like JDKs with java, javac, jar) require the model to identify all
   binaries. Smaller models may miss some.

4. **Complex source builds**: Homebrew source build recipes with multiple
   patches and build steps push the context window limits of small models.

5. **Ambiguous assets**: Repositories with many similarly named assets
   (e.g., separate release assets for musl and glibc) require careful
   disambiguation that larger models handle better.

## Running the Benchmark

```bash
# Infrastructure verification (mock providers, fast)
go test -v -run TestRecipeQualityBenchmark ./internal/llm/

# Real provider benchmark (requires API keys)
LLM_BENCHMARK=true go test -v -run TestRecipeQualityBenchmark ./internal/llm/

# Custom output directory
LLM_BENCHMARK=true LLM_BENCHMARK_OUTPUT=./results \
  go test -v -run TestRecipeQualityBenchmark ./internal/llm/
```

The benchmark writes a JSON report to the output directory containing
per-test-case results and per-provider summaries. Use these reports
for regression detection in CI.

## Interpreting Results

A benchmark report contains:

- **results**: Individual test case outcomes with provider, pass/fail,
  repair turns, and latency.
- **summaries**: Aggregate statistics per provider including pass rate,
  first try rate, average repair turns, and latency percentiles.

When comparing providers:
- **pass_rate**: Higher is better. Local should be within 10% of Claude.
- **first_try_rate**: Higher means fewer wasted turns and faster generation.
- **avg_repair_turns**: Lower is better. Below 2.0 is acceptable.
- **latency_p50**: The typical user experience. Compare against hardware profile table.
- **latency_p99**: Worst-case experience. Should stay within "degraded" range.

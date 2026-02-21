# LLM Manual Test Runbook

CI handles automated quality checks for the local LLM runtime: the `llm-quality` job runs the ground truth suite on every prompt or test matrix change, and `TestSequentialInference` / `TestCrashRecovery` exercise short stability scenarios. But some failure modes only surface under long-running workloads or specific hardware configurations. These three procedures cover what CI can't.

## Prerequisites

All procedures require a built `tsuku-llm` addon binary.

```bash
cd tsuku-llm && cargo build --release
export TSUKU_LLM_BINARY="$(pwd)/target/release/tsuku-llm"
```

Verify the binary works:

```bash
$TSUKU_LLM_BINARY --version
```

## Procedure 1: Full Benchmark

Reproduces a 10-case QA run with fresh server restarts between cases. This is the scenario that originally exposed gRPC transport errors and server crashes in [#1738](https://github.com/tsukumogami/tsuku/issues/1738).

### Steps

1. Build the addon (see Prerequisites above).

2. Run the ground truth suite, one case at a time, restarting the server between each. The `TSUKU_LLM_BINARY` env var tells the test to use the local provider:

   ```bash
   export TSUKU_LLM_BINARY="$(pwd)/tsuku-llm/target/release/tsuku-llm"

   for case in stern ast-grep trivy fly liberica age fzf gh delta ripgrep; do
     echo "=== Running $case ==="
     go test -tags=integration -run "TestLLMGroundTruth/.*_${case}$" \
       -timeout 10m -v ./internal/builders/ 2>&1 | tee "/tmp/llm-bench-${case}.log"
     echo "=== Finished $case ==="
     sleep 10  # cooldown before next case
   done
   ```

   The 10-second cooldown lets the server shut down cleanly between cases.

3. Record results in the table below.

### Results Template

Copy this table and fill it in after each run.

| Case | Duration (s) | Result | Error (if any) |
|------|-------------|--------|----------------|
| stern | | | |
| ast-grep | | | |
| trivy | | | |
| fly | | | |
| liberica | | | |
| age | | | |
| fzf | | | |
| gh | | | |
| delta | | | |
| ripgrep | | | |

### Recording Fields

Include these with every run:

- **Tester**: Who ran the benchmark
- **Date**: When it was run
- **Model**: Which GGUF model was used (check `$TSUKU_HOME/models/`)
- **Hardware**: CPU, RAM, GPU (if any)
- **Addon version**: Output of `$TSUKU_LLM_BINARY --version`

### Success Criteria

All 10 cases complete without gRPC transport errors. Individual test failures (wrong recipe output) are acceptable and tracked by the quality baseline system. The benchmark is checking server stability, not recipe correctness.

## Procedure 2: Soak Test

Detects memory leaks by running 20+ sequential inference requests through a single warm server. Unlike the benchmark above, the server stays alive the entire time.

### Steps

1. Build the addon and start the server:

   ```bash
   export TSUKU_LLM_BINARY="$(pwd)/tsuku-llm/target/release/tsuku-llm"
   $TSUKU_LLM_BINARY serve &
   ```

2. Find the server PID:

   ```bash
   SERVER_PID=$!
   echo "Server PID: $SERVER_PID"
   ```

3. Record the baseline memory before sending any inference requests:

   ```bash
   # Linux
   grep VmRSS /proc/$SERVER_PID/status

   # Cross-platform alternative
   ps -p $SERVER_PID -o rss=
   ```

4. Run 25+ sequential requests. Use the stability test or send requests manually through the ground truth suite without restarting:

   ```bash
   for i in $(seq 1 25); do
     echo "=== Request $i ==="
     go test -tags=integration -run "TestLLMGroundTruth/.*_stern$" \
       -timeout 5m -count=1 -v ./internal/builders/ 2>&1 | tail -5

     # Record memory every 5 requests
     if [ $((i % 5)) -eq 0 ]; then
       echo "VmRSS at request $i:"
       grep VmRSS /proc/$SERVER_PID/status
     fi
   done
   ```

5. Record memory at the end:

   ```bash
   grep VmRSS /proc/$SERVER_PID/status
   kill $SERVER_PID
   ```

### Results Template

| Request # | VmRSS (kB) | Delta from baseline |
|-----------|-----------|---------------------|
| 0 (baseline) | | -- |
| 5 | | |
| 10 | | |
| 15 | | |
| 20 | | |
| 25 | | |

### Interpretation

- **< 5% growth over 25 requests**: No leak detected. Memory is stable.
- **Linear growth**: Likely a KV cache leak or gRPC buffer accumulation. Each request adds memory that isn't freed. File a bug with the memory readings.
- **Step-function growth** (sudden jumps then flat): Possible buffer pool expansion. Check if the jumps correlate with specific test cases or request sizes.

## Procedure 3: New Model Validation

Use this when evaluating a model change (different GGUF quantization, new base model, updated model manifest).

### Steps

1. Run the full ground truth suite with `-update-baseline` to capture results for the new model:

   ```bash
   export TSUKU_LLM_BINARY="$(pwd)/tsuku-llm/target/release/tsuku-llm"
   go test -tags=integration -run TestLLMGroundTruth \
     -timeout 30m -v -update-baseline ./internal/builders/
   ```

   This writes results to `testdata/llm-quality-baselines/local.json`.

2. Compare the new baseline against the previous one:

   ```bash
   git diff testdata/llm-quality-baselines/local.json
   ```

3. Record the comparison in the table below.

4. Decision:
   - If the new model matches or improves on the previous baseline, accept the change and commit the updated baseline.
   - If there are regressions, decide whether they're acceptable for the model's other benefits (speed, size, etc.). Document the trade-off in the PR description.
   - To reject: `git checkout testdata/llm-quality-baselines/local.json` and revert the model change.

### Quality Comparison Template

| Case | Previous Result | New Result | Change |
|------|----------------|------------|--------|
| stern | | | |
| ast-grep | | | |
| trivy | | | |
| fly | | | |
| liberica | | | |
| age | | | |
| fzf | | | |
| gh | | | |
| delta | | | |
| ripgrep | | | |

**Previous model**: (name and quantization)
**New model**: (name and quantization)
**Pass rate change**: X/10 -> Y/10

## Memory Monitoring Reference

### Linux: `/proc/<pid>/status`

The most direct way to check process memory on Linux:

```bash
grep -E 'VmRSS|VmSize|VmPeak' /proc/<pid>/status
```

- **VmRSS**: Resident Set Size. Physical memory currently in use. This is the number to watch for leaks.
- **VmSize**: Virtual memory allocated. Includes mapped files and reserved-but-unused pages. Often large and not a concern by itself.
- **VmPeak**: Peak virtual memory. Useful for finding high-water marks.

### Cross-platform: `ps`

Works on both Linux and macOS:

```bash
ps -p <pid> -o pid,rss,vsz,comm
```

The `rss` column is in kilobytes and corresponds to VmRSS.

### macOS

macOS doesn't have `/proc`. Use `ps` as shown above, or for more detail:

```bash
# Detailed memory breakdown
vmmap <pid> | grep "Physical footprint"
```

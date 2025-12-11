# Benchmark Harness

Measures LLM recipe generation success rate against a corpus of GitHub repositories.

## Usage

```bash
# Run full benchmark (expensive - uses LLM credits)
go run ./cmd/benchmark --corpus testdata/benchmark-repos.txt

# Run subset for quick validation
go run ./cmd/benchmark --corpus testdata/benchmark-repos.txt --limit 5
```

## Corpus File Format

The corpus file contains one repository per line in `owner/repo` format:

```
# Comments start with #
cli/cli
BurntSushi/ripgrep
sharkdp/bat
```

## Requirements

- `ANTHROPIC_API_KEY` or `GOOGLE_API_KEY` environment variable
- `GITHUB_TOKEN` (optional, for higher rate limits)
- Docker (for container validation)

## Pre-Release Gate

Run the benchmark before releases to verify the 80% success rate threshold:

```bash
go run ./cmd/benchmark --corpus testdata/benchmark-repos.txt
```

The harness exits with code 1 if success rate is below 80%.

## Output Format

```
Benchmark Results:
  Total: 25
  Passed: 21
  Failed: 4
  Success Rate: 84%

Failed repositories:
  - owner/repo1: validation failed (binary not found)
  - owner/repo2: repair exhausted (3 attempts)
```

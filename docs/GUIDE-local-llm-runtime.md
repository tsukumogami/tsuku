# Local LLM Runtime Guide

This guide covers tsuku's local LLM inference runtime, which enables recipe generation without cloud API keys.

## Overview

tsuku can generate recipes from complex sources like GitHub releases and Homebrew formulas using LLM analysis. By default, this runs locally on your machine through a small addon binary (`tsuku-llm`) that bundles an open-source model. No accounts, API keys, or billing setup required.

The addon communicates with tsuku over localhost gRPC, starts on demand when inference is needed, and shuts down after an idle timeout. Hardware detection picks the best model for your system automatically.

Cloud providers (Claude, Gemini) remain available as alternatives. When cloud API keys are configured, they take priority over local inference.

## Hardware Requirements

The addon detects your hardware at startup and selects the largest model that fits. Here's what to expect:

| Available Resources | Model Selected | Download Size | Expected Quality |
|---------------------|---------------|---------------|-----------------|
| 8GB+ VRAM (CUDA/Metal) | Qwen 2.5 3B Q4 | ~2.5 GB | Near-cloud quality |
| 4-8GB VRAM | Qwen 2.5 1.5B Q4 | ~1.5 GB | Good |
| CPU only, 16GB+ RAM | Qwen 2.5 3B Q4 | ~2.5 GB | Good, slower |
| CPU only, 8-16GB RAM | Qwen 2.5 1.5B Q4 | ~1.5 GB | Good, slow |
| CPU only, 4-8GB RAM | Qwen 2.5 0.5B Q4 | ~500 MB | Adequate |
| Less than 4GB RAM | Local inference disabled | -- | -- |

GPU acceleration is used when available:
- **NVIDIA**: CUDA (automatic detection)
- **Apple Silicon**: Metal (automatic detection)
- **AMD/Intel**: Vulkan (automatic detection)
- **No GPU**: CPU fallback with AVX2/AVX-512 optimizations

Inference speed varies. GPU inference typically takes 5-15 seconds per turn, while CPU-only runs 10-30 seconds. A single recipe generation involves 3-5 inference turns.

## Configuration

All local LLM settings live in the `[llm]` section of `$TSUKU_HOME/config.toml`. Every option has a sensible default, so you don't need to configure anything for typical use.

```toml
[llm]
# Enable or disable local inference entirely.
# When false, the addon isn't downloaded and LocalProvider isn't registered.
# Default: true
local_enabled = true

# Start the addon server early in tsuku create to hide model loading latency.
# When false, the server starts only when inference is first needed.
# Default: true
local_preemptive = true

# Override automatic model selection (optional).
# Normally the addon picks based on your hardware. Set this if auto-detection
# picks the wrong model, or you want to force a specific size.
# local_model = "qwen2.5-1.5b-instruct-q4"

# Override automatic GPU backend selection (optional).
# Values: "cuda", "metal", "vulkan", "cpu"
# local_backend = "cuda"

# How long the addon server stays alive after the last inference request.
# Shorter values free resources sooner. Longer values help batch workflows.
# Default: "5m"
idle_timeout = "5m"
```

### Configuration via `tsuku config`

You can also read and write these settings with the CLI:

```bash
# View current settings
tsuku config get llm.local_enabled
tsuku config get llm.idle_timeout

# Change settings
tsuku config set llm.local_preemptive false
tsuku config set llm.idle_timeout 10m
```

### Environment Variable Override

The `TSUKU_LLM_IDLE_TIMEOUT` environment variable overrides the `idle_timeout` config value. It accepts Go duration strings (`30s`, `5m`, `10m`).

```bash
# Short timeout for testing
TSUKU_LLM_IDLE_TIMEOUT=10s tsuku create gh --from github:cli/cli

# Long timeout for batch processing
TSUKU_LLM_IDLE_TIMEOUT=30m ./batch-create.sh
```

## Pre-Downloading

The `tsuku llm download` command downloads the addon binary and model ahead of time. This is useful for CI pipelines, offline environments, or slow connections where you'd rather download once upfront.

```bash
# Interactive download with hardware auto-detection
tsuku llm download

# Auto-approve for CI (skips confirmation prompts)
tsuku llm download --yes

# Force a specific model instead of auto-detection
tsuku llm download --model qwen2.5-3b-instruct-q4

# Re-download even if already cached
tsuku llm download --force
```

The addon binary is ~50MB. Model size depends on hardware detection (see the table above).

## How It Works

When you run `tsuku create` with an LLM-powered builder and no cloud API keys are set:

1. The provider factory falls through to `LocalProvider`
2. If this is the first use, tsuku prompts to download the addon (~50MB)
3. With `local_preemptive = true` (default), the addon server starts early while tsuku fetches metadata in parallel
4. The addon detects your hardware, selects a model, and downloads it if needed
5. Inference runs over localhost gRPC with grammar constraints that force valid JSON output
6. After `tsuku create` finishes, the server stays alive for the idle timeout period
7. Subsequent `tsuku create` calls within that window reuse the warm server

The addon stores its files in `$TSUKU_HOME`:
- Binary: `$TSUKU_HOME/tools/tsuku-llm/`
- Models: `$TSUKU_HOME/models/`
- Socket: `$TSUKU_HOME/llm.sock`
- Lock: `$TSUKU_HOME/llm.sock.lock`

## Troubleshooting

### Addon server won't start

**Stale socket file**: If tsuku crashed or the addon was killed without cleanup, a stale socket file may remain. tsuku detects this using a lock file and cleans up automatically. If it persists:

```bash
rm $TSUKU_HOME/llm.sock $TSUKU_HOME/llm.sock.lock
```

**Port or socket in use**: The addon uses a Unix domain socket, not a TCP port. If you see socket errors, check whether another addon instance is already running:

```bash
ls -la $TSUKU_HOME/llm.sock
```

### Model selection seems wrong

The addon picks models based on detected hardware. If it picks too small a model (or too large), override it:

```bash
# Check what model was selected
tsuku llm download  # shows detected hardware and selected model

# Override in config
tsuku config set llm.local_model qwen2.5-3b-instruct-q4
```

Or set `local_model` in `$TSUKU_HOME/config.toml` directly.

### Out of memory during inference

The addon tries to pick a model that fits your available resources. If you still get OOM errors:

1. Let auto-detection pick a smaller model by not setting `local_model`
2. Force a smaller model: `tsuku config set llm.local_model qwen2.5-0.5b-instruct-q4`
3. Switch to a cloud provider if your hardware can't handle local inference

Systems with less than 4GB RAM can't run local inference. Configure a cloud provider instead:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
# or
export GOOGLE_API_KEY="AIza..."
```

### Slow inference on CPU

CPU-only inference is slower than GPU (10-30 seconds per turn vs 5-15 seconds). This is expected. To speed things up:

- If you have a discrete GPU, make sure the correct backend is detected. Check with `tsuku llm download` and override if needed: `tsuku config set llm.local_backend cuda`
- Use a smaller model: `tsuku config set llm.local_model qwen2.5-0.5b-instruct-q4`
- Use a cloud provider for faster results

### Disabling local inference

If you don't want local inference at all (you only use cloud providers, or you don't use LLM features):

```bash
tsuku config set llm.local_enabled false
```

This prevents the addon from being downloaded and removes all download prompts. LLM-powered builders will require a cloud API key.

### Addon download fails

The addon binary is downloaded from tsuku's CDN with SHA256 checksum verification. If downloads fail:

1. Check your internet connection
2. Try again (tsuku retries with exponential backoff)
3. If behind a proxy, make sure `HTTPS_PROXY` is set
4. As a workaround, configure a cloud provider while the issue is resolved

## See Also

- [Environment Variables](ENVIRONMENT.md) - `TSUKU_LLM_IDLE_TIMEOUT` reference
- [README - LLM-Powered Recipe Generation](../README.md#llm-powered-recipe-generation) - Quick start

# Local LLM Guide

tsuku can generate recipes using a small language model running on your machine. No cloud API keys, no accounts, no billing setup. This guide covers how local inference works, what hardware you need, and how to configure it.

## How It Works

When you run `tsuku create <tool> --from github:owner/repo` without cloud API keys configured, tsuku falls through to its local inference provider. Here's what happens behind the scenes:

1. **Addon check.** tsuku looks for the `tsuku-llm` binary in `$TSUKU_HOME/tools/`. If it's not installed, tsuku prompts you to download it (~50 MB).
2. **Server startup.** tsuku starts `tsuku-llm serve` as a background process. The server binds to a Unix domain socket at `$TSUKU_HOME/llm.sock`.
3. **Hardware detection.** The addon detects your GPU (CUDA, Metal, or Vulkan), available VRAM, system RAM, and CPU features. It picks the largest model that fits your hardware.
4. **Model download.** If the selected model isn't cached yet, the addon downloads it (0.5-2.5 GB depending on selection). You're prompted before the download starts.
5. **Inference.** tsuku sends requests to the addon over gRPC. The addon constrains output to valid JSON using grammar rules, same as cloud providers.
6. **Idle shutdown.** After 5 minutes with no requests, the server shuts itself down. If you run `tsuku create` again within that window, the server is already warm.

The addon is a separate Rust binary that bundles llama.cpp. tsuku's core Go binary stays lightweight -- it only contains a gRPC client for talking to the addon.

## Hardware Requirements

The addon detects your hardware at startup and selects a model automatically. You don't need to pick one yourself.

| Available Resources | Model | Download Size | Expected Quality |
|---------------------|-------|---------------|------------------|
| 8 GB+ VRAM (CUDA/Metal) | Qwen 2.5 3B Q4 | ~2.5 GB | Near-cloud |
| 4-8 GB VRAM | Qwen 2.5 1.5B Q4 | ~1.5 GB | Good |
| CPU only, 8 GB+ RAM | Qwen 2.5 1.5B Q4 | ~1.5 GB | Good (slower) |
| CPU only, < 8 GB RAM | Qwen 2.5 0.5B Q4 | ~500 MB | Adequate |
| < 4 GB RAM | Disabled | -- | -- |

GPU inference is noticeably faster (a few seconds per turn vs. 5-30 seconds on CPU). If you have an NVIDIA or Apple Silicon GPU, the addon uses it automatically.

Systems with less than 4 GB of RAM can't run local inference. Configure a cloud provider instead (see [Cloud Providers](#falling-back-to-cloud-providers) below).

## Configuration

All configuration lives in the `[llm]` section of `$TSUKU_HOME/config.toml`. Every option has a sensible default, so most users don't need to change anything.

### Options Reference

```toml
[llm]
# Master switch for local inference. Set to false to prevent the addon
# from being downloaded or registered. Default: true
local_enabled = true

# Start the addon server early during tsuku create to hide model loading
# latency. When false, the server starts only when inference is first
# needed. Default: true
local_preemptive = true

# Override automatic model selection. Leave unset for auto-detection.
# local_model = "qwen2.5-1.5b-instruct-q4"

# Override automatic GPU backend. Leave unset for auto-detection.
# Valid values: "cpu"
# local_backend = "cpu"

# How long the addon server stays alive after the last request.
# Default: 5m
idle_timeout = "5m"
```

Use `tsuku config set` and `tsuku config get` to modify these values:

```bash
# Disable local inference entirely
tsuku config set llm.local_enabled false

# Force CPU backend
tsuku config set llm.backend cpu

# Check current idle timeout
tsuku config get llm.idle_timeout
```

### Environment Variable Override

The `TSUKU_LLM_IDLE_TIMEOUT` environment variable overrides the config file's `idle_timeout` value. It accepts Go duration strings like `30s`, `5m`, or `10m`.

```bash
# Short timeout for testing
TSUKU_LLM_IDLE_TIMEOUT=10s tsuku create ripgrep --from github:BurntSushi/ripgrep
```

## Pre-Downloading for CI and Offline Use

The `tsuku llm download` command downloads the addon binary and model ahead of time so that `tsuku create` doesn't pause for downloads later.

```bash
# Interactive -- prompts before each download
tsuku llm download

# Skip all prompts (for CI scripts)
tsuku llm download --yes
```

The command:

1. Ensures the addon binary is installed (downloads if missing)
2. Starts the addon server to detect hardware
3. Shows the selected model and backend
4. Downloads the model if not already present
5. Verifies everything is ready

For CI pipelines that generate many recipes, run `tsuku llm download --yes` in your setup step. The server's idle timeout keeps the model loaded between `tsuku create` calls, so a batch of hundreds of tools only loads the model once.

### Example CI Setup

```bash
# Install tsuku
curl -fsSL https://get.tsuku.dev/now | bash

# Pre-download addon and model (no prompts)
tsuku llm download --yes

# Generate recipes in a loop -- server stays warm between calls
for tool in $(cat tools.txt); do
  tsuku create "$tool" --from "github:$tool"
done
```

## First-Use Experience

The first time tsuku needs local inference, you'll see two download prompts:

```
Local LLM requires downloading tsuku-llm inference addon (50.0 MB).
Continue? [Y/n]
```

After the addon installs and starts, it checks whether a model is cached:

```
Local LLM requires downloading LLM model (Qwen 2.5 1.5B Q4) (1.5 GB).
Continue? [Y/n]
```

Press Enter (or type `y`) to proceed. Both downloads happen once and are cached for future use. In non-interactive environments (piped input, CI without a TTY), the prompts are declined automatically. Use `tsuku llm download --yes` to handle those cases.

After download, you'll see a spinner during inference:

```
Generating...
```

## Troubleshooting

### Download was declined or failed

If you declined a download prompt or it failed mid-transfer, run:

```bash
tsuku llm download
```

This retries the download. If the addon is already present but the model isn't, it skips straight to the model download.

### "no LLM providers available" error

This means tsuku couldn't find any working provider. Check:

- **Local inference disabled?** Run `tsuku config get llm.local_enabled`. If it's `false`, either set it to `true` or configure a cloud API key.
- **Insufficient hardware?** Systems with less than 4 GB RAM can't run local inference. Set a cloud API key instead.

### Addon server won't start

If the addon crashes at startup:

```bash
# Check if a stale socket exists
ls -la $TSUKU_HOME/llm.sock

# Remove it manually if the server isn't running
rm $TSUKU_HOME/llm.sock $TSUKU_HOME/llm.sock.lock
```

The server lifecycle manager normally handles stale socket cleanup, but manual removal works if something goes wrong.

### Slow inference on CPU

CPU-only inference takes 5-30 seconds per turn depending on your CPU and model size. This is expected. If you have a discrete GPU that isn't being detected, check that the appropriate drivers are installed. The addon logs the detected backend at startup.

To speed things up:

- Install GPU drivers if you have discrete graphics
- Use a smaller model: `tsuku config set llm.local_model qwen2.5-0.5b-instruct-q4`
- Switch to a cloud provider for faster results

### Falling Back to Cloud Providers

If local inference doesn't work for your setup, cloud providers are always available:

```bash
# Claude (Anthropic)
export ANTHROPIC_API_KEY="sk-ant-..."

# Or Gemini (Google)
export GOOGLE_API_KEY="AIza..."

# Or store in config
tsuku config set secrets.anthropic_api_key
```

When a cloud API key is configured, it takes priority over local inference. Cloud providers are faster and produce higher quality results on unusual release layouts, but they require an account and cost ~$0.02-0.15 per recipe.

You can also disable local inference entirely if you only want cloud:

```bash
tsuku config set llm.local_enabled false
```

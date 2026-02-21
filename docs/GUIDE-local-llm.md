# Local LLM Guide

tsuku can generate recipes using a small language model running on your machine. No cloud API keys, no accounts, no billing setup. This guide covers how local inference works, what hardware you need, and how to configure it.

## How It Works

When you run `tsuku create <tool> --from github:owner/repo` without cloud API keys configured, tsuku falls through to its local inference provider. Here's what happens behind the scenes:

1. **Addon check.** tsuku looks for the `tsuku-llm` binary in `$TSUKU_HOME/tools/`. If it's not installed, tsuku prompts you to download it (~50 MB).
2. **Server startup.** tsuku starts `tsuku-llm serve` as a background process. The server binds to a Unix domain socket at `$TSUKU_HOME/llm.sock`.
3. **Hardware detection.** The addon detects your GPU (CUDA, Metal, or Vulkan), available VRAM, and CPU features. It picks the largest model that fits your hardware. A GPU with 8 GB+ VRAM is required.
4. **Model download.** If the selected model isn't cached yet, the addon downloads it (4.9-9.1 GB depending on selection). You're prompted before the download starts.
5. **Inference.** tsuku sends requests to the addon over gRPC. The addon constrains output to valid JSON using grammar rules, same as cloud providers.
6. **Idle shutdown.** After 5 minutes with no requests, the server shuts itself down. If you run `tsuku create` again within that window, the server is already warm.

The addon is a separate Rust binary that bundles llama.cpp. tsuku's core Go binary stays lightweight -- it only contains a gRPC client for talking to the addon.

## Hardware Requirements

A GPU with at least 8 GB VRAM is required. CPU-only inference isn't supported because models below 7B (the minimum for acceptable quality) are too slow on CPU to be practical.

The addon detects your GPU at startup and selects a model automatically. You don't need to pick one yourself.

| Available VRAM | Model | Download Size | Expected Quality |
|----------------|-------|---------------|------------------|
| 14 GB+ | Qwen 2.5 14B Q4 | ~9.1 GB (3 files) | Near-cloud |
| 8-14 GB | Qwen 2.5 7B Q4 | ~4.9 GB (2 files) | Good |
| < 8 GB or no GPU | Not supported | -- | -- |

Supported GPU backends, in priority order:

1. **CUDA** -- NVIDIA GPUs with installed drivers
2. **Metal** -- Apple Silicon Macs (unified memory; ~75% of system RAM counts as VRAM)
3. **Vulkan** -- AMD, Intel, or NVIDIA fallback

If your system doesn't have a supported GPU with 8 GB+ VRAM, configure a cloud provider instead (see [Cloud Providers](#falling-back-to-cloud-providers) below).

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

# Override auto-detected GPU backend for the tsuku-llm binary variant.
# Valid values: "cpu" (force CPU variant). Leave unset for auto-detection.
# backend = "cpu"

# How long the addon server stays alive after the last request.
# Default: 5m
idle_timeout = "5m"
```

Use `tsuku config set` and `tsuku config get` to modify these values:

```bash
# Disable local inference entirely
tsuku config set llm.local_enabled false

# Force CPU backend variant
tsuku config set llm.backend cpu

# Check current idle timeout
tsuku config get llm.idle_timeout
```

There's no config key for overriding model selection. The addon picks the best model for your GPU automatically.

### Environment Variable Overrides

The addon reads several environment variables that take precedence over config and auto-detection:

| Variable | Purpose | Example |
|----------|---------|---------|
| `TSUKU_LLM_MODEL` | Override model selection | `qwen2.5-7b-instruct-q4` |
| `TSUKU_LLM_BACKEND` | Override inference backend | `cuda`, `metal`, `vulkan` |
| `TSUKU_LLM_IDLE_TIMEOUT` | Override idle timeout | `30s`, `10m` |

```bash
# Short timeout for testing
TSUKU_LLM_IDLE_TIMEOUT=10s tsuku create ripgrep --from github:BurntSushi/ripgrep

# Force a specific model
TSUKU_LLM_MODEL=qwen2.5-7b-instruct-q4 tsuku create ripgrep --from github:BurntSushi/ripgrep
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
Local LLM requires downloading LLM model (Qwen 2.5 7B Q4) (4.9 GB).
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
- **No GPU or insufficient VRAM?** Local inference requires a GPU with 8 GB+ VRAM. If you don't have one, set a cloud API key instead.

### Addon server won't start

If the addon crashes at startup:

```bash
# Check if a stale socket exists
ls -la $TSUKU_HOME/llm.sock

# Remove it manually if the server isn't running
rm $TSUKU_HOME/llm.sock $TSUKU_HOME/llm.sock.lock
```

The server lifecycle manager normally handles stale socket cleanup, but manual removal works if something goes wrong.

### GPU not detected

If the addon reports "no GPU detected" even though you have a discrete GPU, check that the appropriate drivers are installed. The addon logs the detected backend at startup.

- **NVIDIA**: Install the NVIDIA driver (provides `libcuda.so` and `nvidia-smi`)
- **AMD/Intel**: Install Vulkan drivers (`libvulkan.so`)
- **Apple Silicon**: Metal is detected automatically; no driver install needed

If you can't get GPU acceleration working, switch to a cloud provider for now.

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

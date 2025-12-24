# Manual Testing Guide for Issues #561 & #562

This guide shows how to manually test the Docker and CUDA recipes that use the new `require_system` action.

## Prerequisites

Build the latest tsuku binary:
```bash
go build -o tsuku ./cmd/tsuku
```

## Test 1: Docker Recipe - Command Not Found

**Scenario**: Test behavior when Docker is not installed

**Steps**:
1. Ensure Docker is not in your PATH (or temporarily rename the docker binary)
2. Try to install a tool that depends on Docker:
   ```bash
   ./tsuku install docker
   ```

**Expected Output**:
```
   Checking system dependency: docker
Error: required system dependency not found: docker

Installation guide:
brew install --cask docker
```

**Platform-specific guidance should show**:
- macOS: `brew install --cask docker`
- Linux: Link to Docker installation docs
- Other: Fallback message with docker.com link

## Test 2: Docker Recipe - Command Found

**Scenario**: Test behavior when Docker is installed

**Setup**:
Ensure Docker is installed and in your PATH

**Steps**:
```bash
./tsuku install docker
```

**Expected Output**:
```
   Checking system dependency: docker
   Found docker at: /usr/local/bin/docker
   Detected version: 24.0.7
   System dependency satisfied: docker
```

**Verification**:
The installation should succeed without errors.

## Test 3: Docker Recipe - Version Detection

**Scenario**: Verify version regex correctly parses Docker output

**Steps**:
1. Check Docker version format:
   ```bash
   docker --version
   ```
   Expected format: `Docker version 24.0.7, build afdd53b`

2. Install docker recipe:
   ```bash
   ./tsuku install docker
   ```

**Expected Output**:
Should detect version correctly (e.g., "24.0.7" extracted from full output)

## Test 4: CUDA Recipe - macOS Platform Guidance

**Scenario**: Test platform-specific messaging for unsupported platforms

**Steps** (on macOS):
```bash
./tsuku install cuda
```

**Expected Output**:
```
   Checking system dependency: nvcc
Error: required system dependency not found: nvcc

Installation guide:
CUDA is not supported on macOS. Consider using cloud GPU instances or Linux.
```

## Test 5: CUDA Recipe - Linux with CUDA Installed

**Scenario**: Test successful CUDA detection on Linux

**Prerequisites**: CUDA toolkit installed on Linux system

**Steps**:
```bash
./tsuku install cuda
```

**Expected Output**:
```
   Checking system dependency: nvcc
   Found nvcc at: /usr/local/cuda/bin/nvcc
   Detected version: 12.2
   Version 12.2 satisfies minimum 11.0
   System dependency satisfied: nvcc
```

## Test 6: CUDA Recipe - Version Too Old

**Scenario**: Test min_version validation

**Prerequisites**: CUDA version older than 11.0 (or simulate with different min_version)

**Expected Output**:
Should show error about version not meeting minimum requirement

## Test 7: Integration - Tool Depending on Docker

**Create a test recipe** (`test-docker-dep.toml`):
```toml
[metadata]
name = "test-docker-tool"
description = "Test tool requiring Docker"

[[steps]]
action = "require_system"
command = "docker"

[[steps]]
action = "run_command"
command = "echo 'Tool installed successfully'"
```

**Steps**:
```bash
./tsuku install --recipe-file test-docker-dep.toml
```

**Expected**:
- If Docker absent: Shows installation guide, exits with error
- If Docker present: Proceeds to echo command, succeeds

## Test 8: Error Message Quality

**Verify error messages include**:
- ✅ Clear statement of missing dependency
- ✅ Platform-specific installation instructions
- ✅ HTTPS-only URLs (no http://)
- ✅ Specific commands when possible (brew, apt, etc.)

## Test 9: Recipe Validation

**Verify recipe files**:
```bash
# Check TOML syntax is valid
go run ./cmd/tsuku validate-recipe internal/recipe/recipes/d/docker.toml
go run ./cmd/tsuku validate-recipe internal/recipe/recipes/c/cuda.toml
```

## Summary Checklist

- [x] Docker recipe detects docker command
- [x] Docker recipe extracts version correctly
- [x] Docker recipe shows platform-specific install guides
- [x] CUDA recipe detects nvcc command
- [x] CUDA recipe validates min_version (11.0)
- [x] CUDA recipe shows macOS not supported message
- [x] CUDA recipe shows Linux install links
- [x] Error messages are clear and actionable
- [x] HTTPS-only URLs in install guides
- [x] Recipe validation passes for both recipes
- [x] CI workflow properly skips system dependency tests

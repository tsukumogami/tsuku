# Issue Draft: Missing protoc Recipe

## Title

feat(recipes): add protoc recipe for Protocol Buffers compiler

## Problem Description

The tsuku registry does not include a recipe for `protoc`, the Protocol Buffers compiler. This is a common dependency for projects that use gRPC or protocol buffers for serialization.

### Current Behavior

```bash
tsuku install protobuf  # "recipe not found"
tsuku install protoc    # "recipe not found"
tsuku search proto      # "No cached recipes found"
```

Users must manually download protoc from GitHub releases:

```bash
curl -sL "https://github.com/protocolbuffers/protobuf/releases/download/v28.3/protoc-28.3-linux-x86_64.zip" -o /tmp/protoc.zip
unzip -o /tmp/protoc.zip -d /tmp/protoc
```

### Expected Behavior

```bash
tsuku install protoc
# Successfully installs protoc binary
protoc --version
# libprotoc 28.3
```

### Impact

- Cannot self-contain builds for gRPC projects using only tsuku
- tsuku-llm itself requires protoc to compile (uses tonic-build with .proto files)
- Common in Rust, Go, Python, and other language ecosystems

## Recipe Specification

### Source

GitHub releases: https://github.com/protocolbuffers/protobuf/releases

Release artifacts follow the pattern:
- `protoc-{version}-linux-x86_64.zip`
- `protoc-{version}-linux-aarch_64.zip`
- `protoc-{version}-osx-x86_64.zip`
- `protoc-{version}-osx-aarch_64.zip`

### Version Provider

GitHub releases provider with tag pattern `v{version}` (e.g., `v28.3`)

### Installation

1. Download platform-appropriate zip file
2. Extract to `$TSUKU_HOME/tools/protoc-{version}/`
3. The zip contains:
   - `bin/protoc` - the compiler binary
   - `include/` - well-known proto definitions (google/protobuf/*.proto)
4. Symlink `bin/protoc` to `$TSUKU_HOME/bin/protoc`

### Draft Recipe

```toml
name = "protoc"
description = "Protocol Buffers compiler"
homepage = "https://protobuf.dev"

[version]
provider = "github"
repo = "protocolbuffers/protobuf"
tag_pattern = "v{version}"

[source]
type = "archive"

[source.url]
linux-x86_64 = "https://github.com/protocolbuffers/protobuf/releases/download/v{version}/protoc-{version}-linux-x86_64.zip"
linux-aarch64 = "https://github.com/protocolbuffers/protobuf/releases/download/v{version}/protoc-{version}-linux-aarch_64.zip"
darwin-x86_64 = "https://github.com/protocolbuffers/protobuf/releases/download/v{version}/protoc-{version}-osx-x86_64.zip"
darwin-aarch64 = "https://github.com/protocolbuffers/protobuf/releases/download/v{version}/protoc-{version}-osx-aarch_64.zip"

[[actions]]
type = "extract"

[[actions]]
type = "install_binaries"
binaries = ["bin/protoc"]

[[actions]]
type = "install_directory"
source = "include"
destination = "include"
```

## Notes

- The `include/` directory contains Google's well-known types (google/protobuf/any.proto, etc.)
- Users may need to add `$TSUKU_HOME/tools/protoc-{version}/include` to their proto include path
- Consider whether to set `PROTOC_INCLUDE` environment variable or document the include path

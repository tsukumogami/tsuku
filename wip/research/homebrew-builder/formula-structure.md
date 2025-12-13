# Homebrew Formula Structure Research

## Overview

This document provides a comprehensive analysis of Homebrew formula structure to inform the design of an LLM-based Homebrew builder for tsuku. The builder will parse Homebrew formulas (Ruby DSL) and generate tsuku recipes.

**Primary Sources:**
- [Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)
- [Homebrew Ruby API Documentation](https://rubydoc.brew.sh/Formula)
- [Homebrew Bottles Documentation](https://docs.brew.sh/Bottles)
- [Homebrew JSON API Documentation](https://formulae.brew.sh/docs/api/)

---

## 1. Formula Ruby DSL Structure

### 1.1 Basic Formula Anatomy

Homebrew formulas are Ruby classes that inherit from `Formula` and use a declarative DSL:

```ruby
class ToolName < Formula
  desc "Description of the tool"
  homepage "https://example.com"
  url "https://example.com/tool-1.0.tar.gz"
  sha256 "abc123..."
  license "MIT"

  bottle do
    # Platform-specific bottle definitions
  end

  depends_on "dependency"

  def install
    # Installation instructions
  end

  test do
    # Test code
  end
end
```

### 1.2 Key DSL Methods

#### Metadata Methods

- **`desc`**: Short description of the tool
- **`homepage`**: Project homepage URL
- **`license`**: SPDX license identifier (e.g., "MIT", "GPL-2.0-only", "Apache-2.0")
- **`revision`**: Forced-recompile version when formula changes without version bump

#### Version and Source Methods

- **`url`**: Download URL for the source archive
  - Supports `.tar.gz`, `.tar.bz2`, `.tar.xz`, `.zip`, etc.
  - Can include Git URLs with `branch:` parameter
- **`sha256`**: SHA-256 checksum of the source archive
- **`version`**: Explicitly specified version (usually auto-detected from URL)
- **`head`**: Optional block for development version from Git
  ```ruby
  head do
    url "https://github.com/example/tool.git", branch: "master"
    depends_on "autoconf" => :build
  end
  ```

#### Stable/Head Blocks

- **`stable do ... end`**: Encloses stable version-specific configuration
- **`head do ... end`**: Encloses development version configuration

Example from neovim formula:
```ruby
stable do
  url "https://github.com/neovim/neovim/archive/refs/tags/v0.11.5.tar.gz"
  sha256 "c63450dfb42bb0115cd5e959f81c77989e1c8fd020d5e3f1e6d897154ce8b771"
  depends_on "tree-sitter@0.25"
end

head do
  url "https://github.com/neovim/neovim.git", branch: "master"
  depends_on "tree-sitter"
end
```

#### Dependency Methods

- **`depends_on "package"`**: Runtime dependency
- **`depends_on "package" => :build`**: Build-time only dependency
- **`depends_on "package" => :test`**: Test-time only dependency
- **`depends_on "package" => :optional`**: Optional dependency (generates `--with-package` flag)
- **`depends_on "package" => :recommended`**: Recommended dependency (generates `--without-package` flag)
- **`uses_from_macos "package"`**: Use system package if available, otherwise install from Homebrew
- **`uses_from_macos "package", since: :sequoia`**: Use system package only on specified macOS version or newer

Example from jq formula:
```ruby
head do
  url "https://github.com/jqlang/jq.git", branch: "master"
  depends_on "autoconf" => :build
  depends_on "automake" => :build
  depends_on "libtool" => :build
end

depends_on "oniguruma"
```

Example from postgresql@17:
```ruby
depends_on "docbook" => :build
depends_on "docbook-xsl" => :build
depends_on "gettext" => :build
depends_on "pkgconf" => :build
depends_on "icu4c@78"
depends_on "krb5"

uses_from_macos "bison" => :build
uses_from_macos "flex" => :build
uses_from_macos "libxml2"
uses_from_macos "zlib"
```

#### Bottle Block

Defines pre-built binary packages for different platforms:

```ruby
bottle do
  rebuild 0  # Optional rebuild counter
  root_url "https://ghcr.io/v2/homebrew/core"  # Usually omitted (uses default)
  sha256 cellar: :any,                 arm64_sequoia: "abc123..."
  sha256 cellar: :any,                 arm64_sonoma:  "def456..."
  sha256 cellar: :any,                 sonoma:        "ghi789..."
  sha256 cellar: :any_skip_relocation, arm64_linux:   "jkl012..."
  sha256 cellar: :any_skip_relocation, x86_64_linux:  "mno345..."
end
```

**Cellar values:**
- `:any`: Bottle contains references to Cellar location (can be installed in any Cellar)
- `:any_skip_relocation`: No Cellar references, fully relocatable
- `"/opt/homebrew/Cellar"` or `"/usr/local/Cellar"`: Specific Cellar path required

**Platform identifiers:**
- macOS ARM: `arm64_tahoe`, `arm64_sequoia`, `arm64_sonoma`, `arm64_ventura`
- macOS Intel: `tahoe`, `sequoia`, `sonoma`, `ventura`
- Linux ARM: `arm64_linux`
- Linux Intel: `x86_64_linux`

#### Additional Configuration Methods

- **`keg_only`**: Formula is keg-only (not linked into Homebrew prefix)
  ```ruby
  keg_only :versioned_formula
  keg_only reason: "conflicts with system library", explanation: "..."
  ```
- **`link_overwrite`**: Files to overwrite during linking
  ```ruby
  link_overwrite "bin/npm", "bin/npx"
  ```
- **`conflicts_with`**: Formulas that conflict with this one
  ```ruby
  conflicts_with "other-formula", because: "both install the same binary"
  ```
- **`fails_with`**: Compiler requirements
  ```ruby
  fails_with :gcc do
    version "11"
    cause "needs GCC 12 or newer"
  end
  ```
- **`pour_bottle_only_if`**: Conditional for when to use bottles (rarely used)
- **`post_install_defined`**: Boolean indicating if `post_install` method exists

### 1.3 Resources

Resources define additional downloads required for installation (common in language-specific tools):

```ruby
resource "resource-name" do
  url "https://example.com/resource.tar.gz"
  sha256 "abc123..."
end
```

Example from neovim (tree-sitter grammars):
```ruby
resource "tree-sitter-c" do
  url "https://github.com/tree-sitter/tree-sitter-c/archive/refs/tags/v0.24.1.tar.gz"
  sha256 "25dd4bb3dec770769a407e0fc803f424ce02c494a56ce95fedc525316dcf9b48"
end
```

Resources can be staged and used in the `install` method:
```ruby
def install
  resources.each do |r|
    r.stage(buildpath/"deps"/r.name)
  end
end
```

Example from git (Perl modules):
```ruby
resource "Authen::SASL" do
  url "https://cpan.metacpan.org/authors/id/E/EH/EHUELS/Authen-SASL-2.1900.tar.gz"
  sha256 "be3533a6891b2e677150b479c1a0d4bf11c8bbeebed3e7b8eba34053e93923b0"
end
```

### 1.4 Patches

Patches can be applied to formulas in several ways:

#### Inline Patches with `__END__`

```ruby
class Tool < Formula
  # ... formula definition ...

  patch :DATA  # or patch :p1, :DATA
end

__END__
diff --git a/file.c b/file.c
index 1234..5678 100644
--- a/file.c
+++ b/file.c
@@ -1,1 +1,1 @@
-old line
+new line
```

#### External Patch URLs

```ruby
patch do
  url "https://example.com/patches/fix.patch"
  sha256 "abc123..."
end
```

Example from python@3.13:
```ruby
patch do
  url "https://raw.githubusercontent.com/Homebrew/homebrew-core/1cf441a0/Patches/python/3.13-sysconfig.diff"
  sha256 "9f2eae1d08720b06ac3d9ef1999c09388b9db39dfb52687fc261ff820bff20c3"
end
```

#### Multiple Patches

Multiple patches can be specified sequentially:
```ruby
patch do
  url "https://example.com/patch1.diff"
  sha256 "abc..."
end

patch do
  url "https://example.com/patch2.diff"
  sha256 "def..."
end
```

**Note:** In external patches, the string `@@HOMEBREW_PREFIX@@` is replaced with the actual Homebrew prefix before application.

### 1.5 Platform Conditionals

Homebrew provides platform-specific blocks for dependencies, resources, patches, and other configuration:

#### Top-Level Platform Blocks

```ruby
on_macos do
  depends_on "llvm" => :build
end

on_linux do
  depends_on "berkeley-db@5"
end

on_arm do
  # ARM-specific configuration
end

on_intel do
  # Intel-specific configuration
end
```

#### Nested Platform Blocks

```ruby
on_macos do
  on_arm do
    # macOS ARM-specific configuration
  end
end
```

#### Version-Specific Conditionals

```ruby
on_sequoia do
  depends_on "gettext"
end

on_sequoia :or_newer do
  depends_on "gettext" => :build
end

on_monterey :or_older do
  depends_on "legacy-lib"
end
```

#### Inside `install` and `test` Methods

Don't use `on_*` methods inside `def install` or `test do` blocks. Use Ruby conditionals instead:

```ruby
def install
  if OS.mac?
    # macOS-specific code
  end

  if OS.linux?
    # Linux-specific code
  end

  if Hardware::CPU.arm?
    # ARM-specific code
  end

  if Hardware::CPU.intel?
    # Intel-specific code
  end

  if MacOS.version >= :tahoe
    # macOS Tahoe or newer
  end
end
```

Example from python@3.13:
```ruby
def install
  if OS.mac?
    inreplace "configure", "libmpdec_machine=universal",
              "libmpdec_machine=#{Hardware::CPU.arm? ? "uint128" : "x64"}"
    args << "--with-lto"
    args << "--enable-framework=#{frameworks}"
  else
    args << "--enable-shared"
  end
end
```

---

## 2. Bottle vs Source

### 2.1 What are Bottles?

Bottles are pre-compiled binary packages distributed as gzipped tarballs. They contain:
- Compiled binaries
- Formula metadata at `<formula>/<version>/.brew/<formula>.rb`
- Platform-specific builds (OS version, architecture)

**Benefits:**
- Faster installation (no compilation)
- Consistent builds
- Reduced dependency on build tools

### 2.2 Bottle Specification in Formulas

Bottles are defined in a `bottle do ... end` block:

```ruby
bottle do
  rebuild 0  # Increment when bottle needs rebuilding without version change
  root_url "https://ghcr.io/v2/homebrew/core"  # Default GHCR URL (usually omitted)
  sha256 cellar: :any, arm64_sequoia: "d7bce557bb82addd6cf01b8bb758d373ee11cb6671e4d7b1dc2a2c89816bcc32"
  sha256 cellar: :any, sonoma:        "a1a5f487f840d9a18abdecdf1c6c5a5385917725c6ba88f7f819ac5f4cfa801"
end
```

### 2.3 When Bottles are Available

Bottles are automatically built by Homebrew's CI (BrewTestBot) when:
- A formula PR is submitted to homebrew-core
- The formula builds successfully on all supported platforms
- A maintainer approves the change

**Storage:** Bottles are stored in GitHub Container Registry (GHCR) at `ghcr.io/homebrew/core/<formula>:<version>`

### 2.4 Detecting Bottle Availability

**From Formula Ruby Source:**
Check for the presence of a `bottle do ... end` block.

**From JSON API:**
```json
{
  "versions": {
    "stable": "1.8.1",
    "bottle": true
  },
  "bottle": {
    "stable": {
      "rebuild": 0,
      "root_url": "https://ghcr.io/v2/homebrew/core",
      "files": {
        "arm64_sequoia": {
          "cellar": ":any",
          "url": "https://ghcr.io/v2/homebrew/core/jq/blobs/sha256:...",
          "sha256": "..."
        }
      }
    }
  }
}
```

### 2.5 When Source Compilation is Required

Bottles may not be available when:
- Formula has `:build` dependencies only
- Formula uses `bottle :unneeded` (deprecated practice)
- Building from `HEAD` version
- User passes `--build-from-source` flag
- Platform-specific bottle doesn't exist
- User has modified the formula

### 2.6 `bottle :unneeded` (Deprecated)

Previously used for formulas that only copy pre-built binaries without compilation. This directive is now deprecated with no replacement. Modern practice:
- Omit the bottle block entirely for simple binary formulas
- Bottles will be added by CI after merge

---

## 3. Dependency Types

### 3.1 Runtime Dependencies

Default dependency type - required at runtime:

```ruby
depends_on "openssl@3"
depends_on "pcre2"
```

### 3.2 Build Dependencies

Required only during compilation, can be skipped when installing from bottle:

```ruby
depends_on "cmake" => :build
depends_on "pkgconf" => :build
depends_on "rust" => :build
```

Example from ripgrep:
```ruby
depends_on "asciidoctor" => :build
depends_on "pkgconf" => :build
depends_on "rust" => :build
depends_on "pcre2"  # Runtime dependency
```

### 3.3 Test Dependencies

Required only when running `brew test`:

```ruby
depends_on "test-framework" => :test
```

### 3.4 Optional Dependencies

Generate `--with-package` flag for optional features:

```ruby
depends_on "foo" => :optional
```

Users can install with: `brew install tool --with-foo`

### 3.5 Recommended Dependencies

Enabled by default, users can disable with `--without-package`:

```ruby
depends_on "foo" => :recommended
```

Users can install without: `brew install tool --without-foo`

### 3.6 System Dependencies

#### `uses_from_macos`

Prefer system-provided packages on macOS, fall back to Homebrew:

```ruby
uses_from_macos "curl"
uses_from_macos "zlib"
uses_from_macos "expat", since: :sequoia  # Only on Sequoia+, otherwise Homebrew
```

Example from postgresql@17:
```ruby
uses_from_macos "bison" => :build
uses_from_macos "flex" => :build
uses_from_macos "libxml2"
uses_from_macos "libxslt"
uses_from_macos "perl"
uses_from_macos "zlib"
```

#### Special System Requirements

```ruby
depends_on :xcode => :build  # Requires Xcode
depends_on :macos => :catalina  # Requires macOS Catalina or newer
```

### 3.7 Versioned Dependencies

Pin to specific major/minor versions:

```ruby
depends_on "python@3.13"
depends_on "icu4c@78"
depends_on "postgresql@17"
```

Example from neovim:
```ruby
stable do
  depends_on "tree-sitter@0.25"
end

head do
  depends_on "tree-sitter"  # Latest version
end
```

### 3.8 Platform-Specific Dependencies

```ruby
on_linux do
  depends_on "linux-pam"
  depends_on "util-linux"
end

on_macos do
  depends_on "llvm" => :build if DevelopmentTools.clang_build_version <= 1699
end
```

Example from node:
```ruby
on_macos do
  depends_on "llvm" => :build if DevelopmentTools.clang_build_version <= 1699
end
```

---

## 4. Formula Sources

### 4.1 Homebrew-Core Repository Structure

```
homebrew-core/
├── Formula/
│   ├── a/
│   │   ├── aws-cli.rb
│   │   └── ansible.rb
│   ├── j/
│   │   └── jq.rb
│   ├── r/
│   │   └── ripgrep.rb
│   └── ...
├── Casks/
└── .github/workflows/
```

**Repository URL:** `https://github.com/Homebrew/homebrew-core`

**Formula Path Pattern:** `Formula/<first-letter>/<formula-name>.rb`

**Raw File URL:** `https://raw.githubusercontent.com/Homebrew/homebrew-core/HEAD/Formula/<letter>/<name>.rb`

Example: `https://raw.githubusercontent.com/Homebrew/homebrew-core/HEAD/Formula/j/jq.rb`

### 4.2 Homebrew JSON API

**Base URL:** `https://formulae.brew.sh/api/`

#### Single Formula

```
GET https://formulae.brew.sh/api/formula/{name}.json
```

Returns: `brew info --json --formula <formula>` output with analytics and metadata

**Example Response Structure:**
```json
{
  "name": "jq",
  "full_name": "jq",
  "tap": "homebrew/core",
  "desc": "Lightweight and flexible command-line JSON processor",
  "license": "MIT",
  "homepage": "https://jqlang.github.io/jq/",
  "versions": {
    "stable": "1.8.1",
    "head": "HEAD",
    "bottle": true
  },
  "urls": {
    "stable": {
      "url": "https://github.com/jqlang/jq/releases/download/jq-1.8.1/jq-1.8.1.tar.gz",
      "tag": null,
      "revision": null,
      "checksum": "2be64e7129cecb11d5906290eba10af694fb9e3e7f9fc208a311dc33ca837eb0"
    },
    "head": {
      "url": "https://github.com/jqlang/jq.git",
      "branch": "master"
    }
  },
  "revision": 0,
  "bottle": {
    "stable": {
      "rebuild": 0,
      "root_url": "https://ghcr.io/v2/homebrew/core",
      "files": {
        "arm64_sequoia": {
          "cellar": ":any",
          "url": "https://ghcr.io/v2/homebrew/core/jq/blobs/sha256:d7bce557...",
          "sha256": "d7bce557..."
        }
      }
    }
  },
  "build_dependencies": [],
  "dependencies": ["oniguruma"],
  "test_dependencies": [],
  "recommended_dependencies": [],
  "optional_dependencies": [],
  "uses_from_macos": [],
  "requirements": [],
  "conflicts_with": [],
  "keg_only": false,
  "keg_only_reason": null,
  "analytics": {
    "install": {
      "30d": {"jq": 48839},
      "90d": {"jq": 149460},
      "365d": {"jq": 634366}
    }
  },
  "ruby_source_path": "Formula/j/jq.rb",
  "ruby_source_checksum": {
    "sha256": "6accc6c2e0c3eef2c4bd4e1628d5015b0caebda30f3a386d9141a3b57bb5f13c"
  }
}
```

#### All Formulae

```
GET https://formulae.brew.sh/api/formula.json
```

Returns: Array of all formulae with full metadata

#### Single Cask

```
GET https://formulae.brew.sh/api/cask/{name}.json
```

Returns: `brew info --json=v2 --cask <cask>` output

#### Analytics

```
GET https://formulae.brew.sh/api/analytics/{category}/{days}.json
```

Categories: `install`, `install-on-request`, `build-error`
Days: `30d`, `90d`, `365d`

### 4.3 GHCR Manifest Structure for Bottles

Homebrew stores bottles in GitHub Container Registry (GHCR) using OCI (Open Container Initiative) format.

**Registry Base:** `ghcr.io/v2/homebrew/core`

**Image Format:** `ghcr.io/homebrew/core/<formula>:<version>`

#### OCI Structure

1. **Image Index**: Top-level entity containing OS/architecture metadata
2. **Image Manifests**: One per platform (OS + architecture)
3. **Layers/Blobs**: Actual bottle tarballs

#### Fetching Bottle Manifest

```bash
# Request manifest
curl -H "Accept: application/vnd.oci.image.index.v1+json" \
     -H "Authorization: Bearer " \
     https://ghcr.io/v2/homebrew/core/jq/manifests/1.8.1
```

**Manifest Response:**
```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "manifests": [
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:...",
      "size": 123,
      "platform": {
        "architecture": "arm64",
        "os": "darwin",
        "os.version": "macOS 14"
      }
    }
  ]
}
```

#### Downloading Bottles

1. Fetch manifest to get layer digest
2. Download blob using digest:
   ```bash
   curl -L https://ghcr.io/v2/homebrew/core/jq/blobs/sha256:d7bce557... \
        -o jq.bottle.tar.gz
   ```

**Bottle Filename Pattern:** `<formula>--<version>.<platform>.bottle.tar.gz`

Example: `jq--1.8.1.arm64_sequoia.bottle.tar.gz`

#### Tools for OCI Operations

- **oras**: CLI tool for OCI Registry As Storage operations
- Homebrew uses GHCR's OCI API for standard query, upload, and download operations

---

## 5. Complexity Spectrum

### 5.1 Simple Formula: jq

**Characteristics:**
- Single source tarball
- One runtime dependency
- Standard autotools build
- Straightforward installation

**Full Formula:**
```ruby
class Jq < Formula
  desc "Lightweight and flexible command-line JSON processor"
  homepage "https://jqlang.github.io/jq/"
  url "https://github.com/jqlang/jq/releases/download/jq-1.8.1/jq-1.8.1.tar.gz"
  sha256 "2be64e7129cecb11d5906290eba10af694fb9e3e7f9fc208a311dc33ca837eb0"
  license "MIT"

  bottle do
    sha256 cellar: :any, arm64_sequoia: "d7bce557bb82addd6cf01b8bb758d373ee11cb6671e4d7b1dc2a2c89816bcc32"
    sha256 cellar: :any, sonoma:        "a1a5f487f1840d9a18abdecdf1c6c5a5385917725c6ba88f7f819ac5f4cfa801"
  end

  head do
    url "https://github.com/jqlang/jq.git", branch: "master"
    depends_on "autoconf" => :build
    depends_on "automake" => :build
    depends_on "libtool" => :build
  end

  depends_on "oniguruma"

  def install
    system "autoreconf", "--force", "--install", "--verbose" if build.head?
    system "./configure", *std_configure_args,
                          "--disable-silent-rules",
                          "--disable-maintainer-mode"
    system "make", "install"
  end

  test do
    assert_equal "2\n", pipe_output("#{bin}/jq .bar", '{"foo":1, "bar":2}')
  end
end
```

**tsuku Recipe Implications:**
- Direct URL download
- Extract tarball
- Run configure + make + make install
- One dependency to resolve

### 5.2 Medium Complexity: ripgrep

**Characteristics:**
- Rust-based build (Cargo)
- Multiple build dependencies
- Shell completion generation
- Man page generation

**Full Formula:**
```ruby
class Ripgrep < Formula
  desc "Search tool like grep and The Silver Searcher"
  homepage "https://github.com/BurntSushi/ripgrep"
  url "https://github.com/BurntSushi/ripgrep/archive/refs/tags/15.1.0.tar.gz"
  sha256 "046fa01a216793b8bd2750f9d68d4ad43986eb9c0d6122600f993906012972e8"
  license "Unlicense"
  head "https://github.com/BurntSushi/ripgrep.git", branch: "master"

  bottle do
    sha256 cellar: :any, arm64_sequoia: "0153b06af62b4b8c6ed3f2756dcc4859f74a6128a286f976740468229265cfbe"
    sha256 cellar: :any, sonoma:        "ab382b4ae86aba1b7e6acab3bc50eb64be7bb08cf33a37a32987edb8bc6affe4"
  end

  depends_on "asciidoctor" => :build
  depends_on "pkgconf" => :build
  depends_on "rust" => :build
  depends_on "pcre2"

  def install
    system "cargo", "install", "--features", "pcre2", *std_cargo_args
    generate_completions_from_executable(bin/"rg", "--generate", shell_parameter_format: "complete-")
    (man1/"rg.1").write Utils.safe_popen_read(bin/"rg", "--generate", "man")
  end

  test do
    (testpath/"Hello.txt").write("Hello World!")
    system bin/"rg", "Hello World!", testpath
  end
end
```

**tsuku Recipe Implications:**
- Cargo build system
- Need Rust toolchain
- Post-build artifact generation (completions, man pages)
- Features flag support

### 5.3 Complex Formula: neovim

**Characteristics:**
- Multiple resources (tree-sitter grammars)
- CMake build system for multiple components
- Platform-specific patches
- File rewriting (inreplace)
- Conditional logic based on stable vs head

**Partial Formula (resources section):**
```ruby
class Neovim < Formula
  desc "Ambitious Vim-fork focused on extensibility and agility"
  homepage "https://neovim.io/"
  license "Apache-2.0"
  revision 1

  stable do
    url "https://github.com/neovim/neovim/archive/refs/tags/v0.11.5.tar.gz"
    sha256 "c63450dfb42bb0115cd5e959f81c77989e1c8fd020d5e3f1e6d897154ce8b771"
    depends_on "tree-sitter@0.25"

    resource "tree-sitter-c" do
      url "https://github.com/tree-sitter/tree-sitter-c/archive/refs/tags/v0.24.1.tar.gz"
      sha256 "25dd4bb3dec770769a407e0fc803f424ce02c494a56ce95fedc525316dcf9b48"
    end

    resource "tree-sitter-lua" do
      url "https://github.com/tree-sitter-grammars/tree-sitter-lua/archive/refs/tags/v0.4.0.tar.gz"
      sha256 "b0977aced4a63bb75f26725787e047b8f5f4a092712c840ea7070765d4049559"
    end
  end

  head do
    url "https://github.com/neovim/neovim.git", branch: "master"
    depends_on "tree-sitter"
  end

  depends_on "cmake" => :build
  depends_on "gettext"
  depends_on "libuv"

  def install
    # Stage resources
    resources.each do |r|
      source_directory = buildpath/"deps-build/build/src"/r.name
      build_directory = buildpath/"deps-build/build"/r.name
      parser_name = r.name.split("-").last

      r.stage(source_directory)
      cp buildpath/"cmake.deps/cmake/TreesitterParserCMakeLists.txt",
         source_directory/"CMakeLists.txt"

      system "cmake", "-S", source_directory, "-B", build_directory,
             "-DPARSERLANG=#{parser_name}", *std_cmake_args
      system "cmake", "--build", build_directory
      system "cmake", "--install", build_directory
    end

    # Patch system paths
    inreplace "src/nvim/os/stdpaths.c" do |s|
      s.gsub! "/etc/xdg/", "#{etc}/xdg/:\\0"
      if HOMEBREW_PREFIX.to_s != HOMEBREW_DEFAULT_PREFIX
        s.gsub! "/usr/local/share/:/usr/share/", "#{HOMEBREW_PREFIX}/share/:\\0"
      end
    end

    # Main build
    system "cmake", "-S", ".", "-B", "build", *std_cmake_args
    system "cmake", "--build", "build"
    system "cmake", "--install", "build"
  end
end
```

**tsuku Recipe Implications:**
- Multiple downloads (main + resources)
- Staged resource builds
- File patching/rewriting
- Complex build orchestration
- Conditional paths based on prefix

### 5.4 Very Complex: postgresql@17

**Characteristics:**
- Many build and runtime dependencies
- Platform-specific dependency variations
- `uses_from_macos` for system libraries
- Keg-only (versioned formula)
- Post-install hook
- Service definition
- Caveats (user instructions)

**Key Sections:**
```ruby
class PostgresqlAT17 < Formula
  desc "Object-relational database system"
  license "PostgreSQL"

  bottle do
    sha256 cellar: "/opt/homebrew/Cellar", arm64_sequoia: "dc25244bf..."
    sha256 cellar: "/usr/local/Cellar",    sonoma:        "ecd08b49..."
  end

  keg_only :versioned_formula

  depends_on "docbook" => :build
  depends_on "gettext" => :build
  depends_on "pkgconf" => :build
  depends_on "icu4c@78"
  depends_on "krb5"
  depends_on "openssl@3"
  depends_on "readline"

  uses_from_macos "bison" => :build
  uses_from_macos "flex" => :build
  uses_from_macos "libxml2"
  uses_from_macos "perl"
  uses_from_macos "zlib"

  on_linux do
    depends_on "linux-pam"
    depends_on "util-linux"
  end

  def install
    args = %W[
      --prefix=#{prefix}
      --enable-ipv6
      --with-openssl=#{Formula["openssl@3"].opt_prefix}
      --with-system-libmpdec
    ]

    system "./configure", *args
    system "make"
    system "make", "install"
  end

  service do
    run ["#{opt_prefix}/bin/postgres", "-D", "#{var}/postgresql@17"]
    keep_alive true
    environment_variables LC_ALL: "en_US.UTF-8"
    log_path var/"log/postgresql@17.log"
    error_log_path var/"log/postgresql@17.log"
  end

  def caveats
    <<~EOS
      This formula has created a default database cluster with:
        initdb --locale=en_US.UTF-8 -E UTF-8 $HOMEBREW_PREFIX/var/postgresql@17
    EOS
  end
end
```

**tsuku Recipe Implications:**
- Platform-specific dependency resolution
- Service management not directly applicable to tsuku
- Caveats would need to be shown to user
- Keg-only status affects linking strategy

### 5.5 Very Complex: python@3.13

**Characteristics:**
- Multiple resources (pip, wheel, flit-core)
- External patches
- Framework build on macOS vs shared library on Linux
- Extensive platform conditionals
- File rewriting (inreplace)
- Complex configure arguments
- Multiple installation targets

**Key Complexity Indicators:**
```ruby
class PythonAT313 < Formula
  resource "pip" do
    url "https://files.pythonhosted.org/packages/.../pip-25.3.tar.gz"
    sha256 "8d0538dbbd7babbd207f261ed969c65de439f6bc9e5dbd3b3b9a77f25d95f343"
  end

  patch do
    url "https://raw.githubusercontent.com/Homebrew/homebrew-core/1cf441a0/Patches/python/3.13-sysconfig.diff"
    sha256 "9f2eae1d08720b06ac3d9ef1999c09388b9db39dfb52687fc261ff820bff20c3"
  end

  def lib_cellar
    on_macos do
      return frameworks/"Python.framework/Versions"/version.major_minor/"lib/python#{version.major_minor}"
    end
    on_linux do
      return lib/"python#{version.major_minor}"
    end
  end

  def install
    if OS.mac?
      inreplace "configure", "libmpdec_machine=universal",
                "libmpdec_machine=#{Hardware::CPU.arm? ? "uint128" : "x64"}"
      args << "--enable-framework=#{frameworks}"
      args << "--with-lto"
    else
      args << "--enable-shared"
    end

    system "./configure", *args
    system "make"
    system "make", "altinstall" if altinstall?
  end
end
```

**tsuku Recipe Implications:**
- Highly platform-dependent
- May be better to use bottles exclusively
- Source builds would require significant custom logic
- Resources need Python-specific staging

---

## 6. Build System Examples

### 6.1 Autotools (configure + make)

**Standard helpers:** `std_configure_args(prefix: prefix, libdir: "lib")`

**Example:**
```ruby
def install
  system "./configure", *std_configure_args, "--disable-debug"
  system "make", "install"
end
```

**With autoreconf:**
```ruby
def install
  system "autoreconf", "--force", "--install", "--verbose"
  system "./configure", *std_configure_args
  system "make", "install"
end
```

### 6.2 CMake

**Standard helpers:** `std_cmake_args(install_prefix: prefix, install_libdir: "lib", find_framework: "LAST")`

**Example:**
```ruby
depends_on "cmake" => :build

def install
  system "cmake", "-S", ".", "-B", "build", *std_cmake_args
  system "cmake", "--build", "build"
  system "cmake", "--install", "build"
end
```

**With custom prefix:**
```ruby
system "cmake", "-S", ".", "-B", "build", *std_cmake_args(install_prefix: libexec)
```

### 6.3 Cargo (Rust)

**Standard helpers:** `std_cargo_args`

**Example:**
```ruby
depends_on "rust" => :build

def install
  system "cargo", "install", *std_cargo_args
end
```

**With features:**
```ruby
system "cargo", "install", "--features", "pcre2", *std_cargo_args
```

### 6.4 Go

**Standard helpers:** `std_go_args`

**Example:**
```ruby
def install
  system "go", "build", *std_go_args, "-o", bin/"tool"
end
```

### 6.5 Python (pip)

**Standard helpers:** `std_pip_args`

**Example:**
```ruby
def install
  virtualenv_install_with_resources
end
```

### 6.6 Make (without configure)

```ruby
def install
  system "make", "PREFIX=#{prefix}", "install"
end
```

### 6.7 Custom Install (binary only)

```ruby
def install
  bin.install "binary-name"
  man1.install "docs/man.1"
end
```

---

## 7. Key Insights for LLM-Based Builder

### 7.1 Parsing Priorities

1. **Bottle availability** should be checked first (JSON API or bottle block presence)
2. **Dependencies** must distinguish build vs runtime
3. **Platform conditionals** require OS/architecture awareness
4. **Build system detection** can be inferred from `depends_on` and `install` method

### 7.2 Complexity Heuristics

**Low complexity (prefer source build):**
- No dependencies or only runtime deps
- Standard build system (autotools, cmake, cargo)
- No resources or patches
- No platform conditionals

**Medium complexity (bottle preferred):**
- Multiple dependencies
- Resources but no patches
- Simple platform conditionals
- Standard build with extra steps

**High complexity (bottle only):**
- External patches
- Complex inreplace operations
- Framework builds (Python, Ruby)
- Heavy platform-specific logic

### 7.3 Bottle vs Source Decision Tree

```
Has bottles in formula?
├─ Yes
│  ├─ Platform bottle available?
│  │  ├─ Yes → Use bottle (fastest)
│  │  └─ No → Assess source complexity
│  └─ Complex formula?
│     ├─ Yes → Skip (too complex for tsuku source build)
│     └─ No → Attempt source build
└─ No
   └─ Simple formula?
      ├─ Yes → Attempt source build
      └─ No → Skip
```

### 7.4 JSON API Usage

**Advantages:**
- Structured data (easier parsing)
- Includes analytics (popularity metrics)
- Bottle availability and URLs
- Dependency lists (categorized)

**Disadvantages:**
- Missing install method logic
- No access to patches or inreplace operations
- Limited visibility into build complexity

**Recommendation:** Use JSON API for initial filtering, fetch Ruby source for detailed analysis.

### 7.5 Bottle Download Strategy

For tsuku's existing `HomebrewBottleAction`:

1. Fetch formula JSON: `https://formulae.brew.sh/api/formula/{name}.json`
2. Check `versions.bottle == true`
3. Get platform-specific bottle URL from `bottle.stable.files.{platform}.url`
4. Download bottle tarball
5. Extract to `$TSUKU_HOME/tools/{name}-{version}/`
6. Handle bottle-specific Cellar paths (`:any`, `:any_skip_relocation`)

**Note:** GHCR URLs are directly accessible in the JSON API response, no need to query OCI manifest.

---

## 8. Recommendations for tsuku Homebrew Builder

### 8.1 Phase 1: Bottle-Only Builder

**Scope:** Use existing `HomebrewBottleAction` with LLM-enhanced formula selection

**Implementation:**
1. Query JSON API for formula metadata
2. LLM analyzes:
   - Bottle availability
   - Dependency tree
   - Platform compatibility
3. Generate tsuku recipe with:
   - `HomebrewBottleAction` for bottle download
   - Dependency resolution via tsuku recipes
4. Handle edge cases (keg-only, conflicts)

**Advantages:**
- Builds on existing functionality
- High success rate (bottles are pre-built)
- Fast installation

**Limitations:**
- Requires bottles for target platform
- Cannot customize build flags

### 8.2 Phase 2: Simple Source Builder

**Scope:** Extend to formulas without bottles but simple build process

**Criteria for source build:**
- No `bottle` block or `bottle :unneeded`
- Standard build system (autotools, cmake, cargo, go)
- No patches or resources
- Minimal/no platform conditionals

**Implementation:**
1. LLM parses install method
2. Detects build system from `system` calls
3. Maps to tsuku actions:
   - `DownloadAction` for source tarball
   - `ExtractAction`
   - `ConfigureMakeAction` or `CMakeAction` or `CargoAction`
   - `InstallBinariesAction`

**Advantages:**
- Supports formulas without bottles
- Enables customization

**Challenges:**
- Build environment setup
- Dependency ordering
- Error handling

### 8.3 Phase 3: Complex Source Builder

**Scope:** Handle formulas with resources, patches, and conditionals

**New capabilities:**
- Resource staging
- Patch application
- Platform conditionals
- inreplace operations

**Implementation:**
1. LLM generates build script from install method
2. tsuku executes script in sandboxed environment
3. Platform detection resolves conditionals

**Advantages:**
- Maximum formula coverage

**Challenges:**
- Complexity of Ruby → tsuku recipe translation
- Testing and validation
- Maintenance burden

### 8.4 Recommended Approach

**Start with Phase 1:**
- Low risk, high value
- Immediate utility for most popular packages (which have bottles)
- Proves LLM integration works

**Evaluate Phase 2:**
- After Phase 1 success, measure demand for source builds
- Prioritize formulas with high analytics but no bottles

**Consider Phase 3:**
- Only if clear user need
- May be better to contribute upstream Homebrew bottles

---

## 9. References

### Documentation
- [Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)
- [Homebrew Bottles Documentation](https://docs.brew.sh/Bottles)
- [Homebrew Ruby API](https://rubydoc.brew.sh/Formula)
- [Homebrew JSON API](https://formulae.brew.sh/docs/api/)
- [Querying Brew](https://docs.brew.sh/Querying-Brew)

### GHCR and OCI
- [How Homebrew serves 52M packages per month](https://betterprogramming.pub/how-homebrew-serves-52m-packages-per-month-413b9f0cf685)
- [Storing Blobs on GitHub Container Registry](https://www.aahlenst.dev/blog/storing-blobs-on-github-container-registry/)
- [OCI Image Manifest Specification](https://github.com/opencontainers/image-spec/blob/main/manifest.md)
- [Fetch from OCI registry (ghcr.io)](https://gist.github.com/wolfv/fc04f85b2bd0141326f6ecff03d9b101)

### Platform Conditionals
- [Add on_{system} blocks PR](https://github.com/Homebrew/brew/pull/13451)

### Formula Patches
- [Homebrew formula-patches repository](https://github.com/Homebrew/formula-patches)

### Example Formulas
- [jq formula](https://github.com/Homebrew/homebrew-core/blob/HEAD/Formula/j/jq.rb)
- [ripgrep formula](https://github.com/Homebrew/homebrew-core/blob/HEAD/Formula/r/ripgrep.rb)
- [neovim formula](https://github.com/Homebrew/homebrew-core/blob/HEAD/Formula/n/neovim.rb)
- [node formula](https://github.com/Homebrew/homebrew-core/blob/HEAD/Formula/n/node.rb)
- [git formula](https://github.com/Homebrew/homebrew-core/blob/HEAD/Formula/g/git.rb)

---

## Appendix: Formula Complexity Examples

### A1: Simple (jq)
- **Dependencies:** 1 runtime, 3 build (head only)
- **Build system:** autotools
- **Platform conditionals:** None
- **Resources:** None
- **Patches:** None

### A2: Medium (ripgrep)
- **Dependencies:** 3 build, 1 runtime
- **Build system:** Cargo
- **Platform conditionals:** None
- **Resources:** None
- **Post-build:** Shell completions, man pages

### A3: Complex (neovim)
- **Dependencies:** 8+ runtime, 1+ build
- **Build system:** CMake (multiple invocations)
- **Platform conditionals:** stable vs head
- **Resources:** 6+ tree-sitter grammars
- **Patches:** File inreplace operations
- **Custom logic:** Resource staging, parser builds

### A4: Very Complex (postgresql@17)
- **Dependencies:** 10+ mixed, platform-specific
- **Build system:** autotools
- **Platform conditionals:** macOS vs Linux variations
- **Keg-only:** Versioned formula
- **Service:** systemd/launchd integration
- **Post-install:** Database initialization

### A5: Very Complex (python@3.13)
- **Dependencies:** 10+ mixed, version-specific
- **Build system:** autotools with heavy customization
- **Platform conditionals:** Framework vs shared, LTO on/off
- **Resources:** 3 (pip, wheel, flit-core)
- **Patches:** External sysconfig patch
- **File rewriting:** Multiple inreplace operations
- **Custom methods:** lib_cellar, site_packages

---

**Document Version:** 1.0
**Last Updated:** 2025-12-13
**Author:** Research for tsuku Homebrew builder design

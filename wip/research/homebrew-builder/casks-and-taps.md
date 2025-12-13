# Homebrew Casks and Taps: Architecture and Complexity Analysis

**Research Date:** 2025-12-13
**Purpose:** Inform the design of an LLM-based Homebrew builder for tsuku

## Executive Summary

This research examines Homebrew casks (GUI applications) and taps (third-party repositories) to inform a scalable LLM-based recipe builder. Key findings:

- **Casks are simpler than formulas**: No compilation, pre-built binaries, declarative installation
- **Taps follow strict conventions**: GitHub-centric, predictable structure, automatic namespacing
- **Linux support adds complexity**: Different dependencies, build environments, and bottle availability
- **Security is a critical concern**: Third-party taps have minimal oversight and executable Ruby code

**Recommendation for tsuku**: Start with core formulas (macOS), then add cask support (simpler), then consider selective third-party taps with security warnings.

---

## 1. Casks vs Formulas

### 1.1 Structural Differences in the Ruby DSL

Both formulas and casks use Ruby DSL, but casks are significantly simpler:

**Formulas** describe compilation:
- Build instructions (`def install`)
- Compiler flags and patches
- Test procedures (`test do`)
- Complex dependency resolution

**Casks** describe extraction and placement:
- Artifact placement (`app`, `pkg`, `binary`)
- Uninstall procedures
- Version tracking and updates
- No compilation steps

### 1.2 What Casks Install

Casks install **pre-built macOS binaries**:

1. **GUI Applications** (.app bundles): Visual Studio Code, Chrome, Firefox, Docker Desktop
2. **Fonts** (.ttf, .otf files): Previously in homebrew-cask-fonts (now migrated to main cask repo)
3. **Drivers and System Extensions**: Hardware drivers, kernel extensions
4. **Installer Packages** (.pkg files): Complex installers requiring system-level changes
5. **Plugins and Artifacts**: Browser extensions, system plugins, arbitrary files

Installation locations differ from formulas:
- Formulas: `/usr/local/Cellar` (Intel) or `/opt/homebrew/Cellar` (Apple Silicon)
- Casks: `/usr/local/Caskroom` or `/Applications` for .app bundles

### 1.3 Cask-Specific DSL Methods

Casks use **artifact stanzas** to declare what to install:

#### Core Artifact Stanzas

**`app` stanza** - Moves .app bundles to /Applications:
```ruby
app "Visual Studio Code.app"
app "Simple Floating Clock/SimpleFloatingClock.app"  # with subfolder
```

**`pkg` stanza** - Runs macOS installer packages:
```ruby
pkg "FabFilter One #{version} Installer.pkg"

# Requires corresponding uninstall
uninstall pkgutil: "com.fabfilter.One.#{version.major}"
```

**`binary` stanza** - Creates symlinks to CLI tools:
```ruby
binary "#{appdir}/Visual Studio Code.app/Contents/Resources/app/bin/code"
binary "#{appdir}/Visual Studio Code.app/Contents/Resources/app/bin/code-tunnel"
```

**`artifact` stanza** - Arbitrary file placement (rare, discouraged):
```ruby
artifact "arbitrary-file.dat", target: "/absolute/path/location"
```

**`installer` stanza** - Manual or scripted installers:
```ruby
installer script: {
  executable: "VirtualBox_Uninstall.tool",
  args: ["--unattended"],
  sudo: true
}
```

#### Metadata Stanzas

**Version tracking:**
```ruby
version "1.107.0"
version :latest  # for version-less tracking
```

**Architecture variants:**
```ruby
arch arm: "arm64", intel: "x86_64"

on_arm do
  sha256 "abc123..."
  url "https://example.com/app-arm64.dmg"
end

on_intel do
  sha256 "def456..."
  url "https://example.com/app-x86_64.dmg"
```

**Auto-updates:**
```ruby
auto_updates true
```

**Uninstall procedures:**
```ruby
uninstall quit: "com.example.app",
          launchctl: "com.example.agent",
          pkgutil: "com.example.pkg.*",
          delete: "/Library/Application Support/Example"

zap trash: [
  "~/Library/Application Support/Example",
  "~/Library/Preferences/com.example.app.plist",
  "~/Library/Caches/com.example.app"
]
```

#### Platform Conditionals (`on_system` blocks)

Recent additions allow OS/version-specific logic:
```ruby
on_monterey :or_newer do
  version "1.107.0"
  sha256 "abc123..."
end

on_catalina do
  version "1.97.2"  # legacy version
  sha256 "old456..."
end
```

### 1.4 How Cask Installation Differs

**Formulas** (compile and link):
1. Download source tarball
2. Extract to build directory
3. Run `./configure && make && make install`
4. Symlink to `/usr/local/bin` or `/opt/homebrew/bin`
5. May build bottles (pre-compiled binaries) on CI

**Casks** (extract and move):
1. Download pre-built binary (.dmg, .pkg, .zip)
2. Mount/extract archive
3. Move .app to /Applications OR run .pkg installer
4. Create symlinks for CLI tools (if `binary` stanza present)
5. No compilation or bottles

**Key difference**: Casks never compile. They're distribution packages.

### 1.5 Why Casks Are Easier to Convert

**Advantages for LLM-based conversion**:

1. **Simpler structure**: Fewer stanzas, no build logic
2. **Declarative**: Just describe artifacts, no procedural code
3. **Predictable patterns**: Most casks follow standard templates
4. **No dependency resolution**: Casks rarely depend on other casks (discouraged)
5. **Clear uninstall mapping**: `pkgutil` patterns are straightforward

**Example cask (Visual Studio Code)**:
```ruby
cask "visual-studio-code" do
  arch arm: "darwin-arm64", intel: "darwin"

  version "1.107.0"
  sha256 arm: "abc...", intel: "def..."

  url "https://update.code.visualstudio.com/#{version}/#{arch}/stable"
  name "Microsoft Visual Studio Code"
  desc "Open-source code editor"
  homepage "https://code.visualstudio.com/"

  auto_updates true

  app "Visual Studio Code.app"
  binary "#{appdir}/Visual Studio Code.app/Contents/Resources/app/bin/code"

  uninstall quit: "com.microsoft.VSCode"
  zap trash: [
    "~/Library/Application Support/Code",
    "~/Library/Preferences/com.microsoft.VSCode.plist"
  ]
end
```

**Conversion to tsuku recipe**:
- `url` → `download` action
- `sha256` → `checksum` field
- `app` → `extract` + `move` actions
- `binary` → `install_binaries` action
- Architecture variants map to tsuku's platform handling

**Challenges**:
- `pkg` installers run arbitrary code (hard to sandbox)
- `uninstall pkgutil` patterns may not map to tsuku
- macOS-specific (tsuku targets Linux too)

---

## 2. Third-Party Taps

### 2.1 Repository Structure and Conventions

**Naming convention**: All tap repositories must use the `homebrew-` prefix on GitHub:
```
github.com/user/homebrew-something
```

Users reference taps without the prefix:
```bash
brew tap user/something  # auto-expands to user/homebrew-something
```

**Standard directory layout**:
```
homebrew-repository/
├── .git/                    # Git repository metadata
├── Formula/                 # Formula definitions (.rb files)
├── Casks/                   # Cask definitions (.rb files)
├── cmd/                     # Custom brew commands
├── formula_renames.json     # Formula rename mappings
├── cask_renames.json        # Cask rename mappings
├── tap_migrations.json      # Cross-tap migrations
└── audit_exceptions/        # Audit rule exceptions
```

### 2.2 Resolving Formulas from Tap Names

**Installation resolution order**:

1. **Fully qualified**: `brew install user/tap/formula`
   - Automatically taps `user/homebrew-tap` if not already tapped
   - Installs `formula` from that tap

2. **Unqualified**: `brew install formula`
   - Searches homebrew/core first
   - Then searches all tapped repositories
   - Fails if ambiguous (exists in multiple taps)

**Example**:
```bash
brew install vim              # installs homebrew/core/vim
brew install username/repo/vim  # installs from custom tap
```

**Tap resolution process**:
- User provides: `brew tap user/repo`
- Homebrew clones: `https://github.com/user/homebrew-repo`
- Into: `$(brew --repository)/Library/Taps/user/homebrew-repo`
- Updates automatically on `brew update`

### 2.3 Popular Taps and Their Patterns

**Official Homebrew taps** (recently consolidated):

1. **homebrew/core** - Default formula tap (command-line tools, libraries)
   - ~7,000+ formulas (estimated, no exact count found)
   - Open-source only, built from source
   - Strict review process

2. **homebrew/cask** - macOS GUI applications, fonts, drivers
   - ~5,000+ casks (estimated)
   - May include closed-source software
   - Recently absorbed homebrew-cask-versions, homebrew-cask-fonts, homebrew-cask-drivers

3. **homebrew/services** - Service management integration (deprecated, functionality moved to core)

**Previously separate, now merged into homebrew/cask**:
- **homebrew-cask-versions**: Beta/dev/legacy versions of casks
- **homebrew-cask-fonts**: Font casks (consolidated May 2024)
- **homebrew-cask-drivers**: Hardware drivers

**Popular third-party taps** (examples):
- Company-specific: `mongodb/brew`, `azure/functions`, `heroku/brew`
- Language ecosystems: `dart-lang/dart`, `adoptopenjdk/openjdk`
- Regional: Country-specific formulas

**Common tap patterns**:
- **Vendor taps**: Companies distribute their tools (e.g., `mongodb/brew/mongodb-community`)
- **Version taps**: Multiple versions of same tool
- **Niche tools**: Specialized software not in core

### 2.4 Security Considerations with Third-Party Taps

**Critical security findings** (Trail of Bits audit, August 2023):

> A Trail of Bits security audit sponsored by the Open Tech Fund, performed in August 2023, uncovered a total of 25 security defects in Homebrew. Multiple vulnerabilities could have allowed attackers to load executable code and modify binary builds, potentially controlling CI/CD workflow execution and exfiltrating secrets.

**Core vulnerability**: Formulas and casks are executable Ruby code:

> Local package management tools install and execute arbitrary third-party code by design and, as such, typically have informal and loosely defined boundaries between expected and unexpected code execution. This is especially true in packaging ecosystems like Homebrew, where the "carrier" format for packages (formulae) is itself executable code (Ruby scripts, in Homebrew's case).

**Third-party tap risks**:

1. **Minimal oversight**: Unlike homebrew/core, third-party taps lack rigorous review
2. **Arbitrary code execution**: Formulas can run any Ruby code during installation
3. **Past incidents**: April 2021 vulnerability in review-cask-pr allowed auto-merging malicious casks
4. **Shared package conflicts**: Wildcards in `uninstall pkgutil` can affect multiple apps

**Homebrew's malware policy**:

> In the world of software there are bad actors that bundle malware with their apps. Even so, Homebrew Cask has decided it will not be an active gatekeeper (macOS already has one) and users are expected to know about the software they are installing.

**Best practices for tsuku**:
- Warn users when installing from third-party taps
- Display tap source repository URL
- Show last-updated date for taps
- Consider allowlist of "trusted" taps (e.g., mongodb/brew, azure/functions)
- Never auto-tap without user confirmation

### 2.5 Tap Namespacing

**Global uniqueness**:
- **Formulas**: Can have same name across taps (e.g., `vim` in core and `user/tap/vim`)
- **Casks**: Must have globally unique names to avoid clashes

**Disambiguation**:
```bash
brew install vim              # ambiguous if exists in multiple taps
brew install homebrew/core/vim  # explicit
brew install user/tap/vim       # explicit
```

**Namespace management**:
- `formula_renames.json` tracks formula renames within a tap
- `tap_migrations.json` handles cross-tap migrations (e.g., formula moved from core to third-party tap)
- Homebrew automatically handles migrations on `brew update`

---

## 3. Linuxbrew / Homebrew on Linux

### 3.1 How Formulas Differ on Linux

**Historical context**:
- Originally separate Linuxbrew project
- Merged into Homebrew proper (unified homebrew-core)
- Single repository now serves both macOS and Linux

**Platform-conditional blocks** (`on_macos`, `on_linux`):

> Often, formulae need different dependencies, resources, patches, conflicts, deprecations or keg_only statuses on different OSes and architectures. In these cases, the components can be nested inside `on_macos`, `on_linux`, `on_arm` or `on_intel` blocks.

**Example - Linux-only dependency**:
```ruby
class MyFormula < Formula
  url "https://example.com/source.tar.gz"

  depends_on "cmake" => :build

  on_linux do
    depends_on "gcc"  # Linux needs gcc, macOS uses system clang
  end

  on_macos do
    depends_on "llvm"  # macOS-specific
  end
end
```

**Example - Different URLs**:
```ruby
on_macos do
  if Hardware::CPU.arm?
    url "https://fake.url/macos/arm"
  else
    url "https://fake.url/macos/intel"
  end
end

on_linux do
  url "https://fake.url/linux/all"
end
```

### 3.2 Linux-Specific Dependencies

**Common patterns**:

1. **Compiler differences**:
   - macOS: System LLVM/clang preferred
   - Linux: GCC (Homebrew's Linux CI uses GCC 12)

2. **System library philosophy**:
   - macOS: Uses system libraries (gettext, curl, ncurses)
   - Linux: Ships all libraries (for Docker, diverse distributions)

> Homebrew does not use any libraries provided by your host system, except glibc and gcc if they are new enough. Homebrew can install its own current versions of glibc and gcc for older distributions of Linux.

3. **Linux-specific depends_on**:
```ruby
depends_on "linuxbrew/xorg/libx11" if OS.linux?
depends_on "systemd" if OS.linux?
```

### 3.3 Bottle Availability on Linux

**Build environment differences**:

> Unlike macOS, Homebrew does not use a sandbox when building on Linux, so formulae may install outside the Homebrew prefix.

**Bottle differences**:
- **macOS bottles**: Built with LLVM/clang in sandboxed environment
- **Linux bottles**: Built with GCC, no sandbox
- **Architecture support**:
  - macOS: x86_64 (Intel), arm64 (Apple Silicon) - both Tier 1
  - Linux: x86_64 (Tier 1), arm64/aarch64 (promoted to Tier 1), arm32 (Tier 3, no bottles)

**Cross-compilation limitations**:

> It is not possible to cross-compile: each platform has its own build machine.

**Installation prefixes**:
- macOS Intel: `/usr/local`
- macOS Apple Silicon: `/opt/homebrew`
- Linux: `/home/linuxbrew/.linuxbrew`

> The prefix `/home/linuxbrew/.linuxbrew` was chosen so that users without admin access can still benefit from precompiled binaries via a linuxbrew role account.

### 3.4 Platform-Conditional Code in Formulas

**Available conditional blocks**:
- `on_macos` / `on_linux`
- `on_arm` / `on_intel`
- `on_monterey`, `on_ventura`, etc. (macOS version-specific)
- Combined: `on_monterey :or_older`, `on_system :linux, macos: :big_sur_or_newer`

**Example - Platform-specific services**:
```ruby
service do
  run macos: [opt_bin/"macos_script", "standalone"],
      linux: var/"special_linux_script"
end
```

**Example - OS-specific patches**:
```ruby
on_macos do
  patch do
    url "https://example.com/macos-fix.patch"
    sha256 "abc123..."
  end
end

on_linux do
  patch :DATA  # embedded patch at end of file
end
```

**Implications for tsuku**:
- LLM must parse conditional blocks
- May need to generate separate tsuku recipes for macOS vs Linux
- Or add platform conditionals to tsuku recipe format

---

## 4. Scope Implications for tsuku

### 4.1 Which Categories Should Be Prioritized?

**Recommendation**: Core formulas (macOS) → Casks → Third-party taps

#### Phase 1: Core Formulas (macOS)
**Why first:**
- Largest user impact (developers need CLI tools)
- Well-documented, consistent patterns
- Extensive test coverage (homebrew/core has strict CI)
- Open-source only (can verify builds)

**Challenges:**
- Complex build logic (LLM must understand compilation)
- Dependencies require topological sorting
- May need manual verification for complex formulas

#### Phase 2: Casks
**Why second:**
- Simpler DSL (no build logic)
- High user value (GUI apps)
- Declarative, predictable structure

**Challenges:**
- macOS-specific (tsuku targets Linux too)
- `pkg` installers run arbitrary code
- Uninstall tracking may not map to tsuku's model

#### Phase 3: Third-Party Taps (Selective)
**Why last:**
- Security concerns (minimal oversight)
- Quality varies widely
- User can always tap manually

**Approach:**
- Allowlist "trusted" taps (mongodb/brew, azure/functions, etc.)
- Require user confirmation before converting from third-party taps
- Display security warnings

### 4.2 Percentage of Tools in Each Category

**Estimated distribution** (based on research, exact numbers not publicly documented):

| Category | Estimated Count | Percentage |
|----------|----------------|------------|
| Core formulas (homebrew/core) | ~7,000 | ~40% |
| Casks (homebrew/cask) | ~5,000 | ~30% |
| Third-party taps | ~5,000+ | ~30% |
| **Total** | **~17,000+** | **100%** |

**Notes**:
- Exact counts unavailable (Homebrew doesn't publish statistics)
- Third-party taps are fragmented (hard to count)
- Core formulas have highest install rate (analytics show top 100 are core)

**Install analytics insights**:
- Top 100 most-installed packages: Almost entirely core formulas
- Casks: High install rates for developer tools (VS Code, Docker, Chrome)
- Third-party taps: Long tail, low individual install rates

**Recommendation for tsuku**:
- **Phase 1 target**: Top 500 core formulas (covers ~80% of installs)
- **Phase 2 target**: Top 200 casks (covers ~60% of GUI app installs)
- **Phase 3**: On-demand conversion for third-party taps

### 4.3 Complexity Ranking

**Ranked from simplest to most complex**:

#### 1. Casks (Simplest)
- **Complexity score**: Low
- **Why**: Declarative DSL, no build logic, predictable patterns
- **LLM conversion difficulty**: Easy
- **Example**: Visual Studio Code cask (30 lines, 8 stanzas)

#### 2. Core Formulas (Medium)
- **Complexity score**: Medium to High
- **Why**: Build instructions, dependency resolution, conditional logic
- **LLM conversion difficulty**: Medium
- **Variability**: Simple formulas (curl, wget) vs complex (gcc, llvm)
- **Example**: Simple formula (jq) - 20 lines, medium formula (python) - 200+ lines

#### 3. Third-Party Taps (Variable, Often High)
- **Complexity score**: High (unpredictable)
- **Why**: No standards, arbitrary Ruby code, poor documentation
- **LLM conversion difficulty**: Hard
- **Security risk**: High
- **Example**: Vendor taps may have custom build logic, proprietary dependencies

**Complexity factors**:
- Number of dependencies
- Conditional platform logic
- Custom build procedures
- Patches and workarounds
- Test complexity

---

## Concrete Examples

### Example 1: Simple Cask (Google Chrome)

```ruby
cask "google-chrome" do
  version :latest
  sha256 :no_check

  url "https://dl.google.com/chrome/mac/stable/GGRO/googlechrome.dmg"
  name "Google Chrome"
  desc "Web browser"
  homepage "https://www.google.com/chrome/"

  auto_updates true

  app "Google Chrome.app"

  uninstall launchctl: [
    "com.google.keystone.agent",
    "com.google.keystone.daemon"
  ]

  zap trash: [
    "~/Library/Application Support/Google/Chrome",
    "~/Library/Caches/Google/Chrome",
    "~/Library/Preferences/com.google.Chrome.plist"
  ]
end
```

**Conversion to tsuku**:
- URL is fixed (no version)
- Extract .dmg
- Move .app to ~/Applications (tsuku doesn't need /Applications)
- Cleanup requires tracking launchctl daemons (may not map)

### Example 2: Complex Cask with pkg (VirtualBox)

```ruby
cask "virtualbox" do
  version "7.0.12,159484"
  sha256 "abc123..."

  url "https://download.virtualbox.org/virtualbox/#{version.csv.first}/VirtualBox-#{version.csv.first}-#{version.csv.second}-OSX.dmg"
  name "Oracle VirtualBox"
  desc "Virtualization software"
  homepage "https://www.virtualbox.org/"

  pkg "VirtualBox.pkg"

  uninstall script: {
    executable: "VirtualBox_Uninstall.tool",
    args: ["--unattended"],
    sudo: true
  },
  pkgutil: "org.virtualbox.pkg.*"

  caveats do
    kext
  end
end
```

**Challenges for tsuku**:
- `pkg` runs macOS installer (requires macOS)
- `uninstall script` needs sudo
- Kernel extension requires user approval
- Not portable to Linux

### Example 3: Formula with Platform Conditionals (git)

```ruby
class Git < Formula
  desc "Distributed revision control system"
  homepage "https://git-scm.com"
  url "https://www.kernel.org/pub/software/scm/git/git-2.43.0.tar.xz"
  sha256 "abc123..."

  depends_on "gettext"
  depends_on "pcre2"

  on_macos do
    depends_on "curl"  # macOS uses system curl but needs headers
  end

  on_linux do
    depends_on "expat"
    depends_on "openssl@3"
  end

  def install
    # Build instructions
    system "./configure", "--prefix=#{prefix}"
    system "make", "install"
  end

  test do
    system bin/"git", "--version"
  end
end
```

**Conversion to tsuku**:
- Platform-specific dependencies
- May need separate recipes for macOS/Linux
- Or add conditional dependencies to tsuku format

---

## References

### Documentation
- [Homebrew Cask Cookbook](https://docs.brew.sh/Cask-Cookbook)
- [Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)
- [Homebrew Taps (Third-Party Repositories)](https://docs.brew.sh/Taps)
- [Homebrew on Linux](https://docs.brew.sh/Homebrew-on-Linux)
- [Homebrew Bottles (Binary Packages)](https://docs.brew.sh/Bottles)

### Security
- [Trail of Bits Homebrew Audit](https://blog.trailofbits.com/2024/07/30/our-audit-of-homebrew/)
- [Homebrew Security Incident Disclosure](https://brew.sh/2021/04/21/security-incident-disclosure/)
- [Homebrew Security Best Practices](https://guessi.github.io/posts/2025/homeberw-tips-security/)

### API References
- [Homebrew Ruby API - Cask::DSL](https://rubydoc.brew.sh/Cask/DSL.html)
- [Homebrew Formulae](https://formulae.brew.sh/)
- [Homebrew Cask Formulae](https://formulae.brew.sh/cask/)

### Guides and Examples
- [How to Create and Maintain a Tap](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap)
- [Installing Chrome, Firefox, Docker, VS Code with Homebrew](https://gist.github.com/brianjbayer/6cb3564c06c9eeda7a9645d01ccc5c2a)
- [Visual Studio Code Cask Source](https://github.com/Homebrew/homebrew-cask/blob/master/Casks/v/visual-studio-code.rb)

### Discussions
- [Homebrew Differences macOS vs Linux](https://github.com/orgs/Homebrew/discussions/2631)
- [Homebrew Cask vs Formula](https://www.unixtutorial.org/brew-cask-vs-brew-formula/)

---

## Conclusion

**For tsuku's LLM-based Homebrew builder**:

1. **Start with core formulas (macOS)**: Highest impact, well-documented, but complex build logic
2. **Add cask support next**: Simpler DSL, declarative, but macOS-specific
3. **Consider taps selectively**: Security concerns, variable quality, allowlist approach

**Key technical insights**:
- Casks are simpler than formulas (no build logic)
- Platform conditionals (`on_macos`, `on_linux`) add complexity
- Third-party taps require security warnings
- Bottles (pre-compiled binaries) differ significantly between macOS/Linux

**LLM conversion challenges**:
- Parsing Ruby DSL (especially procedural `def install` blocks)
- Handling platform conditionals
- Mapping macOS-specific features (pkg installers, launchctl)
- Security verification for third-party taps

**Next steps**:
- Build LLM prompt templates for cask → tsuku recipe conversion (simpler)
- Test with top 100 casks (Chrome, VS Code, Docker)
- Validate against real-world cask patterns
- Then tackle core formulas with build logic

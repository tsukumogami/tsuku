# Gherkin/BDD Testing Runners for CLI Tools

Research findings on Go-based and non-Go BDD testing frameworks for CLI tool testing.

## Go-Based Gherkin Runners

### 1. Godog (Official Cucumber for Go)

**Repository:** https://github.com/cucumber/godog

**Status:** Active, well-maintained (v0.15.1, released July 19, 2025)

**Community Metrics:**
- 2.6k stars, 269 forks
- 71 contributors
- 836 commits on main branch
- Used by 2,300+ projects
- Imported by 578 Go projects

**Key Features:**
- Official Cucumber BDD framework for Go
- Supports full Gherkin syntax (Given/When/Then)
- Attachments/embeddings support (v0.15.0+)
- Ambiguous step definition detection
- Cucumber tag expressions for test selection
- MIT licensed, part of Cucumber organization

**Integration with `go test`:**
Godog integrates with `go test` using Go subtests. You create a `TestFeatures` function with a `godog.TestSuite` that includes a `ScenarioInitializer` function where step definitions are registered. This approach allows running godog tests without installing a separate godog command.

Example integration:
```go
func TestFeatures(t *testing.T) {
    suite := godog.TestSuite{
        ScenarioInitializer: InitializeScenario,
        Options: &godog.Options{
            Format: "pretty",
            Paths:  []string{"features"},
        },
    }

    if suite.Run() != 0 {
        t.Fatal("non-zero status returned, failed to run feature tests")
    }
}

func InitializeScenario(ctx *godog.ScenarioContext) {
    ctx.Step(`^there are (\d+) godogs$`, thereAreGodogs)
    ctx.When(`^I eat (\d+)$`, iEat)
    ctx.Then(`^there should be (\d+) remaining$`, thereShouldBeRemaining)
}
```

**Step Definition Approach:**
- Regular expression matching for plain language steps
- Two registration styles:
  - Generic: `ctx.Step(pattern, func)`
  - Keyword-specific: `ctx.Given()`, `ctx.When()`, `ctx.Then()`
- Step functions can accept regex capture groups as parameters
- Data tables and doc strings supported

**Running Tests:**
- `go test` runs all feature tests
- `go test -test.run ^TestFeatures$/^my_scenario$` runs specific scenario
- Works with standard Go testing tools and IDE debugging
- Can leverage IDE facilities for running and debugging

**Limitations:**
- Historically ran as external binary (older versions)
- Some users report difficulty with debugging (older approach)
- No built-in context passing between steps (state must be managed externally)
- No built-in parallel execution

**CI Friendliness:** Excellent - integrates with standard `go test`, outputs TAP/JUnit formats

**Sources:**
- [GitHub - cucumber/godog](https://github.com/cucumber/godog)
- [Go Packages - godog](https://pkg.go.dev/github.com/cucumber/godog)
- [Semaphore Tutorial - How to Use Godog](https://semaphore.io/community/tutorials/how-to-use-godog-for-behavior-driven-development-in-go)
- [Medium - BDD Style Integration Testing](https://medium.com/tiket-com/go-with-cucumber-an-introduction-for-bdd-style-integration-testing-7aca2f2879e4)

---

### 2. Gocuke

**Repository:** https://github.com/regen-network/gocuke

**Status:** Maintained (v1.1.1, released March 28, 2024)

**Community Metrics:**
- 81 commits on main branch
- Smaller community than godog

**Key Features:**
- Created to address godog and gobdd limitations
- Tight integration with `*testing.T` (allows any assertion library or mocking framework)
- Context passing between steps using suites (better type safety)
- Auto-discovery of step definitions
- Full support for latest Gherkin features including rules
- Property-based testing integration via Rapid library
- User-friendly data table support
- Arbitrary-precision numeric types (big integers, decimals)
- Parallel execution by default

**Integration with `go test`:**
Excellent - designed specifically for tight integration with Go's standard testing framework rather than running as an external tool. Uses `*testing.T` directly.

**Step Definition Approach:**
- Auto-discovery with minimal configuration
- Suite-based context for type-safe state management
- Regex pattern matching similar to godog

**CI Friendliness:** Excellent - native `go test` integration

**Why Created:**
- godog was not a good fit due to lack of tight `*testing.T` integration
- gobdd needed significant updates for newer cucumber/gherkin-go APIs
- Better debugging experience than godog

**Sources:**
- [GitHub - regen-network/gocuke](https://github.com/regen-network/gocuke)
- [Go Packages - gocuke](https://pkg.go.dev/github.com/regen-network/gocuke)

---

### 3. Gobdd

**Repository:** https://github.com/go-bdd/gobdd

**Status:** Active (v1.1.4, released July 21, 2025)

**Community Metrics:**
- 138 commits
- Recent maintenance activity

**Key Features:**
- Native Go testing integration (runs within standard `go test`)
- Debugging with breakpoints (addresses godog limitation)
- Accurate test metrics
- Code style checker recognition
- Build constraints support
- Context object passed to steps (carries data between steps, concurrent-safe)
- Regex-based step pattern matching
- Stable API since v1.0

**Integration with `go test`:**
Excellent - built as an extension to Go's built-in testing library using `testing.T`. No external binary required.

**Step Definition Approach:**
- Context-based state management
- Steps receive dedicated context object
- No need for global variables (improves maintainability)
- Regex pattern matching

**CI Friendliness:** Excellent - standard `go test` integration

**Why Created:**
Specifically to address godog disadvantages:
- No debugging (breakpoints) in tests
- No context in steps (state stored elsewhere)
- Runs as external process

**Sources:**
- [GitHub - go-bdd/gobdd](https://github.com/go-bdd/gobdd)
- [Go Packages - gobdd](https://pkg.go.dev/github.com/go-bdd/gobdd)
- [Alice GG - BDD in Golang](https://alicegg.tech/2019/03/09/gobdd.html)
- [GoBDD - Developer 2.0](https://developer20.com/projects/gobdd/)

---

### Comparison Summary: Godog vs Gocuke vs Gobdd

| Feature | Godog | Gocuke | Gobdd |
|---------|-------|--------|-------|
| **Latest Version** | v0.15.1 (July 2025) | v1.1.1 (March 2024) | v1.1.4 (July 2025) |
| **Community Size** | Large (2.6k stars, 578 imports) | Small | Small |
| **`go test` Integration** | Via subtests (good) | Native `*testing.T` (excellent) | Native `testing.T` (excellent) |
| **Debugging** | Possible but historically difficult | Excellent | Excellent (key feature) |
| **Context Passing** | Manual (external state) | Suite-based (type-safe) | Context object (built-in) |
| **Parallel Execution** | No built-in support | Default behavior | Standard `go test` |
| **Step Auto-discovery** | No | Yes | No (manual registration) |
| **Official Cucumber** | Yes | No | No |
| **Gherkin Features** | Full (with rules) | Full (with rules) | Standard |
| **Maturity** | Very mature | Moderate | Moderate |

**Recommendation for Tsuku:**
- **Godog** if you want official Cucumber compatibility and large community
- **Gocuke** if you prioritize auto-discovery, type safety, and property-based testing
- **Gobdd** if you want simple, debuggable BDD tests with context management

All three work well with CI. Gocuke and gobdd offer better developer experience for debugging.

---

## Other Go BDD Frameworks (Non-Gherkin)

### Ginkgo
**Repository:** https://github.com/onsi/ginkgo

- Popular BDD-style testing framework for Go
- Not Gherkin-based (uses Go DSL)
- Best paired with Gomega matcher library
- Matcher-agnostic design
- More complicated setup than GoConvey
- Better test organization and more features
- More mature than GoConvey

### GoConvey
**Repository:** https://github.com/smartystreets/goconvey

- BDD framework with Go DSL (not Gherkin)
- Keeps test description and code in one place
- Works with `go test`
- Terminal and browser interface
- Great reporting

---

## Non-Go CLI Testing Frameworks

### 1. Behave (Python)

**Repository:** https://github.com/behave/behave

**PyPI:** https://pypi.org/project/behave/

**Status:** Active (documentation updated for 2026)

**Key Features:**
- BDD framework using Gherkin language
- Python step definitions
- Plain English scenario definitions
- Scenario outlines and backgrounds for test reuse
- Custom command-line argument passing
- Automatic feature file detection

**Advantages:**
- Great online documentation
- Easy tutorials available
- Natural language style encourages collaboration
- Strong for bridging business requirements and code

**Limitations:**
- No built-in parallel execution (requires BehaveX or similar)
- Limited IDE support (full support only in PyCharm Professional)

**Integration Approach:**
- Feature files written in Gherkin
- Step definitions in Python modules
- Run with `behave` command
- Integrates with Selenium for web/CLI testing

**CI Friendliness:** Good - command-line driven, standard exit codes

**Use Case for CLI Testing:**
Excellent for teams with Python expertise or polyglot projects. Natural language scenarios make it easy for non-technical stakeholders to understand test coverage.

**Sources:**
- [GitHub - behave/behave](https://github.com/behave/behave)
- [Pytest BDD vs Behave Comparison](https://codoid.com/automation-testing/pytest-bdd-vs-behave-pick-the-best-python-bdd-framework/)
- [Python Behave Complete Guide](https://www.testmu.ai/learning-hub/python-behave/)
- [behave documentation](https://behave.readthedocs.io/)

---

### 2. Bats-core (Bash)

**Repository:** https://github.com/bats-core/bats-core

**Documentation:** https://bats-core.readthedocs.io/

**Status:** Active

**Key Features:**
- Bash Automated Testing System
- TAP-compliant (Test Anything Protocol)
- Tests any UNIX program
- Bash script syntax with special test case definitions
- Test isolation via separate processes
- No built-in assertions (use bats-assert extension)

**Advantages:**
- Simple, familiar syntax for shell script developers
- Native to Unix/Linux environments
- Each test runs in isolated process
- Works with any command-line tool

**Limitations:**
- Bash-specific (requires Bash 3.2+)
- Not BDD/Gherkin based
- Basic assertion library

**Integration Approach:**
- Test files are Bash scripts (`.bats` extension)
- Run with `bats` command
- Each `@test` block is a test case
- Standard Unix exit codes

**CI Friendliness:** Excellent - TAP output, shell-native, minimal dependencies

**Use Case for CLI Testing:**
Perfect for testing Unix command-line tools when you don't need natural language scenarios. Very lightweight and fast.

**Example:**
```bash
@test "addition using bc" {
  result="$(echo 2+2 | bc)"
  [ "$result" -eq 4 ]
}
```

**Sources:**
- [GitHub - bats-core/bats-core](https://github.com/bats-core/bats-core)
- [Testing Bash with BATS - Opensource.com](https://opensource.com/article/19/2/testing-bash-bats)
- [bats-core documentation](https://bats-core.readthedocs.io/)
- [End-to-End CLI Testing with BATS](https://pkaramol.medium.com/end-to-end-command-line-tool-testing-with-bats-and-auto-expect-7a4ffb19336d)

---

### 3. ShellSpec (Shell)

**Repository:** https://github.com/shellspec/shellspec

**Website:** https://shellspec.info/

**Status:** Active

**Key Features:**
- BDD unit testing framework for POSIX shells
- Supports bash, ksh, zsh, dash, and all POSIX shells
- BDD-style DSL with Gherkin-like syntax
- Code coverage support
- Mocking capabilities
- Parameterized tests
- Parallel execution
- Shell-compatible DSL (starts with capital letters)

**Advantages:**
- Works with almost any shell
- Full BDD features (more than bats-core)
- Code coverage determination
- Better structured than basic shell testing

**Limitations:**
- Custom DSL to learn (though shell-compatible)
- Smaller community than bats-core

**Integration Approach:**
- Test files use ShellSpec DSL
- Run with `shellspec` command
- Supports various output formats

**CI Friendliness:** Excellent - designed for automation

**Use Case for CLI Testing:**
Best for teams wanting BDD-style testing for shell scripts with better features than bats-core, especially if you need POSIX shell compatibility.

**Sources:**
- [GitHub - shellspec/shellspec](https://github.com/shellspec/shellspec)
- [ShellSpec comparison page](https://shellspec.info/comparison.html)
- [Writing Unit-Tests for UNIX Shells](https://honeytreelabs.com/posts/writing-unit-tests-and-mocks-for-unix-shells/)

---

### 4. Robot Framework (Python)

**Website:** https://robotframework.org/

**Status:** Very mature

**Key Features:**
- Generic test automation framework
- Keyword-driven approach
- Python-based (also Jython/IronPython)
- Tests many targets (web, FTP, MongoDB, Android, etc.)
- Designed for testers (readable tests)
- Extensible with custom libraries

**Advantages:**
- Very mature solution
- Keyword-driven makes tests readable
- Multi-platform, multi-purpose
- Strong community

**Limitations:**
- Steeper learning curve
- Significant time needed for advanced framework syntax
- Heavier than specialized CLI tools

**Integration Approach:**
- Keyword-based test files
- Run with `robot` command
- Command-line options for test selection

**CI Friendliness:** Good - command-line driven, various output formats

**Use Case for CLI Testing:**
Appropriate for organizations already using Robot Framework or needing multi-domain testing (CLI + web + API). Overkill for CLI-only testing.

**Sources:**
- [ShellSpec comparison](https://shellspec.info/comparison.html)
- [Alternatives to Robot Framework](https://theembeddedkit.io/blog/alternative-to-robot-framework/)
- [Research on Python ATDD frameworks](https://gist.github.com/gregelin/8210441)

---

## Key Comparison Points

### Integration with `go test`

| Framework | Integration | Notes |
|-----------|-------------|-------|
| **Godog** | Via subtests | Good - can run with `go test`, IDE support |
| **Gocuke** | Native `*testing.T` | Excellent - direct integration, full debugging |
| **Gobdd** | Native `testing.T` | Excellent - built as testing extension |
| **Behave** | N/A | Separate `behave` command |
| **Bats-core** | N/A | Separate `bats` command |
| **ShellSpec** | N/A | Separate `shellspec` command |
| **Robot** | N/A | Separate `robot` command |

### Step Definition Approach

| Framework | Approach | Complexity |
|-----------|----------|------------|
| **Godog** | Regex + manual state | Moderate - external state management |
| **Gocuke** | Regex + suite context | Low - type-safe, auto-discovery |
| **Gobdd** | Regex + context object | Low - built-in context passing |
| **Behave** | Regex + Python decorators | Low - Pythonic |
| **Bats-core** | N/A (not Gherkin) | Very low - pure Bash |
| **ShellSpec** | Custom DSL | Moderate - new syntax to learn |
| **Robot** | Keywords | High - keyword library management |

### CI Friendliness

| Framework | CI Rating | Output Formats |
|-----------|-----------|----------------|
| **Godog** | Excellent | TAP, JUnit, pretty, progress, cucumber |
| **Gocuke** | Excellent | Standard `go test` formats |
| **Gobdd** | Excellent | Standard `go test` formats |
| **Behave** | Good | Multiple (plain, JUnit, JSON) |
| **Bats-core** | Excellent | TAP (highly compatible) |
| **ShellSpec** | Excellent | TAP, JUnit, documentation |
| **Robot** | Good | HTML, XML, log files |

All frameworks support CI integration well. Go-based options integrate seamlessly with existing Go toolchains.

### Community Activity (as of January 2026)

| Framework | Activity Level | Latest Release |
|-----------|----------------|----------------|
| **Godog** | Very active | v0.15.1 (July 2025) |
| **Gocuke** | Moderate | v1.1.1 (March 2024) |
| **Gobdd** | Active | v1.1.4 (July 2025) |
| **Behave** | Active | Updated 2026 |
| **Bats-core** | Active | Ongoing |
| **ShellSpec** | Active | Ongoing |
| **Robot** | Very active | Ongoing |

Godog has the largest Go community (2.6k stars, 578 imports). Behave and Robot Framework have large Python communities. Bats-core is standard in Unix/Linux testing.

---

## Recommendations for Tsuku

### For Pure Go Integration:

1. **Godog** - if you need official Cucumber compatibility, large community support, or plan to share feature files with other Cucumber implementations
2. **Gocuke** - if you want modern Go idioms, auto-discovery, and type-safe context management
3. **Gobdd** - if you prioritize debugging experience and simple context handling

### For Lightweight CLI Testing:

1. **Bats-core** - if you want minimal dependencies and native Unix integration
2. **ShellSpec** - if you need BDD structure with shell compatibility

### For Polyglot Projects:

1. **Behave** - if your team has Python expertise and wants natural language scenarios
2. **Robot Framework** - if you need multi-domain testing (CLI + web + API)

### Decision Factors:

- **Team expertise**: Go developers → Go-based options; Python developers → Behave
- **Debugging needs**: Gocuke/Gobdd > Godog
- **Natural language requirements**: Gherkin-based (Godog, Gocuke, Gobdd, Behave) vs. code-based (Bats, ShellSpec)
- **Community size**: Godog (Go), Behave/Robot (Python), Bats (Unix)
- **CI pipeline**: All options are CI-friendly; Go options integrate with existing `go test` workflows

For a Go project like Tsuku, **Godog** offers the best combination of maturity, community support, and feature completeness, though **Gocuke** or **Gobdd** may provide better developer experience for debugging and testing.

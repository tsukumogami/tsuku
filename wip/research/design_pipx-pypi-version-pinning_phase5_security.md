# Security Review: DESIGN-pipx-pypi-version-pinning

**Design**: `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku/docs/designs/DESIGN-pipx-pypi-version-pinning.md`
**Reviewer**: Security review (Phase 5)
**Date**: 2026-04-28

## Verification Performed

Before analyzing the dimensions, the design's structural claims were checked against the live codebase:

- **`getPythonVersion(pythonPath string) (string, error)`** -- exists at
  `internal/actions/pip_exec.go:336`. It runs `exec.Command(pythonPath, "--version")`,
  trims output, and splits on a space. There is **no shell**, no expansion,
  no argv concatenation -- `exec.Command` invokes the binary directly with a
  fixed `--version` literal arg. Confirms the design's assumption.
- **`ResolvePythonStandalone()`** -- defined at `internal/actions/util.go:353`.
  Reads `$TSUKU_HOME/tools/`, filters entries to those whose name starts with
  `python-standalone-`, sorts lexicographically, and returns
  `<toolsDir>/<latest>/bin/python3` only after `os.Stat` confirms the file
  exists and has at least one executable bit. The path is constructed by
  `filepath.Join` against tsuku-controlled directory entries; no user input
  is interpolated. Confirms the design's assumption.
- **Existing call site** -- `pipx_install.Decompose` already calls
  `ResolvePythonStandalone()` (line 318) and `getPythonVersion(pythonPath)`
  (line 337). The design does not introduce a new subprocess invocation
  surface; it reorders an existing one.
- **`*ResolverError` typed-error pattern** -- defined at
  `internal/version/errors.go:42`. Has `Type`, `Source`, `Message`, `Err`
  fields. The PyPI provider already returns it from `ResolvePyPI`
  (`internal/version/pypi.go:37`). The design's plan to add a new
  classification value and return it from `ResolveLatestCompatibleWith`
  follows the established pattern.
- **Minor naming inconsistency.** The design's "Error contract" snippet
  declares `const ErrTypeNoCompatibleRelease ResolverErrorType = "no_compatible_release"`,
  but the actual codebase uses `type ErrorType int` with iota. The new
  constant must be added as an `ErrorType` int, not a string-typed
  pseudo-enum. This is a typo in the design rather than a security issue,
  but should be corrected before implementation.
- **`pypiPackageInfo`** at `internal/version/pypi.go:22` currently models
  releases as `map[string][]struct{}` (file array intentionally empty
  because today nothing reads file metadata). The design's modification
  must extend this to capture per-release `requires_python`. Either the
  release-level top-level `requires_python` (released 2018, available in
  the package's `info.requires_python` aggregate) or the per-file
  `requires_python` field. This is well-trodden PyPI JSON territory.
- **PyPI response is already capped** at 10 MB
  (`maxPyPIResponseSize = 10 * 1024 * 1024`,
  `internal/version/pypi.go:18`). This bound flows directly into the new
  parser's untrusted-input budget.

## Dimension Analysis

### 1. External artifact handling (PyPI requires_python parsing)

**Applies.** The new in-tree PEP 440 evaluator consumes
`requires_python` strings from PyPI's JSON response. PyPI is treated as
untrusted by the existing provider; the design correctly continues that
posture.

**Risk surface:**

- **Memory exhaustion.** The PyPI response is already capped at 10 MB
  by `maxPyPIResponseSize`. A malicious response could pack many
  releases, each with a large `requires_python` string. The design
  walks releases newest-first and stops at the first compatible
  release; in the pathological case of "all incompatible" the walk is
  bounded by the number of releases (in practice <2000 for the largest
  PyPI packages). Per-clause length is not explicitly bounded by the
  design.
  - **Severity: low.** The 10 MB cap on the response, plus the lack of
    recursion in the parser, makes worst-case memory linear in the
    response size already paid for. No amplification factor.
  - **Mitigation: add a clause-length cap.** Recommend the parser
    reject any single specifier longer than, say, 256 bytes (real-world
    max observed is ~64). This makes the rejection point obvious and
    avoids the parser doing useful work on 10 MB of garbage in one
    field. Trivial: one `len(s) > 256` check at the top of
    `ParseSpecifier`.
- **Algorithmic complexity / regex backtracking.** The design states
  byte-level scanning (no regex). Confirmed by reading the design's
  parser interface: tokenize on commas, `TrimSpace`, longest-prefix
  operator match, then integer parsing of dot-separated segments. No
  recursion. No backreferences. Worst-case cost is linear in input
  length.
  - **Severity: N/A** if implemented as designed. **Severity: medium**
    if a future maintainer reaches for `regexp.MustCompile` with
    `.*` quantifiers. Recommend a comment in the parser source forbidding
    regex use, since Go's `regexp` is RE2 (no backtracking) but a
    future contributor unfamiliar with the constraint might pull in a
    third-party PCRE wrapper.
- **Incorrect filtering.** A crafted `requires_python` like
  `>=999999999.0` could cause the filter to skip every release.
  Outcome: the typed error fires (Decision 3). No code execution, no
  install. This is the design's intended behavior.
  - **Severity: low.** The "attack" is "make tsuku produce an error
    instead of installing the wrong version." Not a meaningful attack
    surface; install never runs.
- **Integer overflow on segment parsing.** Each segment is parsed via
  `strconv.Atoi` or equivalent. A clause like `>=99999999999999999999.0`
  would overflow Go's int. The design says "1- to 4-segment integer";
  it does not say the integers are bounded.
  - **Severity: low.** Outcome on overflow is a parse error or wrong
    comparison; either way the release is treated as incompatible per
    the design's "skip on parse failure" rule. No memory unsafety in
    Go. **Mitigation: clamp segments to e.g. 6 digits** or reject
    versions where any segment exceeds `math.MaxInt32`. Cheap defense
    in depth.
- **Unicode confusables.** PyPI's JSON is UTF-8. The parser scans
  bytes. If the parser uses byte-level comparison for ASCII operators
  (`>=`, `<`, etc.) and tolerates non-ASCII bytes inside numeric
  segments, a clause like `>=3.10​` (zero-width space) could
  parse differently than expected.
  - **Severity: low.** PyPI publishes ASCII-only `requires_python` in
    practice; observed format variance is whitespace, operator
    ordering, and four-segment versions, not Unicode. **Mitigation:
    reject any byte > 0x7F in the input** with `ErrMalformed`. One
    loop, one constant. Closes the door before anyone walks through it.

**Verdict:** The design's claim of linear cost, no backtracking, and
byte-level scanning is realistic. The defense-in-depth recommendations
above are cheap to add in Phase 1 and would harden the parser against
malformed-input concerns that are otherwise theoretical.

### 2. Permission scope (subprocess invocation, path handling)

**Applies, no new risk.**

- `pythonPath` is produced solely by `ResolvePythonStandalone()`, which
  reads tsuku-owned directories (`$TSUKU_HOME/tools/python-standalone-*`)
  and constructs the binary path via `filepath.Join`. The path is not
  user-controlled.
- `getPythonVersion` calls `exec.Command(pythonPath, "--version")`.
  This bypasses any shell; argv is `[pythonPath, "--version"]` with no
  user input flowing into either element. No injection vector.
- The same call already exists in `pipx_install.Decompose`. The design
  reuses it; it does not add a new exec point.
- Path-traversal risk: `python-standalone-*` matches only directory
  names tsuku itself created during install; an attacker who can write
  arbitrary directory names under `$TSUKU_HOME/tools/` already owns
  tsuku.

**Severity: N/A.** No new permission scope, no new subprocess surface,
no new path-traversal vector.

### 3. Supply chain / dependency trust

**Applies.** The design rejects `aquasecurity/go-pep440-version` and
writes ~250 LOC in-tree.

**Trade-off:**

- **For in-tree:** Avoids three new direct deps
  (`aquasecurity/go-version`, `golang.org/x/xerrors`,
  `stretchr/testify`) plus their transitive closure. Each new dep is a
  new compromise vector (typosquat, maintainer takeover, CVE in a
  transitive). Tsuku's stated convention favors stdlib + minimal deps;
  ~250 LOC of focused code with table-driven tests is a reasonable
  trade for the marginal correctness gain of the external library.
- **Against in-tree:** A new in-tree parser is one more thing for tsuku
  to maintain. PyPI metadata format drift could surface unsupported
  clauses that were already handled by the external library. This is
  an operational cost, not a security cost.
- **Against the external lib specifically:** `aquasecurity/go-pep440-version`
  pulls `stretchr/testify` as a transitive. Testify's surface area is
  large and unrelated to PEP 440 parsing; pulling it for a parser is
  disproportionate to the value.

**Severity: low.** The decision is defensible from a security
posture: smaller dependency surface, no new transitive trust, parser
operates on bounded input under tsuku's own test coverage. The design
should commit to keeping the parser strictly local (no public re-export
that would invite external callers and complicate API stability).

### 4. Data exposure (error messages)

**Applies, no risk.**

The error template is:

```
pypi resolver: no release of <package> is compatible with bundled Python <X.Y> (latest is <V>, requires Python <Z>)
```

- `<package>` -- from the recipe TOML, public.
- `<X.Y>` -- bundled Python's major.minor, public.
- `<V>` -- latest PyPI version of the package, public.
- `<Z>` -- the package's published `Requires-Python`, public.

No paths, no secrets, no credentials, no environment values. All four
fields are values an attacker could already obtain by hitting PyPI
directly. The message is plain ASCII per the wording template.

**Severity: N/A.** Confirmed: no sensitive info leakage.

### 5. Bypass risk (user pinning)

**Applies, intentional design choice -- safe.**

User pins (`tsuku install foo@x.y`) flow through `Executor.ResolveVersion`
with the user's constraint and bypass `ResolveLatestCompatibleWith`.
This is documented in the design's Data Flow section.

The question is whether a recipe could exploit this pathway to install
an incompatible version silently. The answer is no, because:

- **Recipes do not declare versions** in the TOML (the design
  explicitly forbids this in the "Decisions Already Made" section: "No
  manual recipe-level constraints"). Recipe authors cannot encode a
  version pin that bypasses the filter.
- **User pins are CLI-side**: they require a human to type `@x.y`. A
  recipe cannot inject CLI flags.
- **The bypass behavior matches existing pip behavior**: an explicit
  pin is authoritative even if pip would otherwise refuse. tsuku
  preserves the same authority model.
- The error from `pip download` on an incompatible explicit pin
  surfaces normally (pip's exit code), so the user gets feedback even
  in the bypass path.

**Severity: N/A.** The design's bypass is user-driven, not
recipe-driven. No silent-install vector.

### 6. Failure mode security (DoS via no-compatible-release)

**Applies, low risk.**

A malicious PyPI response could declare `requires_python = ">=99"` on
every release, causing tsuku to always return the typed error. Effect:
the install fails with a clear stderr message; `tsuku eval` exits
non-zero.

- **DoS scope:** Per-package, per-recipe. The user can `tsuku install`
  any other package successfully. The "denial of service" is "this
  one tool can't be installed via tsuku from this PyPI mirror."
- **Threat model:** An attacker who controls PyPI's response for a
  given package already has full supply-chain compromise of that
  package -- they could publish a malicious version, or refuse to
  serve any version. Causing tsuku to error is the *least* harmful
  outcome of that compromise.
- **Resource cost on tsuku:** The candidate walk is bounded by release
  count (per-package, in practice <2000) and per-clause parse cost
  (linear in clause length, capped by the recommended 256-byte cap).
  No quadratic walk, no caching that could be poisoned across
  packages.

**Severity: low.** The "attack" is "tsuku tells the user clearly that
no compatible release exists" which is the correct behavior. There is
no DoS amplification, no cross-package poisoning, no resource
exhaustion path. The pre-flight error is strictly safer than the
current state (where pip emits its own enumeration after running).

### Tsuku-specific dimensions

- **Recipe trust model.** Confirmed unchanged. Recipes do not declare
  Python version, do not declare specifier strings, and gain no new
  privilege. The auto-filter consumes only PyPI metadata.
- **Network trust.** Confirmed: same HTTPS endpoint, same JSON
  response, two more fields read. No new endpoint.
- **File system writes.** Confirmed: the new code path is read-only
  against PyPI and the installed Python binary. No file writes
  introduced. Verified by reading the PEP 440 evaluator surface (pure
  parsing) and the new provider method (returns a struct).
- **Subprocess invocation.** Confirmed: `getPythonVersion` is already
  invoked in `pipx_install.Decompose` today (line 337). The design
  hoists it earlier in the function but does not introduce a new exec
  call. No new subprocess surface.

## Recommended Outcome

**Option 2 -- Document considerations** with three small parser
hardening notes added to Phase 1's deliverables.

The design's core security posture is sound: no new endpoints, no new
subprocesses, no new file writes, no new credential surface, no new
recipe privilege. The PEP 440 parser is a well-bounded byte-level
scanner over a 10 MB-capped input. The typed pre-flight error replaces
an opaque pip failure with a tighter signal.

Three defense-in-depth hardening items should be folded into the
parser implementation rather than treated as design changes:

1. **Per-clause length cap** (e.g., 256 bytes) at `ParseSpecifier`
   entry. Real-world clauses are <64 bytes; rejecting longer ones
   forces an obvious parse error before the byte scanner does any
   work.
2. **ASCII-only input check.** Reject any byte > 0x7F as malformed.
   PyPI's `requires_python` is ASCII in practice; this closes Unicode
   confusable concerns before they exist.
3. **Segment-magnitude cap.** Reject version segments > 6 digits (or
   `math.MaxInt32`) as malformed. Avoids overflow in integer
   comparison.

These are one-line checks each. They do not change the design's
architecture, the chosen operator set, or the failure semantics. They
should be called out in the design's Security Considerations section
and added to Phase 1's test table.

The design's existing Security Considerations section is well-shaped
and accurate; replace it with the refined version below.

### Suggested Security Considerations Section (replacement text)

> ## Security Considerations
>
> The change consumes additional fields from PyPI's existing JSON
> response and runs new in-tree string parsing. Concrete review:
>
> - **Untrusted input source.** PyPI metadata is fetched from an
>   external service over HTTPS. The existing provider already
>   treats this as untrusted (parses JSON, validates fields,
>   returns typed errors on malformed responses). Adding
>   `requires_python` retention does not introduce a new input
>   source -- it consumes one more field from the same response.
>   The response is capped at 10 MB
>   (`maxPyPIResponseSize`, `internal/version/pypi.go:18`); the
>   parser inherits that bound on its untrusted-input budget.
>
> - **PEP 440 parser as attack surface.** The parser uses byte-level
>   scanning (no regex, no recursion). Mitigations beyond the
>   bounded grammar:
>   - **Per-clause length cap of 256 bytes** at `ParseSpecifier`
>     entry. Real-world clauses are <64 bytes; longer inputs are
>     rejected as malformed.
>   - **ASCII-only validation.** Any byte > 0x7F is rejected. PyPI's
>     `requires_python` is ASCII in practice; this prevents Unicode
>     confusable inputs from reaching the operator matcher.
>   - **Segment-magnitude cap.** Version segments above 6 digits (or
>     `math.MaxInt32`) are rejected as malformed, preventing
>     integer overflow in comparison.
>   - Output is `[]int` plus a small struct -- no execution path,
>     no eval. Worst-case parser cost is linear in input length.
>
> - **No new secrets, no new credentials, no new file writes.** The
>   new code path is read-only against PyPI and the installed
>   Python binary.
>
> - **Subprocess call to `python --version`.** Already done today
>   by `getPythonVersion(pythonPath)`. The path comes from
>   `ResolvePythonStandalone()`, which constructs the path inside
>   `$TSUKU_HOME/tools/python-standalone-*` via `filepath.Join`
>   over directories tsuku itself creates. `exec.Command` invokes
>   the binary directly with a literal `--version` arg -- no shell,
>   no argv injection. Not a new subprocess invocation surface.
>
> - **Error messages.** The typed error includes the package name,
>   bundled Python version, latest release, and `Requires-Python`.
>   None of these fields contain secrets or paths; all are public
>   PyPI metadata. The wording template is plain ASCII.
>
> - **Recipe-side trust model unchanged.** Recipes do not declare
>   Python version, do not declare specifier strings, and gain no
>   new privilege. The auto-filter consumes only upstream PyPI
>   metadata. User pins (`tsuku install foo@x.y`) bypass the filter
>   intentionally; pins are a CLI-side mechanism, so a recipe
>   cannot inject one.
>
> - **DoS via crafted `requires_python`.** A malicious PyPI
>   response declaring "no release is compatible" causes the
>   typed pre-flight error to fire and tsuku to exit non-zero
>   per-package. The "attack" is per-package and per-mirror; an
>   attacker who controls PyPI for a package already has stronger
>   compromise primitives (e.g., publishing a malicious version).
>   No cross-package poisoning, no quadratic walks, no resource
>   amplification.
>
> No new privileged operations, no new credential surface, no new
> input source beyond the existing PyPI JSON response. The PEP 440
> parser is a small in-tree component with a bounded grammar,
> bounded input length, and clear failure modes.

## Summary

The design is security-clean as written, with three small parser
hardening additions recommended (length cap, ASCII validation,
segment-magnitude cap) folded into Phase 1's parser deliverables. No
new attack surface beyond two more fields read from an already-trusted
JSON response. No new subprocess, no new file writes, no new
credentials, no new recipe privilege. The bypass for user pins is
CLI-driven and matches existing pip authority semantics; recipes
cannot trigger it. The typed pre-flight error replaces an opaque pip
failure with a tighter, lower-noise signal, which is a security
improvement over today's behavior. One typo correction needed in the
design's error-contract snippet (`ResolverErrorType` should be
`ErrorType`).

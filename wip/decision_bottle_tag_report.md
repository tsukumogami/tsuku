<!-- decision:start id="homebrew-bottle-tag-selection" status="assumed" -->
### Decision: Host-aware Homebrew bottle tag selection (tsuku#2675)

**Context**

`getPlatformTag` in `internal/actions/homebrew.go` returns `arm64_sonoma` (or `sonoma` on Intel)
for every Mac, and both the execute and decompose paths use it. Homebrew stopped building Sonoma
bottles on 2026-09-10 (brew PR #23928; Sonoma is now at its lowest support level), so every
rebottled formula loses the tag and tsuku fails on it, on every macOS version. #2613 recorded the
fix as "walk down from the host's macOS version, walk down from a named `arm64_sequoia` constant for
plans made for another machine, record the choice on the step and refuse too-new bottles at
install time". The task brief for this fix adds a constraint #2613 didn't have: the fix must not
change which bottle a correctly working install already gets. Walking down from the host breaks
that constraint for every formula that still ships a Sonoma bottle, on every Sequoia or Tahoe host.

Research findings that shape the answer (each read from source):
- Homebrew's own rule (`find_older_compatible_tag`): exact host tag, then the first same-arch tag at
  or below the host version. Never newer, never cross-arch. Its "newest first" order comes from
  how bottle blocks are written, not from sorting, so tsuku has to sort manifest entries itself.
  Tags run big_sur (11) through golden_gate (27); tahoe is 26.
- `syscall.Sysctl("kern.osproductversion")` works in tsuku's pure-Go, CGO_ENABLED=0 darwin
  binary (Go 1.25.8). Go's linker stamps SDK 12.0, so the Big Sur `10.16` compat value doesn't
  apply; Tahoe betas 1-2 returned `16.0` to old-SDK binaries, which Apple removed in beta 3.
- Every install path calls `GeneratePlan` with `OS: runtime.GOOS` set explicitly, so a host
  install can't be told from a cross-machine plan by an empty OS. `tsuku eval` and golden
  regeneration (always `--os/--arch`, on macos-15 runners) produce plans meant to be carried.
- `homebrew_relocate` reads only `formula` and has no Preflight; step params survive state.json
  round trips; cached plans are reused without a content-hash check; `ValidatePlan` runs before any
  step in `ExecutePlan`, for dependencies too, and already refuses a plan for another OS/arch. An
  optional step param needs no plan format bump, by the same precedent as `PlanVerify.Additional`.
- 16 darwin embedded goldens carry bottle steps (make, libyaml, zlib, openssl@3, patchelf,
  pkgconf, and ninja/ruby through dependencies). Code-change CI validates only linux embedded
  goldens; changed darwin-arm64 goldens are executed with `install --plan` on `macos-latest`.

**Assumptions**
- Released macOS 26.x returns `26.x` from `kern.osproductversion` to a Go binary with SDK 12.0.
  Reasoned from source and Apple forum reports, not tested on hardware. If wrong: a Tahoe host
  reads 16.x; mapping 16.x to 26 makes that harmless.
- A bottle built for macOS N runs on newer macOS (Homebrew's rule, not tested by tsuku).
- GHCR manifest entry order carries no meaning; tsuku sorts by macOS version.

**Chosen: oldest-supported-first selection up to a ceiling, a host ceiling from sysctl, and a
`bottle_tag` param checked by ValidatePlan**

1. **Selection.** From the manifest's same-arch macOS tags for the resolved version, keep those at
   or below a ceiling. Try Sonoma (the tag tsuku requests today) first, then newer tags in
   ascending order up to the ceiling, then older tags below Sonoma in descending order. The first
   present tag wins. One shared helper does this for the action and for the recipe builder.
2. **Ceiling.** Installs on the user's machine (`install`, library and dependency installs) use the
   host's macOS version from `syscall.Sysctl("kern.osproductversion")`, with `10.16` read as 11
   and `16.x` as 26. `tsuku eval`, golden files, and any plan for a different OS/arch use a named
   constant, macOS 15 (Sequoia). An unreadable host version also falls back to the constant. The
   ceiling reaches `Decompose` through a new `PlanConfig`/`EvalContext` field that install callers
   set and dependency plans inherit.
3. **Guard.** Darwin `homebrew_relocate` steps gain an optional string param `bottle_tag` naming the
   chosen tag. `ValidatePlan` refuses, before any download, a plan containing a `bottle_tag` whose
   macOS version is above the host's ("bottle for macOS 26 (arm64_tahoe) can't run on macOS 15").
   An absent param means no check, so plans written earlier, including plans stored in
   state.json, run exactly as before. No format version bump. Linux steps don't get the param.
4. **One copy.** The tag table and helpers live in `internal/actions`; the builder's duplicate
   constants and `getCurrentPlatformTag` go; `tap_parser.go`'s codename map gains tahoe and
   golden_gate.

**Rationale**

Oldest-first is the only direction that meets both the brief's constraint and the bug: every
formula that works today resolves to the same tag, and every formula that fails today only because
Sonoma is missing gets the oldest bottle the host (or the constant) allows. It never selects a
bottle newer than the ceiling, so it keeps Homebrew's safety rule while departing from its
preference order. The departure is temporary by construction: Homebrew no longer builds Sonoma
bottles, so each rebottle removes the Sonoma tag and selection moves to the same Sequoia or newer
bottle Homebrew would pick. It also keeps the golden churn to the new `bottle_tag` param on the 16
darwin embedded goldens, with no digest changes for formulae that already resolve, which matters
because the registry golden regeneration runs over the user's own network connection.

Recording on the step and checking in `ValidatePlan` puts the guard in the one function every
plan execution path already runs before touching the network, and it's the function that already
answers "does this plan fit this host".

**Alternatives Considered**
- **Newest first, Homebrew's rule (as recorded on #2613)**: matches `brew install` exactly, but
  switches the bottle for every formula that still ships a Sonoma bottle on Sequoia and Tahoe hosts,
  and changes the blob digest of nearly every darwin golden. Rejected because it breaks the "don't
  change a working install's bottle" constraint and multiplies golden regeneration for no user-visible
  fix. It stays the right choice if that constraint is dropped.
- **Newest first on the host, oldest first for plans made elsewhere**: golden files would stop
  describing what `tsuku install` does on the same machine. Rejected.
- **Host version from `sw_vers` or `SystemVersion.plist`**: a subprocess per plan, or a file
  libSystem redirects under compat mode. Rejected in favour of the sysctl.
- **Step-level or plan-level struct field, or a plan format bump**: touches `install.Plan`, both
  conversions and hashing; a bump invalidates every cached plan and every golden. Rejected.
- **Floor check in a `homebrew_relocate` Preflight**: Preflight is environment-independent param
  validation, and recipe validation calls it on Linux hosts. Rejected.

**Consequences**

Formulae missing `arm64_sonoma` install on Sequoia, Tahoe and newer; on Sonoma they fail with a
clear "no compatible bottle for macOS 14" error. The 16 darwin embedded goldens gain `bottle_tag`
and need regenerating; registry goldens gain it too at their next regeneration, and those for
formulae that failed on the Sonoma tag become generatable. Plans made by `tsuku eval` are portable
to macOS 15 and newer. A plan made before this change carries no `bottle_tag` and is not checked.
tsuku's bottle choice on newer hosts differs from Homebrew's while Sonoma bottles remain for a
formula version; that difference shrinks as formulae are rebottled.
<!-- decision:end -->

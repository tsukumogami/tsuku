# Decision A: What enforces containment during extraction, and what becomes of the existing lexical validators?

Tier: critical (security implications; effectively irreversible once the extractor is
reshaped; experts genuinely disagree -- the industry took years to converge).

This decision was settled during the exploration phase by three independent
investigations plus direct measurement. Sources:
`wip/research/explore_extract-symlink-escape_r1_lead-prior-art.md`,
`..._lead-precedent-2467.md`, `..._lead-blast-radius.md`, and probes run inline. This
report consolidates that evidence; it does not re-derive it.

## The framing that unlocks the decision

Two rules were being conflated under one set of helpers:

- **Link-content rule** -- where may a symlink *point*? Enforced today by
  `validateSymlinkTarget`. This is tsukumogami/tsuku#2275's territory.
- **Traversal rule** -- may a write *walk through* a symlink that leaves the
  destination? Enforced today by nothing. This is #2473's territory.

The bug is not that the link-content rule is too weak. It is that the link-content rule
was mistaken for a traversal rule. A symlink pointing outside the tree is harmless until
something is written through it; a symlink pointing *inside* the tree (`a -> "."`) is the
one that carries the attack. Any fix that stays in link-content space will keep missing
this, which is exactly the history below.

## Options

### A1. Route extraction I/O through `os.Root` (`os.OpenRoot(destPath)`)

Open a directory handle on the destination and perform every `MkdirAll`, `OpenFile` and
`Symlink` through it with the entry's *relative* path. The kernel enforces containment
per-component against held descriptors.

### A2. `filepath.EvalSymlinks` resolve-then-check per entry

Resolve each entry's parent before the containment check, then open normally.

### A3. `github.com/cyphar/filepath-securejoin`

Delegate path resolution to `SecureJoin` / `pathrs-lite.OpenInRoot`.

### A4. Two-pass validation (scan all headers, simulate, then extract)

Model symlink resolution lexically across the whole entry list before writing anything.

### A5. Extract to staging, then verify

Extract into a scratch directory, walk the result, reject and delete if anything escaped.

## Evidence

**Measured behavior of A1** (three probes, two run directly in this session):

```
Symlink a -> "."                       -> nil     (creating links is unrestricted)
Symlink b -> "a/.."                    -> nil
OpenFile b/pwned                       -> openat b/pwned: path escapes from parent
MkdirAll  b/x                          -> mkdirat b/x: path escapes from parent
Symlink esc -> "../../outside"         -> nil     (escaping targets still creatable)
OpenFile through escaping final symlink -> blocked; target file byte-identical after
OpenFile through in-root symlink        -> nil     (legitimate archives unaffected)
Symlink abs -> "/etc"; open abs/passwd  -> blocked
```

A1 additionally blocked the two-archive CVE-2025-45582 variant and a live directory-swap
race, because enforcement is per-syscall against held directory descriptors -- there is no
resolved-path cache to poison.

**A2 is disqualified, not merely weaker.** Homebrew bottle symlinks are *dangling* at
extraction time, so `EvalSymlinks` returns `no such file or directory` and cannot
distinguish "dangling" from "escaping" without parsing error strings. That is precisely
CPython issue #107845. It is also structurally TOCTOU: it returns a string, and the open
that follows is a separate, unprotected syscall.

**A3 is disqualified on semantics.** Measured: `SecureJoin` returned `err=<nil>` for
*every* attack path. It **clamps** rather than rejects -- `b/pwned` becomes `dest/pwned`,
`/etc/shadow` becomes `dest/etc/shadow`. Safe-by-rewriting silently mangles filenames
instead of failing on a tampered archive, which is worse than an error for a package
manager. `pathrs-lite.OpenInRoot` is TOCTOU-safe but uses `RESOLVE_IN_ROOT`, the same
clamping semantics, and lacks `Symlink`/`Chmod`, forcing a hybrid with plain `os`. It also
adds a dependency.

**A4 is the approach with the worst track record.** It is a lexical simulation of the
filesystem that must model symlink resolution, `..`, `strip_dirs`, pre-existing
destination contents, and hardlinks -- forever. node-tar's symlink cache, tar-fs, and PEP
706's `data` filter all took this shape and all shipped either a bypass or broke
legitimate archives. It also cannot see state that a *second* extraction into the same
destination created, which is the CVE-2025-45582 gap.

**A5 does not prevent the vulnerability**, it detects it. By the time the walk runs, the
write has already landed on whatever the symlink pointed at. It is only sound if staging
sits on a filesystem the attack cannot reach out of -- which symlinks defeat by definition.

**Convergent industry practice.** GNU tar, Docker (`chrootarchive`), containerd and git
all police traversal rather than link content. GNU tar's own CVE-2025-45582 fix is
this shape: jail the destination, let the kernel enforce.

**Cost of A1 is zero on the dependency axis.** `go.mod` declares `go 1.25.8`; `os.Root`
has been stdlib since 1.24 and carries a `unix||windows||wasip1` build tag covering both
supported platforms. The two workflows naming an older Go switch toolchains via
`GOTOOLCHAIN=auto`.

**Compatibility cost of A1 is measured at zero.** Across 146 real archives from the local
download cache -- 89 containing symlinks, 49,590 symlink entries -- there was not one
archive that writes an entry through a symlink, no absolute link target, and no link
escaping at the `strip_dirs` its recipe actually uses.

## Recommendation

**A1 -- `os.Root`.** It is the only option strong on all four axes (correctness against
the chain, compatibility with real archives, dependency cost, blast radius), it is where
the rest of the ecosystem landed, and it draws the line at traversal, which is where the
line belongs.

### The coupled half: what happens to the lexical validators

`isPathWithinDirectory` and `validateSymlinkTarget` are **retained**, demoted from
security boundary to policy layer and pre-filter.

Retaining `validateSymlinkTarget` is a deliberate scope decision, not an oversight.
Deleting it would allow symlinks whose targets leave the destination -- which is exactly
what #2275 asks for and what this change is explicitly not chartered to grant. Keeping it
makes this change **strictly additive on security**: no archive that extracts today stops
extracting, and no archive that fails today starts succeeding. The blast radius is
confined to archives that were escaping, which is the population the issue is about.

It also leaves #2275 as a clean, separable follow-up: once traversal is enforced by the
kernel, granting #2275 is deleting one function and its call sites, with the security
guarantee already in place underneath. That is a strictly better position than today,
where #2275 cannot be granted at all without opening the hole this change closes.

`isPathWithinDirectory` is retained as a cheap pre-filter for a second reason: `os.Root`
exports no sentinel error and its message (`openat b/pwned: path escapes from parent`)
names neither the archive nor the entry. The lexical check catches the plain `../`
traversals with a message that names the offending entry, and `os.Root` catches everything
it cannot see. Neither is load-bearing alone; only `os.Root` is load-bearing for security.

## Consequences

**Positive.** Containment becomes a kernel-enforced property rather than a predicted one.
The guarantee survives archive shapes nobody enumerated, including the two-archive and
racing variants. The `SECURITY:` comments become true. #2275 becomes tractable.

**Negative.** Extraction now holds an open directory handle for the duration of the loop.
Error messages from the enforcement layer are less specific than hand-written ones, which
the pre-filter partially offsets. The two validators now have a subtler role than their
names suggest, so their comments must say plainly that they are policy and not the
security boundary -- otherwise the next reader repeats the original mistake.

**Mitigation for the subtlety.** The `SECURITY:` comments get rewritten to state exactly
what each layer guarantees and, for the lexical helpers, what they explicitly do not.

## Summary

`os.Root` is the mechanism: it is stdlib at the version already required, it blocks the
#2473 chain and its racing and two-archive variants at the kernel, and it costs nothing
measurable on a 146-archive corpus. The four alternatives fail concretely rather than
theoretically -- `EvalSymlinks` cannot see dangling bottle links, `securejoin` clamps
instead of rejecting, two-pass validation is the approach that produced the CVEs, and
staging-then-verify detects after the write lands. The lexical validators are retained and
demoted to policy so the change stays strictly additive and leaves #2275 separable.

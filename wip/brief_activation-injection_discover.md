# Brief Discovery: activation-injection

Grounding: no ROADMAP. This brief is framed from tsukumogami/tsuku#2553
and from this chain's own `/shirabe:explore`, whose findings are at
`wip/explore_activation-injection_findings.md`. No `upstream:` is recorded —
there is no roadmap and therefore no durable strategic ancestor to walk up to.

Invoked under `/scope` (sentinel present, `rationale: fresh-chain`), so
Phase 4 finalize takes the Parent-delegated-approval shape: the BRIEF lands
in Draft and `/scope` owns the acceptance prompt.

## The problem/outcome pair

**Problem.** Values a cloned repository controls reach two kinds of sink
unguarded: a filesystem path component, and text a login shell evaluates. The
guards that would stop them exist in the codebase already and are wired to
other consumers.

**Outcome.** A repository the user has not read cannot change what runs on
their machine by declaring a tool, and the documents that describe these
controls describe what the code does.

## What the explore settled, carried in

- The class is a **placement** class. Six controls do the right thing one
  consumer deep from the choke point. The design record cites each guard's
  existence as proof it covers a sink it never runs on.
- **Three** hostile-controlled values reach the activation sink, not two:
  the tool name, the declared version, and `result.Dir`. `Dir` is fixable
  only at the emitter — it is a legitimate path that happens to contain
  metacharacters — which is what proves the validator and the quoter
  orthogonal.
- The fix is a **boundary**: `parseConfigFile` in `internal/project` is the
  single place `.tsuku.toml` becomes a config, and it enforces only
  `MaxTools`. Validating there covers activation, `tsuku install`,
  `tsuku shim install`, auto-apply and the `tsuku run` fast path at once.
- A correct POSIX quoter already exists, unexported, at
  `internal/actions/set_env.go:252` — one package from the `%q` that got it
  wrong. Four packages hold four different answers to shell quoting.

## Journeys the feature has to serve

Four distinct entry points, each a different user and trigger:

1. Someone clones a repository and activates in it (the reported vector).
2. Someone clones a repository and runs a project tool — `tsuku run`,
   no shell integration at all. Broader: no hook, no `tsuku shell`.
3. Someone writes a `.tsuku.toml` by hand and typos a name (the honest case
   that the same rejection catches, and the reason the error must name the key).
4. Someone reads the design record to decide whether a control exists.

## Scope edges, settled with the reviewer seats

**In:** the boundary validator (name and version), the shell quoter as a leaf
package, the second unquoted emitter in `cmd/tsuku`, two sink-level backstop
calls, and the design-record corrections.

**Out, each with an owner:** the `/`-walk discovery scope (#2555), the
`--recipe` destructive-sink hardening (its own issue), the org-scoped
activation failure (`tsuku_activation_pins`' PR), the fish `shellenv` path
(#2556), and the install consent prompt hiding the registry source.

## Open framing question deferred to the PRD

Whether the BRIEF survives consolidation at all. `/scope` Phase 2 decides that
against the PRD once both exist; it is not decided here.

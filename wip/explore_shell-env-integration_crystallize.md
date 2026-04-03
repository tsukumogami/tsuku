# Crystallize Decision: shell-env-integration

## Chosen Type

Design Doc

## Rationale

The "what" is clear from research: update `envFileContent` in config.go and install.sh
to source the init cache, fix the TSUKU_NO_TELEMETRY preservation bug in EnsureEnvFile,
and implement `tsuku doctor --rebuild-cache` with a shell setup staleness check. But
the "how" has real implementation decisions that need to be on record: how does a static
env file detect which shell it's running in? How does EnsureEnvFile handle migration
without clobbering user customizations? What does doctor's repair mode look like? These
are architectural choices that a future contributor (or the implementer mid-PR) would
need to re-derive without a design doc.

Additionally, decisions were made during exploration that must survive beyond this branch:
the choice of static env file over `eval "$(tsuku shellenv)"`, the three-part fix scope,
the deferral of fish shell handling. If these aren't captured, the next person who looks
at this problem starts from scratch.

## Signal Evidence

### Signals Present

- What to build is clear, how to build it is not: requirements established (tools with
  `install_shell_init` must work in new terminals); implementation approach has open
  decisions (shell detection, migration behavior, doctor repair scope)
- Multiple viable implementation paths surfaced: three approaches identified (update env
  file, switch to eval, add separate sourcing line in installer)
- Technical decisions need to be made and recorded: static file vs eval tradeoff,
  EnsureEnvFile rewrite-vs-preserve behavior, `--rebuild-cache` design
- Architectural decisions made during exploration: chosen approach (static env file update)
  needs to be on record with its rationale
- Core question is "how should we build this?": confirmed by user's ready-to-decide
  after round 1

### Anti-Signals Checked

- What to build is still unclear: not present — requirements confirmed
- No meaningful technical risk or trade-offs: not present — real tradeoffs exist
  (subprocess overhead, static file shell detection, migration safety)
- Problem is operational not architectural: not present — this is a design gap in how
  two subsystems (env file, shell.d) are wired together

## Alternatives Considered

- **Plan**: Ranked second. Would work once the design doc exists. Tiebreaker confirms:
  no upstream design doc for this topic yet.
- **No Artifact**: Demoted. Architectural decisions made during exploration need permanent
  documentation before wip/ is cleaned.
- **PRD**: Demoted. Requirements were given as input by the user, not identified during
  exploration.

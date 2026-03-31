# Crystallize Decision: background-update-checks

## Chosen Type
Design Doc

## Rationale
The PRD already defines the requirements (R4: time-cached checks, R5: layered triggers, R19: zero latency). The exploration made 7 architectural decisions that need a permanent record: per-tool cache files, advisory flock for dedup, separate detached process, hidden check-updates subcommand, shared CheckUpdateStaleness function, notification state separation, and LLMConfig-pattern configuration. These decisions answer "how should we build this?" -- the core question for a design doc.

## Signal Evidence
### Signals Present
- What to build is clear, but how is not: PRD defines R4/R5/R19 requirements; exploration investigated 6 technical leads to determine the implementation approach
- Technical decisions need to be made between approaches: per-tool vs single cache file, flock vs PID files vs mtime-only dedup, goroutine vs detached process, embedded in ComputeActivation vs separate code path
- Architecture, integration, or system design questions remain: trigger layering across hook-env/shim/command, background process lifecycle, cache schema design for downstream consumers
- Multiple viable implementation paths: each lead surfaced 2-3 alternatives that were evaluated and narrowed
- Architectural decisions were made during exploration: 7 decisions captured in decisions file that need permanent documentation
- Core question is "how should we build this?": the PRD answered "what"; the design doc answers "how"

### Anti-Signals Checked
- What to build is still unclear: NOT PRESENT -- PRD requirements are specific and accepted
- No meaningful technical risk or trade-offs: NOT PRESENT -- significant trade-offs exist (single vs per-tool files, dedup mechanisms, process models)
- Problem is operational: NOT PRESENT -- this is architectural (new subsystem with multiple integration points)

## Alternatives Considered
- **PRD**: Requirements already exist in docs/prds/PRD-auto-update.md (Accepted status). Anti-signal: "requirements were provided as input." Score 0, demoted.
- **Plan**: No design doc exists to decompose. Anti-signals: "technical approach still debated" and "open architectural decisions." The exploration's 7 decisions need a formal design doc before planning. Score 0, demoted.
- **No Artifact**: 7 architectural decisions were made that need a permanent home. Anti-signals: "others need documentation to build from" (Features 3, 5, 6 depend on this design) and "decisions were made during exploration." Score -1, demoted.

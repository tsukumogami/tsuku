# Design Documents

This directory contains design documents for tsuku features and architectural decisions.

## Status Lifecycle

Design documents progress through the following statuses:

| Status | Description | Location |
|--------|-------------|----------|
| **Proposed** | Design under discussion, not yet approved | `docs/designs/` |
| **Accepted** | Design approved, awaiting implementation | `docs/designs/` |
| **Planned** | Design approved, issues and milestones created | `docs/designs/` |
| **Current** | Implementation complete, stable | `docs/designs/current/` |
| **Superseded** | Replaced by another design (includes link) | `docs/designs/archive/` |

## Planned Designs

These designs are approved with milestones created, but implementation is ongoing.

| Design | Description |
|--------|-------------|
| [DESIGN-structured-install-guide.md](DESIGN-structured-install-guide.md) | Sandbox container building and full golden coverage |

## Current Designs

These designs are implemented and represent the current system architecture.

### Core Infrastructure

| Design | Description |
|--------|-------------|
| [DESIGN-deterministic-resolution.md](current/DESIGN-deterministic-resolution.md) | Plan-based installation and deterministic execution |
| [DESIGN-dependency-pattern.md](current/DESIGN-dependency-pattern.md) | Implicit dependency system for actions |
| [DESIGN-dependency-provisioning.md](current/DESIGN-dependency-provisioning.md) | Build essentials and system dependency provisioning |
| [DESIGN-dependency-version-refs.md](current/DESIGN-dependency-version-refs.md) | Variable expansion for dependency versions |
| [DESIGN-sandbox-dependencies.md](current/DESIGN-sandbox-dependencies.md) | Unified plan generation for sandbox mode |
| [DESIGN-relocatable-library-deps.md](current/DESIGN-relocatable-library-deps.md) | Native library dependency management |

### LLM & Builder Infrastructure

| Design | Description |
|--------|-------------|
| [DESIGN-llm-builder-infrastructure.md](current/DESIGN-llm-builder-infrastructure.md) | LLM-powered recipe generation infrastructure |
| [DESIGN-recipe-builders.md](current/DESIGN-recipe-builders.md) | Ecosystem-specific recipe builders |

### Sandbox & Container Testing

| Design | Description |
|--------|-------------|
| [DESIGN-install-sandbox.md](current/DESIGN-install-sandbox.md) | Centralized sandbox testing infrastructure |
| [DESIGN-system-dependency-actions.md](current/DESIGN-system-dependency-actions.md) | System dependency action vocabulary |

### Golden Files & Validation

| Design | Description |
|--------|-------------|
| [DESIGN-golden-plan-testing.md](current/DESIGN-golden-plan-testing.md) | CI-driven regression testing for recipe plans |
| [DESIGN-hardcoded-version-detection.md](current/DESIGN-hardcoded-version-detection.md) | Recipe validation for version templating |
| [DESIGN-non-deterministic-validation.md](current/DESIGN-non-deterministic-validation.md) | Constrained evaluation for ecosystem recipe validation |
| [DESIGN-version-verification.md](current/DESIGN-version-verification.md) | Version format transforms and verification modes |

### Ecosystem Support

| Design | Description |
|--------|-------------|
| [DESIGN-go-ecosystem.md](current/DESIGN-go-ecosystem.md) | Go toolchain, version provider, and go_install action |
| [DESIGN-perl-ecosystem.md](current/DESIGN-perl-ecosystem.md) | Perl/CPAN support with cpan_install action |
| [DESIGN-homebrew.md](current/DESIGN-homebrew.md) | Homebrew bottle integration |
| [DESIGN-fossil-archive.md](current/DESIGN-fossil-archive.md) | Fossil SCM source archive support |

### CLI & UX Features

| Design | Description |
|--------|-------------|
| [DESIGN-info-enhancements.md](current/DESIGN-info-enhancements.md) | CLI info command improvements |
| [DESIGN-telemetry-cli.md](current/DESIGN-telemetry-cli.md) | Anonymous usage telemetry |
| [DESIGN-structured-logging.md](current/DESIGN-structured-logging.md) | Unified structured logging framework |
| [DESIGN-multi-version-support.md](current/DESIGN-multi-version-support.md) | Multiple installed versions per tool |
| [DESIGN-recipe-detail-pages.md](current/DESIGN-recipe-detail-pages.md) | Individual tool pages on tsuku.dev |
| [DESIGN-when-clause-platform-tuples.md](current/DESIGN-when-clause-platform-tuples.md) | Platform tuple support in when clauses |

### Security & Verification

| Design | Description |
|--------|-------------|
| [DESIGN-checksum-pinning.md](current/DESIGN-checksum-pinning.md) | Post-installation binary integrity verification |
| [DESIGN-pgp-verification.md](current/DESIGN-pgp-verification.md) | PGP signature verification for downloads |

## Archived Designs

See [archive/](archive/) for superseded designs. Each archived design includes a link to its replacement.

## Creating New Designs

1. Create a new file in `docs/designs/` with status "Proposed"
2. Follow the design document template (title, status, context, decision, consequences)
3. After approval, update status to "Accepted" or "Planned"
4. When implementation is complete, move to `current/` with status "Current"
5. If superseded by a new design, move to `archive/` with status "Superseded by [link]"

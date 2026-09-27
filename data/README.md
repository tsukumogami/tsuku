# Data Directory

Static data files used by batch recipe generation infrastructure.

## Files

### schemas/

JSON Schema (draft-07) definitions for data files:

- **priority-queue.schema.json** - Validates `data/queues/priority-queue.json`, the unified queue: one entry per tool with a pre-resolved `ecosystem:identifier` source, priority (1-3), status, confidence and backoff state. Mirrors `batch.QueueEntry` in `internal/batch/queue_entry.go`.
- **failure-record.schema.json** - Validates `data/failures/*.json`. Defines per-ecosystem-environment failure records with categorized failure types and optional `blocked_by` dependencies.

### Validation

Schema validation scripts in `scripts/`:

- `scripts/validate-queue.sh` - Validates `data/queues/priority-queue.json` against the queue schema
- `scripts/validate-queue-recipes.sh` - Fails when a pending queue entry has the same name as an existing recipe
- `scripts/validate-failures.sh` - Validates `data/failures/*.json` against the failure record schema

### examples/

Sample data files demonstrating valid structure:

- **priority-queue.json** - Example unified queue with entries in several statuses
- **failure-record.json** - Example failure records showing different failure categories

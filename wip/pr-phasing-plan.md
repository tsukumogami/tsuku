# PR Phasing Plan: System-Lib Backfill

Split PR #1863 into 3 phases, each validated through test-recipe
workflow before merge.

## P0: crates.io + rubygems (185 recipes)

- Branch: new branch off main
- Content: 102 crates.io + 83 rubygems recipes
- All standalone (no dep relationships), 100% generation success rate
- Validate through test-recipe, merge

## P1: homebrew focused (507 recipes)

- Branch: new branch off main (after P0 merges)
- Content:
  - 10 library blocker recipes (libgit2, bdw-gc, ada-url, dav1d,
    oniguruma, glib, tree-sitter, libevent, libidn2, notmuch)
  - 21 satisfies backfill modifications on existing recipes
  - 403 homebrew recipes with valid runtime_dependencies
  - 73 homebrew recipes with stripped deps (needed fix-script)
- Validate through test-recipe, merge

## P2: rest of homebrew + design doc (639+ recipes)

- Branch: rebase current branch (docs/system-lib-backfill) onto main
  after P0+P1 merge
- Content:
  - 639 standalone homebrew recipes (no deps)
  - Design doc transition (Current status)
  - Parent design doc updates
  - Queue status updates (priority-queue.json)
  - Friction log, ranked blocker list
- Most recipe files will already be on main from P0+P1, so rebase
  should auto-resolve those. Remaining diff is the standalone homebrew
  recipes + non-recipe files.
- Validate through test-recipe, merge

## Current state

- Branch `docs/system-lib-backfill` has all 1,310 new + 21 modified
  recipes, design doc changes, and queue updates
- PR #1863 is open but CI was cancelled (waiting for this split)
- wip/pr-scope-analysis.md has the detailed recipe breakdown

# Issue 14 Baseline

## Issue Summary
- **Number**: 14
- **Title**: feat(cli): add progress bars for downloads
- **Milestone**: v0.2.0

## Problem Statement
Large downloads (nodejs ~85MB, rust toolchain ~300MB) show no visual feedback. Users cannot tell if a download is progressing or stuck.

## Expected Behavior
Display progress during downloads with:
- Percentage complete
- Downloaded vs total size
- Transfer speed
- Estimated time remaining

Progress bars should be suppressed in non-TTY environments (CI) or when `--quiet` is set.

## Branch
- Feature branch: `feature/14-progress-bars`
- Base: `main` at commit 7900ce5

## Acceptance Criteria
1. Progress bar displays during downloads
2. Shows percentage, size, speed, and ETA
3. Suppressed in non-TTY environments
4. Suppressed when --quiet flag is set (when implemented)

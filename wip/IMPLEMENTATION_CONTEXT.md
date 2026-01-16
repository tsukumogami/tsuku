# Implementation Context for Issue #884

## Goal

Investigate and fix gdbm-source sandbox test failures due to GNU FTP mirror returning 403.

## Context

Observed in PR #858 CI run. The gdbm-source sandbox test fails when downloading from GNU FTP:

```
Downloading: https://ftpmirror.gnu.org/gnu/gdbm/gdbm-1.26.tar.gz
Installation failed: step 1 (download_file) failed: download failed: bad status: 403 Forbidden
```

This is a transient infrastructure issue - the GNU FTP mirror is rejecting requests.

## Acceptance Criteria

- [ ] Investigate if this is a rate limiting or geo-blocking issue
- [ ] Consider adding retry logic for transient HTTP errors
- [ ] Consider using alternative mirrors or caching strategy
- [ ] Sandbox test passes reliably

## Dependencies

None

## Tier

Simple (no design doc reference)

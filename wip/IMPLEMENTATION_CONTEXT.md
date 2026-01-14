## Goal

Fix the libsixel-source recipe test failure on macOS Apple Silicon.

## Context

During CI batching work in #878, the aggregated macOS Apple Silicon job revealed that libsixel-source fails during the meson build step on macOS.

The Linux version of this test passes successfully.

## Error Details

The failure occurs in the Build Essentials workflow when running the aggregated macOS tests. The specific error needs further investigation, but it appears to be related to the meson build process on Apple Silicon.

## Acceptance Criteria

- [ ] Investigate root cause of libsixel-source failure on macOS
- [ ] Fix the recipe or build configuration
- [ ] Re-enable libsixel-source in macOS CI tests
- [ ] Verify the fix passes on both Apple Silicon and Intel

## Dependencies

None

## Notes

libsixel-source is temporarily excluded from macOS CI tests until this is resolved.

# Issue 1148 Summary

## What Changed

Regenerated golden plan files for library recipes to use per-family naming:
- `gcc-libs`: 1 generic file -> 5 per-family files (debian, rhel, arch, alpine, suse)
- `libyaml`: 1 generic file -> 5 per-family files
- `openssl`: 1 generic file -> 5 per-family files
- `ruby`: Updated to reflect new libyaml recipe hash (cascade effect)

## Files Changed

- Deleted: 3 generic linux-amd64 files
- Added: 15 per-family files (5 families x 3 recipes)
- Modified: 6 darwin files (updated recipe metadata), 1 ruby linux file

## Validation

All golden file validations pass for both linux and darwin platforms.
